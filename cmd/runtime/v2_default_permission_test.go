package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ly-solutions/harnessly/internal/wire"
)

func TestDefaultPermissionOf(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bypass := write("bypass.json", `{"permissions": {"defaultMode": "bypassPermissions"}}`)
	plan := write("plan.json", `{"permissions": {"defaultMode": "plan"}}`)
	never := write("never.toml", "approval_policy = \"never\"\n")
	asks := write("asks.toml", "model = \"x\"\n\n[t]\napproval_policy = \"never\"\n")
	cases := []struct {
		name          string
		p             wire.Provider
		skip          bool
		codex, claude string
		want          wire.PermissionMode
	}{
		{"runtime skips", wire.ProviderClaude, true, "", "", wire.PermissionBypass},
		{"claude bypass default", wire.ProviderClaude, false, "", bypass, wire.PermissionBypass},
		{"claude plan does not decide", wire.ProviderClaude, false, "", plan, wire.PermissionAsk},
		{"claude no settings", wire.ProviderClaude, false, "", filepath.Join(dir, "none.json"), wire.PermissionAsk},
		{"claude reads no codex", wire.ProviderClaude, false, never, "", wire.PermissionAsk},
		{"codex never", wire.ProviderCodex, false, never, bypass, wire.PermissionBypass},
		{"codex in a table", wire.ProviderCodex, false, asks, bypass, wire.PermissionAsk},
	}
	for _, c := range cases {
		if got := defaultPermissionOf(c.p, c.skip, c.codex, c.claude); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
