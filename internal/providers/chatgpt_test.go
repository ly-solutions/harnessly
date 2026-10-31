package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// The ChatGPT profile's settings point Claude Code at the proxy with the token it ignores, and have no key helper: there
// is no key to read.
func TestTheChatGPTSettingsGoToTheProxyWithoutAKey(t *testing.T) {
	s := newStore(t)
	s.Proxy = fakeProxy(t, "")
	hooks := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(hooks, []byte(`{"hooks":{},"env":{"KEEP":"1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := s.SettingsFile("chatgpt", "gpt-6-sol", hooks)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["apiKeyHelper"]; ok {
		t.Fatalf("a key helper with no key: %s", raw)
	}
	env, _ := got["env"].(map[string]any)
	// The address is that of the proxy the runtime started, on the port it picked.
	if base, _ := env["ANTHROPIC_BASE_URL"].(string); base != proxyURL(s.Proxy.port) || s.Proxy.port == 0 {
		t.Errorf("base %q, proxy on %d", base, s.Proxy.port)
	}
	for k, want := range map[string]string{
		"ANTHROPIC_AUTH_TOKEN":                      "unused",
		"ANTHROPIC_API_KEY":                         "",
		"ANTHROPIC_MODEL":                           "gpt-6-sol",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":             "gpt-6-luna",
		"ANTHROPIC_SMALL_FAST_MODEL":                "gpt-6-luna",
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS":            "272000",
		"CLAUDE_CODE_AUTO_COMPACT_WINDOW":           "272000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC":  "1",
		"CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK": "1",
		"CLAUDE_CODE_OAUTH_TOKEN":                   OAuthStub,
		"KEEP":                                      "1",
	} {
		if v, ok := env[k]; !ok || v != want {
			t.Errorf("env %s = %v (present %v), want %q", k, v, ok, want)
		}
	}
	// The proxy holds the sign-in: a key is refused, not saved where nothing reads it.
	if _, err := s.Set("chatgpt", fakeKey); !errors.Is(err, wire.ErrProfileNoKey) {
		t.Fatalf("a key for chatgpt: %v", err)
	}
}

func fakeJWT(claims string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(claims)) + ".sig"
}

func TestTheChatGPTSignInIsReadFromTheProxysFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "codex", "auth.json")
	now := time.UnixMilli(1_800_000_000_000)
	l := &ChatGPTLogin{Path: file, Now: func() time.Time { return now }}
	if _, ok := l.Account(); ok {
		t.Fatal("signed in without a file")
	}
	if _, err := l.Token(context.Background()); !errors.Is(err, ErrNoLogin) {
		t.Fatalf("token without a sign-in: %v", err)
	}
	write := func(v any) {
		_ = os.MkdirAll(filepath.Dir(file), 0o700)
		raw, _ := json.Marshal(v)
		if err := os.WriteFile(file, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	access := fakeJWT(`{"https://api.openai.com/profile":{"email":"me@example.com"}}`)
	// An access token that ran out is still a sign-in while the proxy has the refresh token to renew it.
	write(map[string]any{"access": access, "refresh": "r", "expires": now.UnixMilli() - 1})
	if who, ok := l.Account(); !ok || who != "me@example.com" {
		t.Fatalf("account %q %v", who, ok)
	}
	if tok, err := l.Token(context.Background()); err != nil || tok != ProxyToken {
		t.Fatalf("token %q %v", tok, err)
	}
	write(map[string]any{"access": access, "refresh": "", "expires": now.UnixMilli() - 1})
	if _, ok := l.Account(); ok {
		t.Fatal("a sign-in that can no longer be renewed")
	}
	write(map[string]any{"access": "opaque", "refresh": "r", "expires": 0})
	if who, ok := l.Account(); !ok || who != "" {
		t.Fatalf("a token that names no one: %q %v", who, ok)
	}

	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if got := ChatGPTAuthFile("/home/me", getenv); got != "/home/me/.config/claude-code-proxy/codex/auth.json" {
		t.Errorf("default %s", got)
	}
	env["XDG_CONFIG_HOME"] = "/x"
	if got := ChatGPTAuthFile("/home/me", getenv); got != "/x/claude-code-proxy/codex/auth.json" {
		t.Errorf("xdg %s", got)
	}
	env["CCP_CONFIG_DIR"] = "/c"
	if got := ChatGPTAuthFile("/home/me", getenv); got != "/c/codex/auth.json" {
		t.Errorf("override %s", got)
	}
}
