package obsidian

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// What a connection keeps on the machine, under its own folder (Instance.Dir):
//
//	index.json         the vault's name, the device, the notes it shares (path → hash, size, times, title, tags,
//	                   links) and the batches already taken;
//	notes/<key>.md     the text of each shared note, named by the hash of its path (no path of the vault is ever a path
//	                   of the machine);
//	changes.jsonl      the journal of what came: created, modified, removed, the newest journalMax lines.
//
// Only what the person chose to share in Obsidian ever reaches here; a note they stop sharing is removed at once.

const (
	journalMax = 2000
	// idsMax batches are remembered, a repeat of one is taken once.
	idsMax = 500
)

var storeMu sync.Mutex

type note struct {
	Hash  string    `json:"hash"`
	Size  int       `json:"size"`
	MTime time.Time `json:"mtime"`
	CTime time.Time `json:"ctime,omitzero"`
	Title string    `json:"title,omitempty"`
	Tags  []string  `json:"tags,omitempty"`
	Links []string  `json:"links,omitempty"`
	// Seen is when the machine first got it; EvAt when it last gave a note.changed (one per changeQuiet).
	Seen time.Time `json:"seen"`
	EvAt time.Time `json:"evAt,omitzero"`
}

// manifest is a full list of the shared paths the plugin sends in parts at the end of a full sync: whatever is not in
// it is no longer shared and goes.
type manifest struct {
	Sync  string   `json:"sync"`
	Parts int      `json:"parts"`
	Got   []int    `json:"got"`
	Paths []string `json:"paths"`
}

type index struct {
	Vault    string           `json:"vault,omitempty"`
	Device   string           `json:"device,omitempty"`
	Plugin   string           `json:"plugin,omitempty"`
	PairedAt time.Time        `json:"pairedAt,omitzero"`
	SyncedAt time.Time        `json:"syncedAt,omitzero"`
	Learned  bool             `json:"learned,omitempty"`
	Notes    map[string]*note `json:"notes"`
	IDs      []string         `json:"ids,omitempty"`
	Pending  *manifest        `json:"pending,omitempty"`
}

type change struct {
	At     time.Time `json:"at"`
	Path   string    `json:"path"`
	Change string    `json:"change"`
	Size   int       `json:"size,omitempty"`
}

type store struct{ dir string }

func (st store) indexFile() string   { return filepath.Join(st.dir, "index.json") }
func (st store) journalFile() string { return filepath.Join(st.dir, "changes.jsonl") }
func (st store) noteFile(p string) string {
	sum := sha256.Sum256([]byte(p))
	return filepath.Join(st.dir, "notes", hex.EncodeToString(sum[:16])+".md")
}

// load reads the index; a missing one is an empty vault, not paired yet.
func (st store) load() (*index, error) {
	ix := &index{Notes: map[string]*note{}}
	if st.dir == "" {
		return ix, nil
	}
	raw, err := os.ReadFile(st.indexFile())
	if errors.Is(err, os.ErrNotExist) {
		return ix, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, ix); err != nil {
		return nil, err
	}
	if ix.Notes == nil {
		ix.Notes = map[string]*note{}
	}
	return ix, nil
}

func (st store) save(ix *index) error {
	if err := os.MkdirAll(st.dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(ix)
	if err != nil {
		return err
	}
	return writeFile(st.indexFile(), raw)
}

func writeFile(name string, raw []byte) error {
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}

func (st store) writeNote(p, text string) error {
	if err := os.MkdirAll(filepath.Join(st.dir, "notes"), 0o700); err != nil {
		return err
	}
	return writeFile(st.noteFile(p), []byte(text))
}

func (st store) readNote(p string) (string, error) {
	raw, err := os.ReadFile(st.noteFile(p))
	return string(raw), err
}

func (st store) removeNote(p string) {
	_ = os.Remove(st.noteFile(p))
}

// journal appends changes and keeps the newest journalMax.
func (st store) journal(list []change) error {
	if len(list) == 0 {
		return nil
	}
	old := st.changes()
	old = append(old, list...)
	if len(old) > journalMax {
		old = old[len(old)-journalMax:]
	}
	var b bytes.Buffer
	for _, c := range old {
		raw, _ := json.Marshal(c)
		b.Write(raw)
		b.WriteByte('\n')
	}
	return writeFile(st.journalFile(), b.Bytes())
}

func (st store) changes() []change {
	f, err := os.Open(st.journalFile())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []change
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var c change
		if json.Unmarshal(sc.Bytes(), &c) == nil {
			out = append(out, c)
		}
	}
	return out
}

// seenID remembers a batch id and says whether it was taken before.
func (ix *index) seenID(id string) bool {
	if slices.Contains(ix.IDs, id) {
		return true
	}
	ix.IDs = append(ix.IDs, id)
	if len(ix.IDs) > idsMax {
		ix.IDs = ix.IDs[len(ix.IDs)-idsMax:]
	}
	return false
}

// backlinks are the shared notes that link to p.
func (ix *index) backlinks(p string) []string {
	var out []string
	for from, n := range ix.Notes {
		if slices.Contains(n.Links, p) {
			out = append(out, from)
		}
	}
	slices.Sort(out)
	return out
}

// cleanPath is a vault path fit to keep: relative, forward slashes, no «..», no control characters, a note.
func cleanPath(p string) (string, bool) {
	if p == "" || len(p) > 512 || !utf8.ValidString(p) || strings.ContainsAny(p, "\\\x00") {
		return "", false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	if strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return "", false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == "." || part == "" {
			return "", false
		}
	}
	if !isNote(p) {
		return "", false
	}
	return p, true
}

func isNote(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	return ext == ".md" || ext == ".canvas"
}

// folderOf is the note's folder, "/" for the root.
func folderOf(p string) string {
	if d := path.Dir(p); d != "." {
		return d
	}
	return "/"
}

// cleanTag is a tag without its «#», or "" when it is not one.
var tagRe = regexp.MustCompile(`^[\p{L}\p{N}_/\-]{1,80}$`)

func cleanTag(t string) string {
	t = strings.TrimPrefix(strings.TrimSpace(t), "#")
	if !tagRe.MatchString(t) {
		return ""
	}
	return t
}

// taskRe is an open task with a due date as the Tasks plugin writes it: «- [ ] call Ann 📅 2026-11-20» (or
// «due: 2026-11-20», «[due:: 2026-11-20]»).
var taskRe = regexp.MustCompile(`^\s*[-*+] \[ \] .*?(?:📅\s*|due::?\s*)(\d{4}-\d{2}-\d{2})`)

type task struct {
	Line int
	Due  string
	Key  string
}

// tasks are the open dated tasks of a text; Key tells one apart from another of the same note without its words.
func tasks(text string) []task {
	var out []task
	for i, line := range strings.Split(text, "\n") {
		m := taskRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		sum := sha256.Sum256([]byte(strings.TrimSpace(line)))
		out = append(out, task{Line: i + 1, Due: m[1], Key: hex.EncodeToString(sum[:8])})
	}
	return out
}
