package sessionaccounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A noted session keeps its account over a restart; a session nobody noted has none.
func TestASessionKeepsItsAccountOverARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2", "session-accounts.jsonl")
	s := &Store{Path: path}
	if err := s.Note(Entry{Provider: "claude", Session: "s-1", Account: "x-kimi-work", Label: "Kimi рабочий", Ref: "key:sha256:0123456789abcdef", Host: "api.kimi.com"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Note(Entry{Provider: "claude", Session: "s-1", Account: "x-kimi-work", Label: "Kimi рабочий", Ref: "key:sha256:0123456789abcdef", Host: "api.kimi.com"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if n := strings.Count(string(raw), "\n"); n != 1 {
		t.Fatalf("the same account was written %d times", n)
	}
	again := &Store{Path: path}
	if e, ok := again.Of("claude", "s-1"); !ok || e.Account != "x-kimi-work" || e.Label != "Kimi рабочий" || e.Ref != "key:sha256:0123456789abcdef" || e.Host != "api.kimi.com" {
		t.Fatalf("after a restart: %+v %v", e, ok)
	}
	if _, ok := again.Of("claude", "s-2"); ok {
		t.Fatal("a session nobody noted has an account")
	}
	if _, ok := again.Of("codex", "s-1"); ok {
		t.Fatal("another agent's session of the same id has the account")
	}
}

// The account's name as it is now replaces the one noted before, and a line cut short is left out.
func TestTheLastNoteWinsAndABrokenLineIsLeftOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-accounts.jsonl")
	s := &Store{Path: path}
	_ = s.Note(Entry{Provider: "claude", Session: "s-1", Account: "kimi-code", Label: "Kimi Code"})
	_ = s.Note(Entry{Provider: "claude", Session: "s-1", Account: "kimi-code", Label: "Kimi личный"})
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"provider":"claude","session":"s-9","acc`)
	_ = f.Close()
	again := &Store{Path: path}
	if e, _ := again.Of("claude", "s-1"); e.Label != "Kimi личный" {
		t.Fatalf("label %q", e.Label)
	}
	if _, ok := again.Of("claude", "s-9"); ok {
		t.Fatal("a line cut short was read")
	}
	if err := again.Note(Entry{Session: "s-3", Account: "a", Label: "b"}); err != nil {
		t.Fatal(err)
	}

	// What another process adds is read without a restart.
	other := &Store{Path: path}
	if err := other.Note(Entry{Provider: "codex", Session: "s-7", Account: "cx-work", Label: "Codex рабочий"}); err != nil {
		t.Fatal(err)
	}
	if e, ok := again.Of("codex", "s-7"); !ok || e.Label != "Codex рабочий" {
		t.Fatalf("a line of another process was not read: %+v %v", e, ok)
	}
}
