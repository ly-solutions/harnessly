package codexaccounts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAnAccountsHomeIsItsOwnWithTheSettingsOfTheMainOne(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "home", ".codex")
	write(t, filepath.Join(main, "auth.json"), `{"main":true}`)
	write(t, filepath.Join(main, "config.toml"), "model = \"x\"\n")
	write(t, filepath.Join(main, "rules", "default.rules"), "allow\n")
	write(t, filepath.Join(main, "skills", "x", "SKILL.md"), "# x\n")
	write(t, filepath.Join(main, "sessions", "2026", "a.jsonl"), "{}\n")
	write(t, filepath.Join(main, "state_5.sqlite"), "db")
	elsewhere := filepath.Join(dir, "elsewhere")
	write(t, filepath.Join(elsewhere, "SKILL.md"), "# linked\n")
	if err := os.Symlink(elsewhere, filepath.Join(main, "skills", "linked")); err != nil {
		t.Fatal(err)
	}
	s := &Store{Dir: filepath.Join(dir, "data"), Main: main}

	a, err := s.Add("  Вторая   подписка ")
	if err != nil || a.Name != "Вторая подписка" || !idRe.MatchString(a.ID) {
		t.Fatalf("%+v %v", a, err)
	}
	home := s.Home(a.ID)
	// The sign-in, the sessions and the databases are the account's own: nothing of the main ones is there.
	for _, name := range []string{"auth.json", "sessions", "state_5.sqlite"} {
		if _, err := os.Lstat(filepath.Join(home, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s of the main home is in the account's: %v", name, err)
		}
	}
	if s.SignedIn(a.ID) {
		t.Fatal("a new account has a sign-in")
	}
	// The settings are copies, not links.
	for _, name := range []string{"config.toml", "rules", filepath.Join("rules", "default.rules"), "skills", filepath.Join("skills", "x", "SKILL.md")} {
		if st, err := os.Lstat(filepath.Join(home, name)); err != nil || st.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s is not a copy: %v", name, err)
		}
	}
	if read(t, filepath.Join(home, "config.toml")) != "model = \"x\"\n" || read(t, filepath.Join(home, "skills", "linked", "SKILL.md")) != "# linked\n" {
		t.Fatal("the settings were not copied")
	}

	// What the CLI writes in the account's home stays there; the main home is never written.
	write(t, s.AuthFile(a.ID), `{"second":true}`)
	write(t, filepath.Join(home, "sessions", "2026", "b.jsonl"), "{}\n")
	write(t, filepath.Join(home, "state_5.sqlite"), "its own")
	write(t, filepath.Join(home, "config.toml"), "written by the CLI\n")
	if !s.SignedIn(a.ID) || read(t, filepath.Join(main, "auth.json")) != `{"main":true}` || read(t, filepath.Join(main, "config.toml")) != "model = \"x\"\n" {
		t.Fatal("the homes are not apart")
	}
	if _, err := os.Stat(filepath.Join(main, "sessions", "2026", "b.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a session of the account is in the main home: %v", err)
	}

	// The person changes the settings in the main home: the next launch has them, and nothing else is touched.
	write(t, filepath.Join(main, "config.toml"), "model = \"y\"\n")
	write(t, filepath.Join(main, "AGENTS.md"), "be brief\n")
	if err := os.RemoveAll(filepath.Join(main, "skills", "x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(a.ID); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(home, "config.toml")) != "model = \"y\"\n" || read(t, filepath.Join(home, "AGENTS.md")) != "be brief\n" {
		t.Fatal("a change of the settings was not copied")
	}
	if _, err := os.Lstat(filepath.Join(home, "skills", "x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a removed skill was kept: %v", err)
	}
	if read(t, s.AuthFile(a.ID)) != `{"second":true}` || read(t, filepath.Join(home, "state_5.sqlite")) != "its own" ||
		read(t, filepath.Join(home, "sessions", "2026", "b.jsonl")) != "{}\n" {
		t.Fatal("a sync touched what is the account's own")
	}

	// Names: no empty one, no second of the same.
	if _, err := s.Add("вторая подписка"); !errors.Is(err, ErrBadName) {
		t.Fatalf("%v", err)
	}
	if _, err := s.Rename(a.ID, " "); !errors.Is(err, ErrBadName) {
		t.Fatalf("%v", err)
	}
	if r, err := s.Rename(a.ID, "Рабочая"); err != nil || r.Name != "Рабочая" {
		t.Fatalf("%+v %v", r, err)
	}
	if list, err := s.List(); err != nil || len(list) != 1 || list[0].Name != "Рабочая" {
		t.Fatalf("%+v %v", list, err)
	}

	// One sign-in at most is the one a new chat is offered first.
	b, err := s.Add("Ещё одна")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMain(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMain(b.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(); len(list) != 2 || list[0].Main || !list[1].Main {
		t.Fatalf("%+v", list)
	}
	if err := s.SetMain("nosuch1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if err := s.SetMain(""); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(); list[0].Main || list[1].Main {
		t.Fatalf("%+v", list)
	}
	if err := s.Remove(b.ID); err != nil {
		t.Fatal(err)
	}

	// Removing an account takes its home, and nothing of the main one or of what a skill linked to.
	if err := s.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(home)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the home is still there: %v", err)
	}
	if read(t, filepath.Join(main, "sessions", "2026", "a.jsonl")) != "{}\n" || read(t, filepath.Join(main, "config.toml")) == "" ||
		read(t, filepath.Join(main, "auth.json")) != `{"main":true}` || read(t, filepath.Join(elsewhere, "SKILL.md")) != "# linked\n" {
		t.Fatal("removing an account took something that is not its own")
	}
	if err := s.Remove(a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if err := s.Sync("../x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}
