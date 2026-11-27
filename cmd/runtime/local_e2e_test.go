package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/providers"
)

// The whole way of a local model against a real Ollama on 127.0.0.1:11434 and the `claude` on the
// PATH: found by the discovery, made a provider, its profile's settings file, and a chat of Claude Code that gets an
// answer. It runs only where HARNESSLY_E2E_OLLAMA names a model the server has (qwen2.5:0.5b on a CPU machine).
func TestLocalModelWithARealOllama(t *testing.T) {
	model := os.Getenv("HARNESSLY_E2E_OLLAMA")
	if model == "" {
		t.Skip("HARNESSLY_E2E_OLLAMA names a model of a running Ollama")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	svc := &extproviders.Service{Store: &extproviders.Store{Dir: t.TempDir()}, Secrets: memSafe{}}
	found, err := svc.Discover(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var server *extproviders.LocalServer
	for i, s := range found.Servers {
		if s.Kind == "ollama" {
			server = &found.Servers[i]
		}
	}
	if server == nil || server.API != extproviders.APIAnthropic {
		t.Fatalf("discovery: %+v", found)
	}
	view, err := svc.Create(extproviders.Input{Name: extproviders.LocalName(server.Name), BaseURL: server.BaseURL, API: server.API,
		Preset: extproviders.PresetLocal, Models: server.Models})
	if err != nil {
		t.Fatal(err)
	}
	store := &providers.Store{Dir: filepath.Join(t.TempDir(), "providers")}
	t.Cleanup(func() { providers.SetCustom(nil) })
	syncExtProfiles(svc, store)
	id := extProfileID(view.ID)
	if !store.Ready(id) {
		t.Fatal("the profile of a local model is not ready")
	}
	hooks := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(hooks, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := store.SettingsFile(id, model, hooks)
	if err != nil {
		t.Fatal(err)
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		home, _ := os.UserHomeDir()
		claude = filepath.Join(home, ".local", "bin", "claude")
	}
	cmd := exec.CommandContext(ctx, claude, "-p", "Say hi in one word.", "--settings", settings, "--model", model, "--setting-sources", "")
	// An empty home of its own: nothing of the person's sign-in is there to be used.
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+t.TempDir(), "ANTHROPIC_API_KEY=", "ANTHROPIC_AUTH_TOKEN=", "CLAUDE_CODE_OAUTH_TOKEN=")
	out, err := cmd.CombinedOutput()
	t.Logf("claude: %s", out)
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("claude: %v", err)
	}

	// The same server for Codex: its custom provider at /v1, no key.
	cid := localCodexProfileID(view.ID)
	if !store.Ready(cid) {
		t.Fatal("the Codex profile of a local model is not ready")
	}
	prof, err := (providers.Launcher{Store: store}).Codex(cid)
	if err != nil || prof.KeyEnv != "" {
		t.Fatalf("codex profile: %+v, %v", prof, err)
	}
	codex, err := exec.LookPath("codex")
	if err != nil {
		home, _ := os.UserHomeDir()
		codex = filepath.Join(home, ".local", "bin", "codex")
	}
	args := append([]string{"exec", "--skip-git-repo-check", "-m", model}, prof.Args...)
	args = append(args, "Say hi in one word.")
	cx := exec.CommandContext(ctx, codex, args...)
	cx.Dir = t.TempDir()
	cx.Env = append(os.Environ(), "CODEX_HOME="+t.TempDir())
	out, err = cx.CombinedOutput()
	t.Logf("codex: %s", out)
	if err != nil || !strings.Contains(string(out), "\ncodex\n") {
		t.Fatalf("codex: %v", err)
	}
}

type memSafe map[string]string

func (m memSafe) Put(name, value string) error { m[name] = value; return nil }
func (m memSafe) Reveal(name string) (string, error) {
	return m[name], nil
}
func (m memSafe) Remove(name string) error { delete(m, name); return nil }
func (m memSafe) Has(name string) bool     { _, ok := m[name]; return ok }
