// Package codexaccounts keeps more than one sign-in of the Codex CLI on a machine. Codex keeps its sign-in
// in $CODEX_HOME/auth.json and renews it there itself, so a second sign-in is a second home, and the home is the
// sign-in's own whole: its auth.json, its sessions, its databases and everything else the CLI writes. Nothing in it
// is a link to the main home, and the main home (the CLI's own, ~/.codex) is only ever read. What a person sets up
// by hand there (settings) is copied into the sign-in's home before a launch on it (Sync), so the main home stays
// the one place to change it.
package codexaccounts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// settings are the entries of the main home a person sets up by hand: the configuration, the instructions, the rules,
// the prompts and the skills. They are copied into a sign-in's home; nothing else of the main home is.
var settings = []string{"config.toml", "AGENTS.md", "rules", "prompts", "skills"}

// MaxAccounts caps the sign-ins of a machine; MaxName a name's length in runes.
const (
	MaxAccounts = 8
	MaxName     = 60
)

var idRe = regexp.MustCompile(`^[a-z0-9]{6,16}$`)

// The refusals the caller is told.
var (
	ErrNotFound = errors.New("codexaccounts: no such account")
	ErrTooMany  = errors.New("codexaccounts: too many accounts")
	ErrBadName  = errors.New("codexaccounts: the name is empty, too long or taken")
)

// Account is one more sign-in of Codex beside the main one (the main one is the CLI's own home and is not listed).
type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Main: a new Codex chat that names no sign-in is offered this one first. One account at most; none means the
	// CLI's own home.
	Main bool `json:"main,omitempty"`
}

// Store keeps the accounts of a data folder. Main is the CLI's own home (~/.codex or $CODEX_HOME).
type Store struct {
	// Dir is the runtime's data folder: the list is <Dir>/v2/codex-accounts.json, a home <Dir>/codex-accounts/<id>.
	Dir  string
	Main string

	mu sync.Mutex
}

func (s *Store) file() string { return filepath.Join(s.Dir, "v2", "codex-accounts.json") }

func (s *Store) folder(id string) string { return filepath.Join(s.Dir, "codex-accounts", id) }

// Home is the CODEX_HOME of an account. It is named .codex like the main one, so a transcript in it reads as Codex's
// wherever a path is told apart by that name.
func (s *Store) Home(id string) string { return filepath.Join(s.folder(id), ".codex") }

// AuthFile is the auth.json of an account.
func (s *Store) AuthFile(id string) string { return filepath.Join(s.Home(id), "auth.json") }

func (s *Store) read() ([]Account, error) {
	raw, err := os.ReadFile(s.file())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Account
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(a Account) bool { return !idRe.MatchString(a.ID) }), nil
}

func (s *Store) write(list []Account) error {
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file()), 0o700); err != nil {
		return err
	}
	tmp := s.file() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file())
}

// List is the accounts in the order they were added.
func (s *Store) List() ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

// cleanName is a name as it is kept; ok is false for one that is not taken.
func cleanName(name string, others []Account, self string) (string, bool) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > MaxName {
		return "", false
	}
	for _, a := range others {
		if a.ID != self && strings.EqualFold(a.Name, name) {
			return "", false
		}
	}
	return name, true
}

// Add makes an account and its home (not signed in yet).
func (s *Store) Add(name string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return Account{}, err
	}
	if len(list) >= MaxAccounts {
		return Account{}, ErrTooMany
	}
	name, ok := cleanName(name, list, "")
	if !ok {
		return Account{}, ErrBadName
	}
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return Account{}, err
	}
	a := Account{ID: hex.EncodeToString(b), Name: name}
	if err := s.sync(a.ID); err != nil {
		return Account{}, err
	}
	return a, s.write(append(list, a))
}

// Rename gives an account another name.
func (s *Store) Rename(id, name string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return Account{}, err
	}
	i := slices.IndexFunc(list, func(a Account) bool { return a.ID == id })
	if i < 0 {
		return Account{}, ErrNotFound
	}
	name, ok := cleanName(name, list, id)
	if !ok {
		return Account{}, ErrBadName
	}
	list[i].Name = name
	return list[i], s.write(list)
}

// SetMain makes an account the one a new chat is offered first; "" gives that back to the CLI's own home.
func (s *Store) SetMain(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	if id != "" && !slices.ContainsFunc(list, func(a Account) bool { return a.ID == id }) {
		return ErrNotFound
	}
	for i := range list {
		list[i].Main = id != "" && list[i].ID == id
	}
	return s.write(list)
}

// Remove forgets an account and takes its home away: its sign-in, its sessions and its copies of the settings. The
// main home has nothing of it.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(list, func(a Account) bool { return a.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if err := s.write(slices.Delete(list, i, i+1)); err != nil {
		return err
	}
	return os.RemoveAll(s.folder(id))
}

// Sync copies the settings of the main home into an account's home: each as the main home has it now, and none the
// main home lost. Everything else in the account's home is left as it is. It is called before every launch on the
// account.
func (s *Store) Sync(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !idRe.MatchString(id) {
		return ErrNotFound
	}
	return s.sync(id)
}

func (s *Store) sync(id string) error {
	home := s.Home(id)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	for _, name := range settings {
		if err := mirror(filepath.Join(s.Main, name), filepath.Join(home, name)); err != nil {
			return err
		}
	}
	return nil
}

// mirror makes dst what src is: a copy of the file, a folder with copies of what the folder has, or nothing when src
// is not there. A link at src is followed; a link inside a folder is made again as it is (it is the person's own).
func mirror(src, dst string) error {
	st, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return os.RemoveAll(dst)
	}
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst)
	}
	if was, err := os.Lstat(dst); err == nil && !was.IsDir() {
		if err := os.Remove(dst); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, e := range ents {
		from, to := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		switch {
		case e.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(from)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(src, target)
			}
			if had, err := os.Readlink(to); err != nil || had != target {
				if err := os.RemoveAll(to); err != nil {
					return err
				}
				if err := os.Symlink(target, to); err != nil {
					return err
				}
			}
		case e.IsDir() || e.Type().IsRegular():
			if err := mirror(from, to); err != nil {
				return err
			}
		default:
			continue
		}
		keep[e.Name()] = true
	}
	mine, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, e := range mine {
		if !keep[e.Name()] {
			if err := os.RemoveAll(filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile puts a copy of src at dst in one step, so a CLI reading dst sees the old file or the new one.
func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if had, err := os.ReadFile(dst); err == nil && string(had) == string(raw) {
		return nil
	}
	if was, err := os.Lstat(dst); err == nil && was.IsDir() {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// SignedIn says whether an account's home has a sign-in.
func (s *Store) SignedIn(id string) bool {
	st, err := os.Stat(s.AuthFile(id))
	return err == nil && st.Mode().IsRegular() && st.Size() > 2
}
