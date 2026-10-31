package sources

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/jscompat"
	"github.com/ly-solutions/harnessly/internal/jsvalue"
	"github.com/ly-solutions/harnessly/internal/messagecounts"
	"github.com/ly-solutions/harnessly/internal/nodejson"
	"github.com/ly-solutions/harnessly/internal/transcript"
)

const (
	maxIndexes = 64
	chunkBytes = 8 * 1024 * 1024
)

// Route answers GET <platform API>/sources?jsonlPath=&cwd=: the links and work paths of a whole conversation. The
// index of a transcript is kept and grows with the file: a request reads only the whole lines appended since the
// previous one, in the stretches the previous runtime read, so both see the same entries.
type Route struct {
	Links *Links
	// Home holds the transcripts the route may read (.claude/projects and .codex/sessions under it).
	Home string
	// Codex are the options Codex transcripts are read with.
	Codex transcript.CodexOptions
	// Authorized is the old API's own check (a page session or the cli-token), SessionOK the page session the handler
	// asks for besides.
	Authorized, SessionOK func(*http.Request) bool

	mu      sync.Mutex
	order   []string
	indexes map[string]*index
}

type index struct {
	mu        sync.Mutex
	ino       uint64
	offset    int64
	collector Collector
	mentions  Mentions
	paths     Paths
	made      Made
	// reading: a background read of a long growth is under way; err is what it ended with.
	reading bool
	err     error
}

// Result is what the route answers for a transcript.
type Result struct {
	Links []Link
	// MRs are the merge requests the agent and the person wrote down, oldest first; they are not in Value().
	MRs       []MRMention
	WorkPaths []any
	Size      int64
	// Made are the documents, boards and designs the conversation made or touched, newest first.
	Made []MadeLink
}

// Value is the answer as the previous runtime wrote it.
func (r Result) Value() *jsvalue.Object {
	links := make([]any, len(r.Links))
	for i, l := range r.Links {
		links[i] = l.Value()
	}
	paths := r.WorkPaths
	if paths == nil {
		paths = []any{}
	}
	return jsvalue.NewObject("links", links, "workPaths", paths, "size", r.Size)
}

func errorBody(msg string) map[string]string { return map[string]string{"error": msg} }

// Answer is the whole answer, refusals included.
func (rt *Route) Answer(r *http.Request) (int, any, http.Header) {
	if !rt.Authorized(r) {
		return http.StatusUnauthorized, gate.Raw{ContentType: "application/json", Body: []byte(`{"error":"Unauthorized"}`)}, nil
	}
	if !rt.SessionOK(r) {
		return nodejson.Answer(r, http.StatusForbidden, errorBody("Forbidden"), nil)
	}
	if r.Method != http.MethodGet {
		return http.StatusMethodNotAllowed, nil, http.Header{"Allow": {"GET"}}
	}
	h := http.Header{"Cache-Control": {"no-store"}}
	q := jscompat.Query(r.URL.RawQuery)
	path, cwd := "", ""
	if v := q["jsonlPath"]; len(v) == 1 {
		path = v[0]
	}
	if v := q["cwd"]; len(v) == 1 && strings.HasPrefix(v[0], "/") {
		cwd = v[0]
	}
	res, found, failed := rt.Read(path, cwd)
	if failed {
		return http.StatusInternalServerError, gate.Raw{ContentType: "text/plain", Body: []byte("Internal Server Error")}, h
	}
	if !found {
		// No transcript yet (a chat that has not written its first line): nothing found, not an error.
		return nodejson.Answer(r, http.StatusOK, jsvalue.NewObject("links", []any{}, "workPaths", []any{}, "complete", false), h)
	}
	return nodejson.Answer(r, http.StatusOK, res.Value(), h)
}

// Read is the sources of a transcript: found is false for a path that is not an allowed transcript file, failed true
// where the previous runtime's parser threw (it answered 500).
func (rt *Route) Read(path, cwd string) (res Result, found, failed bool) {
	if path == "" || strings.Contains(path, "\x00") || !messagecounts.AllowedJsonl(rt.Home, path) {
		return Result{}, false, false
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return Result{}, false, false
	}
	idx := rt.indexFor(path, st)
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if err := idx.advance(path, st.Size(), rt.parser(path)); err != nil {
		return Result{}, true, true
	}
	return idx.result(cwd), true, false
}

func (idx *index) result(cwd string) Result {
	return Result{Links: idx.collector.List(), MRs: idx.mentions.List(), WorkPaths: idx.paths.List(cwd), Size: idx.offset, Made: idx.made.List()}
}

// ReadBudget is Read for a page that cannot wait for a long transcript: a growth of at most budget bytes is read now
// and the answer is complete; a larger one is read behind (one reader a transcript, in the background) and the answer
// is what has been read so far, not complete. Ask again for the rest.
func (rt *Route) ReadBudget(path, cwd string, budget int64) (res Result, found, failed, complete bool) {
	if path == "" || strings.Contains(path, "\x00") || !messagecounts.AllowedJsonl(rt.Home, path) {
		return Result{}, false, false, false
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return Result{}, false, false, false
	}
	idx := rt.indexFor(path, st)
	parse := rt.parser(path)
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.err != nil {
		idx.err = nil
		return Result{}, true, true, false
	}
	if st.Size()-idx.offset <= budget {
		if idx.reading {
			// The background reader is at the end: let it finish; what is read is what we answer.
			return idx.result(cwd), true, false, false
		}
		if err := idx.advance(path, st.Size(), parse); err != nil {
			return Result{}, true, true, false
		}
		return idx.result(cwd), true, false, true
	}
	if !idx.reading {
		idx.reading = true
		go idx.readBehind(path, st.Size(), parse)
	}
	return idx.result(cwd), true, false, false
}

// readBehind reads to size a chunk at a time, letting go of the index between chunks so answers do not wait for it.
func (idx *index) readBehind(path string, size int64, parse parseFunc) {
	for {
		idx.mu.Lock()
		if idx.offset >= size {
			idx.reading = false
			idx.mu.Unlock()
			return
		}
		before := idx.offset
		err := idx.advanceOne(path, size, parse)
		if err != nil || idx.offset == before {
			idx.reading = false
			if err != nil {
				idx.err = err
			}
			idx.mu.Unlock()
			return
		}
		idx.mu.Unlock()
	}
}

func inode(st os.FileInfo) uint64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Ino)
	}
	return 0
}

// indexFor is the file's index, a fresh one when the file was replaced or shrank; the most recently read are kept.
func (rt *Route) indexFor(path string, st os.FileInfo) *index {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.indexes == nil {
		rt.indexes = map[string]*index{}
	}
	idx := rt.indexes[path]
	if idx != nil {
		idx.mu.Lock()
		stale := idx.ino != inode(st) || idx.offset > st.Size()
		idx.mu.Unlock()
		if stale {
			idx = nil
		}
	}
	if idx == nil {
		idx = &index{ino: inode(st), collector: Collector{Links: rt.Links}, mentions: Mentions{Links: rt.Links}, made: Made{Links: rt.Links}}
	}
	rt.indexes[path] = idx
	for i, k := range rt.order {
		if k == path {
			rt.order = append(rt.order[:i], rt.order[i+1:]...)
			break
		}
	}
	rt.order = append(rt.order, path)
	for len(rt.order) > maxIndexes {
		delete(rt.indexes, rt.order[0])
		rt.order = rt.order[1:]
	}
	return idx
}

type parseFunc func(content string) ([]*jsvalue.Object, bool)

func (rt *Route) parser(path string) parseFunc {
	if strings.Contains(path, "/.codex/sessions/") {
		return func(content string) ([]*jsvalue.Object, bool) {
			return transcript.ParseCodexContentChecked(content, rt.Codex)
		}
	}
	return transcript.ParseContentChecked
}

type parseError struct{}

func (parseError) Error() string { return "the transcript parser threw" }

// advance reads whole lines from the index's offset to size in bounded stretches; a line still being written is left
// for the next request, a single line longer than a stretch is skipped.
func (idx *index) advance(path string, size int64, parse parseFunc) error {
	for idx.offset < size {
		before := idx.offset
		if err := idx.advanceOne(path, size, parse); err != nil {
			return err
		}
		if idx.offset == before {
			return nil
		}
	}
	return nil
}

// advanceOne reads the next stretch; the offset does not move when only a line still being written is left.
func (idx *index) advanceOne(path string, size int64, parse parseFunc) error {
	if idx.offset >= size {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	length := min(int64(chunkBytes), size-idx.offset)
	buf := make([]byte, length)
	n, err := f.ReadAt(buf, idx.offset)
	if err != nil && err != io.EOF {
		return err
	}
	last := bytes.LastIndexByte(buf[:n], '\n')
	if last < 0 {
		if length < chunkBytes {
			return nil
		}
		idx.offset += int64(n)
		return nil
	}
	entries, threw := parse(jscompat.DecodeUTF8(buf[:last+1]))
	if threw {
		return parseError{}
	}
	idx.collector.Add(entries)
	idx.mentions.Add(entries)
	idx.paths.Add(entries)
	idx.made.Add(entries)
	idx.offset += int64(last + 1)
	return nil
}
