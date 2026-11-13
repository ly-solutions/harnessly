package settingsctl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// JournalKept is how many changes the journal keeps; the oldest go first.
const JournalKept = 500

// Journal is the changes of the settings in a file, a JSON line each, the newest last.
type Journal struct {
	Path string

	mu sync.Mutex
}

func (j *Journal) read() []Change {
	raw, err := os.ReadFile(j.Path)
	if err != nil {
		return nil
	}
	var out []Change
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var c Change
		if json.Unmarshal(sc.Bytes(), &c) == nil && c.ID != "" {
			out = append(out, c)
		}
	}
	return out
}

// Append adds a change. A journal past JournalKept is written anew without its oldest lines.
func (j *Journal) Append(c Change) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(j.Path), 0o700); err != nil {
		return err
	}
	if all := j.read(); len(all) >= JournalKept {
		all = append(all[len(all)-JournalKept+1:], c)
		var b bytes.Buffer
		for _, x := range all {
			l, _ := json.Marshal(x)
			b.Write(l)
			b.WriteByte('\n')
		}
		tmp := j.Path + ".tmp"
		if err := os.WriteFile(tmp, b.Bytes(), 0o600); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, j.Path)
	}
	f, err := os.OpenFile(j.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// List is the last changes, the newest first: at most limit of them (all the kept ones when limit is 0), of one
// setting when key is given.
func (j *Journal) List(limit int, key string) []Change {
	j.mu.Lock()
	all := j.read()
	j.mu.Unlock()
	out := []Change{}
	for _, c := range slices.Backward(all) {
		if key != "" && c.Key != key {
			continue
		}
		out = append(out, c)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
