// Package sessionaccounts remembers which account of a provider each agent session ran on: the report of
// spend says how much went to every account, and a transcript does not say whose key paid for it. The runtime notes the
// account when it learns the agent's session; a session nobody noted (one from before this was kept, or one on the
// CLI's own sign-in) has no account.
package sessionaccounts

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is the account a session ran on: the launch profile it was started with and the name the person saw then.
type Entry struct {
	Provider string `json:"provider"`
	Session  string `json:"session"`
	Account  string `json:"account"`
	Label    string `json:"label"`
	// Ref names the account to the hub without its key (a digest of the key, usagefeed.KeyRef); "" when the account
	// has no key of its own. Host is where its calls go.
	Ref  string `json:"ref,omitempty"`
	Host string `json:"host,omitempty"`
	At   int64  `json:"at"`
}

// Store keeps the entries in one file, a line each, added to and never rewritten; the last line of a session wins.
type Store struct {
	Path string
	// Now is the clock (tests move it); nil is the real one.
	Now func() time.Time

	mu sync.Mutex
	// read is the size and the change time of the file as it was last read: another process (the runtime, for the
	// machine's agent) adds lines to it.
	read [2]int64
	by   map[string]Entry
}

func key(provider, session string) string { return provider + ":" + session }

func (s *Store) load() {
	at := [2]int64{-1, 0}
	if st, err := os.Stat(s.Path); err == nil {
		at = [2]int64{st.Size(), st.ModTime().UnixNano()}
	}
	if s.by != nil && at == s.read {
		return
	}
	s.read, s.by = at, map[string]Entry{}
	f, err := os.Open(s.Path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		// A line cut short by a stop of the machine is left out; the ones around it stand.
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Provider != "" && e.Session != "" && e.Account != "" {
			s.by[key(e.Provider, e.Session)] = e
		}
	}
}

// Note says the session ran on the account (At is set here). Saying the same again writes nothing.
func (s *Store) Note(e Entry) error {
	provider, session := e.Provider, e.Session
	if provider == "" || session == "" || e.Account == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	if had, ok := s.by[key(provider, session)]; ok {
		had.At = 0
		if had == e {
			return nil
		}
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	e.At = now().UnixMilli()
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	// A line a stop of the machine cut short is closed first, so that this one does not go on from it.
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		if tail, err := os.ReadFile(s.Path); err == nil && len(tail) > 0 && tail[len(tail)-1] != '\n' {
			line = append([]byte{'\n'}, line...)
		}
	}
	if _, err = f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	s.by[key(provider, session)] = e
	if st, err := os.Stat(s.Path); err == nil {
		s.read = [2]int64{st.Size(), st.ModTime().UnixNano()}
	}
	return nil
}

// Of is the account the session ran on; ok is false for a session nobody noted.
func (s *Store) Of(provider, session string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	e, ok := s.by[key(provider, session)]
	return e, ok
}
