package hlcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/apps"
)

func TestAppGuardHook(t *testing.T) {
	dir := t.TempDir()
	snap := apps.Snapshot{
		Apps:      []apps.SnapshotApp{{ID: "acme.review", Name: "Ревью", Tools: &apps.ToolPolicy{Deny: []string{"Bash(git push*)"}, Confirm: []string{"Edit"}}}},
		Protected: []string{"/data/apps"},
	}
	raw, _ := json.Marshal(snap)
	sum := sha256.Sum256(raw)
	policy := filepath.Join(dir, "policy-"+hex.EncodeToString(sum[:8])+".json")
	if err := os.WriteFile(policy, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(path, stdin string) (int, string) {
		var out bytes.Buffer
		code := Run([]string{"hook", "app-guard", "--policy", path}, Env{Getenv: func(string) string { return "" }, Stdin: strings.NewReader(stdin), Stdout: &out})
		return code, out.String()
	}
	decision := func(out string) string {
		var a struct {
			H struct {
				D string `json:"permissionDecision"`
			} `json:"hookSpecificOutput"`
		}
		_ = json.Unmarshal([]byte(out), &a)
		return a.H.D
	}

	for _, c := range []struct{ name, call, want string }{
		{"denied", `{"tool_name":"Bash","tool_input":{"command":"git push origin"}}`, "deny"},
		{"denied inside a line", `{"tool_name":"Bash","tool_input":{"command":"git status && git push"}}`, "deny"},
		{"a line it cannot read", `{"tool_name":"Bash","tool_input":{"command":"sh -c 'git push'"}}`, "ask"},
		{"confirm", `{"tool_name":"Edit","tool_input":{"file_path":"/w/a.go"}}`, "ask"},
		{"no rule", `{"tool_name":"Bash","tool_input":{"command":"git status"}}`, ""},
		{"turning an app off from inside", `{"tool_name":"Bash","tool_input":{"command":"harnessly app disable acme.review"}}`, "deny"},
		{"editing a package", `{"tool_name":"Write","tool_input":{"file_path":"/data/apps/packages/x/app.json"}}`, "deny"},
		{"an unreadable call", `not json`, "deny"},
	} {
		code, out := run(policy, c.call)
		if code != 0 || decision(out) != c.want || (c.want == "" && out != "") {
			t.Errorf("%s: %d %q", c.name, code, out)
		}
	}
	tampered := filepath.Join(dir, "policy-0000000000000000.json")
	_ = os.WriteFile(tampered, []byte(`{"apps":[]}`), 0o600)
	if code, out := run(tampered, `{"tool_name":"Bash","tool_input":{"command":"git push"}}`); code != 0 || decision(out) != "deny" {
		t.Fatalf("a changed snapshot refuses: %d %q", code, out)
	}
	if code, out := run(filepath.Join(dir, "gone.json"), `{"tool_name":"Read","tool_input":{}}`); code != 0 || decision(out) != "deny" {
		t.Fatalf("no snapshot refuses: %d %q", code, out)
	}
	var out bytes.Buffer
	if code := Run([]string{"hook", "app-guard"}, Env{Getenv: func(string) string { return "" }, Stdin: strings.NewReader(`{}`), Stdout: &out, Stderr: &out}); code == 0 {
		t.Fatal("app-guard without --policy should be a usage error")
	}
}
