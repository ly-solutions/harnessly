package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLauncherClaude(t *testing.T) {
	data := t.TempDir()
	reg := &Registry{Root: filepath.Join(data, "apps")}
	pkg := filepath.Join(t.TempDir(), "review")
	if err := Scaffold(pkg, "acme.review", "Ревью", "chat"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "instructions.md"), []byte("  Review only, never push.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.Install(pkg, InstallOptions{Dev: true, HostVersion: "0.17.0"}); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "hooks.json")
	own := `{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"design-guard"}]}],"Stop":[]},"env":{"A":"1"}}`
	if err := os.WriteFile(base, []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	l := &Launcher{Registry: reg, GuardCommand: "/bin/harnessly hook app-guard"}
	got, err := l.Claude([]string{"acme.review"}, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.PluginDirs) != 1 || got.PluginDirs[0] != pkg {
		t.Fatalf("plugins: %v", got.PluginDirs)
	}
	prompt, _ := os.ReadFile(got.PromptFile)
	if !strings.Contains(string(prompt), "## App: Ревью (acme.review)\n\nReview only, never push.") {
		t.Fatalf("prompt: %q", prompt)
	}
	raw, _ := os.ReadFile(got.Settings)
	var s struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	pre := s.Hooks["PreToolUse"]
	if len(pre) != 2 || pre[0].Matcher != "*" || !strings.HasPrefix(pre[0].Hooks[0].Command, "/bin/harnessly hook app-guard --policy ") || pre[0].Hooks[0].Timeout != GuardTimeoutSeconds || pre[1].Matcher != "Edit" {
		t.Fatalf("PreToolUse: %+v", pre)
	}
	policy := strings.Trim(strings.TrimPrefix(pre[0].Hooks[0].Command, "/bin/harnessly hook app-guard --policy "), "'")
	var snap Snapshot
	rawSnap, _ := os.ReadFile(policy)
	if json.Unmarshal(rawSnap, &snap) != nil || len(snap.Apps) != 1 || snap.Apps[0].Name != "Ревью" || len(snap.Protected) != 2 || snap.Protected[1] != pkg {
		t.Fatalf("snapshot: %s", rawSnap)
	}
	if s.Env["A"] != "1" {
		t.Fatalf("the base settings were lost: %s", raw)
	}
	if again, _ := l.Claude([]string{"acme.review"}, base); again.Settings != got.Settings || again.PromptFile != got.PromptFile {
		t.Fatalf("the same launch wrote new files: %+v %+v", again, got)
	}
	if _, err := l.Claude([]string{"acme.nope"}, base); err == nil {
		t.Fatal("an app not installed launched")
	}
}
