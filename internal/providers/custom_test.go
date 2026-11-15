package providers

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// customToken is a value only the safe has: it is joined from parts so no scanner takes it for a key.
const customToken = "corp-" + "token-0000-abcdef"

func customStore(t *testing.T, safe map[string]string) (*Store, string) {
	t.Helper()
	s := newStore(t)
	s.Safe = func(name string) (string, error) {
		v, ok := safe[name]
		if !ok {
			return "", errors.New("no secret")
		}
		return v, nil
	}
	hooks := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(hooks, []byte(`{"hooks":{"Stop":[]},"env":{"HARNESSLY_DATA_DIR":"/data","KEPT":"1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetCustom(nil) })
	return s, hooks
}

func envOf(t *testing.T, path string) (map[string]any, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	env, _ := settings["env"].(map[string]any)
	return settings, env
}

// The environment of a Claude Code chat on a provider the person added: what they typed replaces what the launch sets,
// what they left empty stays as the launch makes it, and the secret values come from the safe.
func TestCustomProfileLaunchEnvironment(t *testing.T) {
	s, hooks := customStore(t, map[string]string{"provider-corp-env-anthropic-auth-token": customToken, "provider-corp-env-ai-seats-key": "seats-0001"})
	SetCustom([]Preset{{
		ID: "x-corp", Label: "Corp proxy", Vendor: wire.VendorAuto, Via: wire.ProviderClaude,
		BaseURL: "https://llm.corp.example/anthropic", Host: "llm.corp.example",
		Models: []Model{{ID: "corp-large", Label: "Large"}, {ID: "corp-small", Label: "Small"}}, Small: "corp-large", Test: "corp-large",
		Env: map[string]string{
			"ANTHROPIC_BASE_URL":         "https://proxy.corp.example/v2",
			"ANTHROPIC_SMALL_FAST_MODEL": "corp-small",
			"NODE_EXTRA_CA_CERTS":        "/etc/ssl/corp-ca.pem",
			"CORP_TEAM":                  "platform",
		},
		SecretEnv: map[string]string{"ANTHROPIC_AUTH_TOKEN": "provider-corp-env-anthropic-auth-token", "AI_SEATS_KEY": "provider-corp-env-ai-seats-key"},
	}})
	if !s.Ready("x-corp") {
		t.Fatal("a profile with a token of its own is ready without a saved key")
	}
	path, err := s.SettingsFile("x-corp", "corp-large", hooks)
	if err != nil {
		t.Fatal(err)
	}
	settings, env := envOf(t, path)
	want := map[string]string{
		// typed by the person: always replaces
		"ANTHROPIC_BASE_URL":         "https://proxy.corp.example/v2",
		"ANTHROPIC_SMALL_FAST_MODEL": "corp-small",
		"NODE_EXTRA_CA_CERTS":        "/etc/ssl/corp-ca.pem",
		"CORP_TEAM":                  "platform",
		"ANTHROPIC_AUTH_TOKEN":       customToken,
		"AI_SEATS_KEY":               "seats-0001",
		// left empty: as the launch makes it
		"ANTHROPIC_MODEL":         "corp-large",
		"ANTHROPIC_API_KEY":       "",
		"CLAUDE_CODE_OAUTH_TOKEN": OAuthStub,
		// the hooks file's own
		"HARNESSLY_DATA_DIR": "/data",
		"KEPT":               "1",
	}
	// An agent that prints its environment does not show the token in the feed: neither in this runtime, which
	// launched the chat, nor in one started after it (the chat goes on with the same variables).
	frame := []byte(`{"text":"ANTHROPIC_AUTH_TOKEN=` + customToken + ` AI_SEATS_KEY=seats-0001"}`)
	again := &Store{Dir: s.Dir, Safe: s.Safe}
	for name, st := range map[string]*Store{"the launching runtime": s, "a restarted runtime": again} {
		if out := string(st.Redact(frame)); strings.Contains(out, customToken) || strings.Contains(out, "seats-0001") || !strings.Contains(out, Mask(customToken)) {
			t.Errorf("%s shows a secret variable in the feed: %s", name, out)
		}
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %v, want %q", k, env[k], v)
		}
	}
	if _, has := env["CLAUDE_CODE_MAX_CONTEXT_TOKENS"]; has {
		t.Error("a window nobody told is not passed as 0")
	}
	if _, has := settings["apiKeyHelper"]; has {
		t.Error("the provider's own token is not shadowed by a key helper")
	}
	if settings["hooks"] == nil {
		t.Error("the hooks of the launch are gone")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the settings file is not the person's alone: %v %v", info, err)
	}
}

func TestCustomProfileWithAKeyOnly(t *testing.T) {
	s, hooks := customStore(t, map[string]string{"provider-deepseek-key": fakeKey})
	SetCustom([]Preset{{ID: "x-deepseek", Label: "DeepSeek", Vendor: wire.VendorAuto, Via: wire.ProviderClaude,
		BaseURL: "https://api.deepseek.com/anthropic", Host: "api.deepseek.com", KeyRef: "provider-deepseek-key",
		Models: []Model{{ID: "deepseek-chat", Label: "Chat"}}, Small: "deepseek-chat", Test: "deepseek-chat"}})
	if !s.Ready("x-deepseek") {
		t.Fatal("a profile whose key the safe has is ready")
	}
	path, err := s.SettingsFile("x-deepseek", "deepseek-chat", hooks)
	if err != nil {
		t.Fatal(err)
	}
	settings, env := envOf(t, path)
	if env["ANTHROPIC_BASE_URL"] != "https://api.deepseek.com/anthropic" || env["ANTHROPIC_AUTH_TOKEN"] != "" {
		t.Errorf("env: %v", env)
	}
	helper, _ := settings["apiKeyHelper"].(string)
	if !strings.Contains(helper, "x-deepseek.key") {
		t.Errorf("the key is read by the helper from its file: %q", helper)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), fakeKey) {
		t.Error("the key is in the settings file")
	}
}

func TestCustomProfileRefusals(t *testing.T) {
	s, hooks := customStore(t, map[string]string{})
	SetCustom([]Preset{
		{ID: "x-gone", Label: "Gone", Vendor: wire.VendorAuto, Via: wire.ProviderClaude, BaseURL: "https://a.example", Host: "a.example",
			Models: []Model{{ID: "m"}}, Small: "m", Test: "m", SecretEnv: map[string]string{"ANTHROPIC_AUTH_TOKEN": "missing"}},
		{ID: "x-codex", Label: "Codex one", Via: wire.ProviderCodex, Models: []Model{{ID: "m"}}},
		{ID: "x-empty", Label: "No models", Via: wire.ProviderClaude},
		{ID: "kimi", Label: "Not Kimi", Via: wire.ProviderClaude, Models: []Model{{ID: "m"}}},
	})
	// A secret the safe does not have stops the launch; it never starts with an empty token.
	if _, err := s.SettingsFile("x-gone", "m", hooks); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("a missing secret: %v", err)
	}
	// A secret that is the token of a Claude subscription starts no chat: the machine has one such sign-in.
	token := "sk-ant-" + "oat01-not-a-token-of-anyone"
	s2, hooks2 := customStore(t, map[string]string{"named": token, "named-key": token})
	SetCustom([]Preset{
		{ID: "x-second", Label: "Second", Vendor: wire.VendorOwn, Via: wire.ProviderClaude, BaseURL: "https://api.anthropic.com", Host: "api.anthropic.com",
			Models: []Model{{ID: "m"}}, Small: "m", Test: "m", SecretEnv: map[string]string{"ANTHROPIC_AUTH_TOKEN": "named"}},
		{ID: "x-keyed", Label: "Keyed", Vendor: wire.VendorOwn, Via: wire.ProviderClaude, BaseURL: "https://api.anthropic.com", Host: "api.anthropic.com",
			KeyRef: "named-key", Models: []Model{{ID: "m"}}, Small: "m", Test: "m"},
		{ID: "x-gone", Label: "Gone", Vendor: wire.VendorAuto, Via: wire.ProviderClaude, BaseURL: "https://a.example", Host: "a.example",
			Models: []Model{{ID: "m"}}, Small: "m", Test: "m", SecretEnv: map[string]string{"ANTHROPIC_AUTH_TOKEN": "missing"}},
		{ID: "x-codex", Label: "Codex one", Via: wire.ProviderCodex, Models: []Model{{ID: "m"}}},
		{ID: "x-empty", Label: "No models", Via: wire.ProviderClaude},
		{ID: "kimi", Label: "Not Kimi", Via: wire.ProviderClaude, Models: []Model{{ID: "m"}}},
	})
	if _, err := s2.SettingsFile("x-second", "m", hooks2); !errors.Is(err, ErrSubscriptionToken) {
		t.Fatalf("a subscription's token in a variable: %v", err)
	}
	if s2.Ready("x-keyed") {
		t.Fatal("a provider whose key is a subscription's token is ready")
	}
	if raw, err := os.ReadFile(s2.KeyFile("x-keyed")); err == nil {
		t.Fatalf("the token was written as the key: %d bytes", len(raw))
	}
	for _, id := range []string{"x-codex", "x-empty"} {
		if _, ok := Find(id); ok {
			t.Errorf("%s is no profile Claude Code starts on", id)
		}
	}
	// A provider of the person never stands in for a built-in preset.
	if p, _ := Find("kimi"); p.Custom || p.Label == "Not Kimi" {
		t.Errorf("kimi was replaced: %+v", p.Label)
	}
}
