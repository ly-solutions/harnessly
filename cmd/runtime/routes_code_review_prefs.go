package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ly-solutions/harnessly/internal/gate"
)

// What the «Ревью кода» app keeps for the person on the machine, so that it is the same on every device:
//
//	GET  <platform>/code-review/prefs                      {instructions, repos, hideBots, bots}
//	POST <platform>/code-review/prefs {instructions?, repo?: {key, text}, hideBots?, bots?}   the same, changed
//	GET  <platform>/code-review/viewed?scope=<change>      {marks: {<path>: <version>}}
//	POST <platform>/code-review/viewed {scope, path, version}   version "" takes the mark off -> {marks}
//
// instructions go to every review the agent is asked for; repos are the instructions of one repository (key
// "<host>/<project>"), added to them. hideBots hides the merge requests of the code hosts' bots from the lists; bots are
// the authors the person calls bots too. The «viewed» marks are GitLab's: GitHub keeps its own (the review's acts
// "viewed" and "unviewed"). A mark holds the version of the file it was put on.

const (
	reviewTextMax   = 8000
	reviewReposMax  = 500
	reviewBotsMax   = 100
	reviewKeyMax    = 400
	reviewScopesMax = 400
	reviewMarksMax  = 5000
)

type reviewScope struct {
	// At is when the scope was last marked, for dropping the oldest past the limit.
	At    int64             `json:"at"`
	Marks map[string]string `json:"marks"`
}

type reviewPrefsFile struct {
	Instructions string                  `json:"instructions"`
	Repos        map[string]string       `json:"repos"`
	HideBots     bool                    `json:"hideBots"`
	Bots         []string                `json:"bots"`
	Viewed       map[string]*reviewScope `json:"viewed,omitempty"`
}

type reviewPrefs struct {
	path string
	now  func() time.Time

	mu sync.Mutex
}

var (
	reviewPrefsMu  sync.Mutex
	reviewPrefsFor = map[string]*reviewPrefs{}
)

func reviewPrefsOf(dataDir string) *reviewPrefs {
	reviewPrefsMu.Lock()
	defer reviewPrefsMu.Unlock()
	if s := reviewPrefsFor[dataDir]; s != nil {
		return s
	}
	s := &reviewPrefs{path: filepath.Join(dataDir, "v2", "code-review.json"), now: time.Now}
	reviewPrefsFor[dataDir] = s
	return s
}

// load reads the file; a missing or broken one is the defaults.
func (s *reviewPrefs) load() reviewPrefsFile {
	var f reviewPrefsFile
	if raw, err := os.ReadFile(s.path); err == nil {
		if json.Unmarshal(raw, &f) != nil {
			f = reviewPrefsFile{}
		}
	}
	if f.Repos == nil {
		f.Repos = map[string]string{}
	}
	if f.Bots == nil {
		f.Bots = []string{}
	}
	if f.Viewed == nil {
		f.Viewed = map[string]*reviewScope{}
	}
	return f
}

func (s *reviewPrefs) save(f reviewPrefsFile) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// reviewText is a text the person wrote, as it is kept: trimmed, and refused when it is too long.
func reviewText(v string) (string, bool) {
	v = strings.TrimSpace(v)
	return v, utf8.RuneCountInString(v) <= reviewTextMax
}

// reviewKey says whether a key (a repository, a change, a path) is one to keep.
func reviewKey(v string) bool {
	return v != "" && len(v) <= reviewKeyMax && !strings.ContainsAny(v, "\x00\n\r")
}

type reviewPrefsPatch struct {
	Instructions *string `json:"instructions"`
	Repo         *struct {
		Key  string `json:"key"`
		Text string `json:"text"`
	} `json:"repo"`
	HideBots *bool     `json:"hideBots"`
	Bots     *[]string `json:"bots"`
}

// change applies a patch; false when a part of it is not one to keep (nothing is written then).
func (s *reviewPrefs) change(p reviewPrefsPatch) (reviewPrefsFile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	if p.Instructions != nil {
		text, ok := reviewText(*p.Instructions)
		if !ok {
			return f, false, nil
		}
		f.Instructions = text
	}
	if p.Repo != nil {
		key := strings.ToLower(strings.TrimSpace(p.Repo.Key))
		text, ok := reviewText(p.Repo.Text)
		if !ok || !reviewKey(key) {
			return f, false, nil
		}
		if text == "" {
			delete(f.Repos, key)
		} else if _, known := f.Repos[key]; !known && len(f.Repos) >= reviewReposMax {
			return f, false, nil
		} else {
			f.Repos[key] = text
		}
	}
	if p.HideBots != nil {
		f.HideBots = *p.HideBots
	}
	if p.Bots != nil {
		bots := []string{}
		for _, b := range *p.Bots {
			b = strings.TrimSpace(b)
			if b == "" || slices.Contains(bots, b) {
				continue
			}
			if !reviewKey(b) || len(bots) >= reviewBotsMax {
				return f, false, nil
			}
			bots = append(bots, b)
		}
		f.Bots = bots
	}
	return f, true, s.save(f)
}

func (s *reviewPrefs) marks(scope string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sc := s.load().Viewed[scope]; sc != nil && sc.Marks != nil {
		return sc.Marks
	}
	return map[string]string{}
}

// mark puts a file's mark at a version, or takes it off when the version is empty.
func (s *reviewPrefs) mark(scope, path, version string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.load()
	sc := f.Viewed[scope]
	if sc == nil || sc.Marks == nil {
		sc = &reviewScope{Marks: map[string]string{}}
	}
	delete(sc.Marks, path)
	if version != "" && len(sc.Marks) < reviewMarksMax {
		sc.Marks[path] = version
	}
	sc.At = s.now().UnixMilli()
	if len(sc.Marks) == 0 {
		delete(f.Viewed, scope)
	} else {
		f.Viewed[scope] = sc
	}
	if len(f.Viewed) > reviewScopesMax {
		keys := make([]string, 0, len(f.Viewed))
		for k := range f.Viewed {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return f.Viewed[keys[i]].At < f.Viewed[keys[j]].At })
		for _, k := range keys[:len(f.Viewed)-reviewScopesMax] {
			delete(f.Viewed, k)
		}
	}
	return sc.Marks, s.save(f)
}

func reviewPrefsBody(f reviewPrefsFile) map[string]any {
	return map[string]any{"instructions": f.Instructions, "repos": f.Repos, "hideBots": f.HideBots, "bots": f.Bots}
}

func reviewPrefsRoutes(d deps, refuse func(*http.Request) (int, any, http.Header, bool), noStore http.Header) []gate.JSONRoute {
	store := reviewPrefsOf(d.DataDir)
	bad := func(r *http.Request) (int, any, http.Header) {
		return answerJSON(r, http.StatusBadRequest, map[string]string{"error": "bad-request"}, noStore)
	}
	failed := func(r *http.Request) (int, any, http.Header) {
		return answerJSON(r, http.StatusInternalServerError, map[string]string{"error": "not-saved"}, noStore)
	}
	either := func(path string) func(*http.Request) bool {
		return func(r *http.Request) bool {
			return (r.Method == http.MethodGet || r.Method == http.MethodPost) && pathIs(r, platformAPI+path)
		}
	}
	return []gate.JSONRoute{{
		Slice: "services",
		Match: either("/code-review/prefs"),
		Compute: func(r *http.Request) (int, any, http.Header) {
			if s, v, h, refused := refuse(r); refused {
				return s, v, h
			}
			if r.Method == http.MethodGet {
				store.mu.Lock()
				f := store.load()
				store.mu.Unlock()
				return answerJSON(r, http.StatusOK, reviewPrefsBody(f), noStore)
			}
			var in reviewPrefsPatch
			if !readSecretBody(r, &in) {
				return bad(r)
			}
			f, ok, err := store.change(in)
			if !ok {
				return bad(r)
			}
			if err != nil {
				return failed(r)
			}
			return answerJSON(r, http.StatusOK, reviewPrefsBody(f), noStore)
		},
	}, {
		Slice: "services",
		Match: either("/code-review/viewed"),
		Compute: func(r *http.Request) (int, any, http.Header) {
			if s, v, h, refused := refuse(r); refused {
				return s, v, h
			}
			if r.Method == http.MethodGet {
				scope := r.URL.Query().Get("scope")
				if !reviewKey(scope) {
					return bad(r)
				}
				return answerJSON(r, http.StatusOK, map[string]any{"marks": store.marks(scope)}, noStore)
			}
			var in struct {
				Scope   string `json:"scope"`
				Path    string `json:"path"`
				Version string `json:"version"`
			}
			if !readSecretBody(r, &in) || !reviewKey(in.Scope) || !reviewKey(in.Path) || len(in.Version) > reviewKeyMax {
				return bad(r)
			}
			marks, err := store.mark(in.Scope, in.Path, in.Version)
			if err != nil {
				return failed(r)
			}
			return answerJSON(r, http.StatusOK, map[string]any{"marks": marks}, noStore)
		},
	}}
}
