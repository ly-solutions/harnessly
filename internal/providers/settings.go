package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// OAuthStub is the Claude Code OAuth token of a profile's chat. Claude Code takes the key of apiKeyHelper before an
// OAuth token, so the stub changes nothing for the chat itself; it is for a `claude` the chat's agent starts from Bash,
// which inherits the environment but not the helper's key: without a token of its own that one would fall back to the
// person's subscription and send it to the vendor.
const OAuthStub = "harnessly-no-oauth"

var (
	// ErrNotClaude: only a profile that runs through Claude Code has a settings file.
	ErrNotClaude = errors.New("providers: the profile does not run through Claude Code")
	// ErrUnknownModel: the model is none of the preset's.
	ErrUnknownModel = errors.New("providers: the preset has no such model")

	slugRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// ProxyToken is the token Claude Code sends the local proxy of a Proxied profile: Claude Code wants one, and the proxy
// reads none (it holds the vendor's sign-in itself, and takes no credential from a client). What keeps the chat from
// another listener is that its address is that of the runtime's own proxy (Proxy.Ensure).
const ProxyToken = "unused"

// ClaudeEnv is the environment block of a profile's Claude Code settings for a chat on model, its requests going to
// base. Every variable that could carry an Anthropic credential to the vendor is blanked or stubbed; the vendor's key is
// not here, it is read from a file by apiKeyHelper. A Proxied profile has no key: its token is ProxyToken.
func (p Preset) ClaudeEnv(model Model, base string) map[string]string {
	env := map[string]string{
		"ANTHROPIC_BASE_URL":                       strings.TrimRight(base, "/"),
		"ANTHROPIC_MODEL":                          model.ID,
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             model.ID,
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           model.ID,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            p.Small,
		"CLAUDE_CODE_SUBAGENT_MODEL":               model.ID,
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS":           strconv.Itoa(model.Window),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_AUTH_TOKEN":                     "",
		"ANTHROPIC_API_KEY":                        "",
		"CLAUDE_CODE_OAUTH_TOKEN":                  OAuthStub,
	}
	if p.Proxied {
		env["ANTHROPIC_AUTH_TOKEN"] = ProxyToken
	}
	for k, v := range p.Env {
		env[k] = v
	}
	return env
}

// SettingsFile writes (rewritten only when its content changes) the `--settings` file of a Claude Code chat on a
// profile and a model, and gives its path. It is a copy of the hooks settings file (hooksPath) with the profile's
// environment and an apiKeyHelper that reads the key from its file (none for a Proxied profile, which has no key); a
// variable the hooks file already had in `env` is kept unless the profile sets it. The key itself is never written here. The file is named after the profile and the
// model, because the context window is the model's. A Proxied profile's proxy is started first (Proxy.Ensure): the
// address in the file is its own.
func (s *Store) SettingsFile(id, model, hooksPath string) (string, error) {
	p, ok := Find(id)
	if !ok {
		return "", fmt.Errorf("providers: no profile %q", id)
	}
	if p.Via != "claude" {
		return "", ErrNotClaude
	}
	m, ok := p.ModelOf(model)
	if !ok {
		return "", ErrUnknownModel
	}
	base := p.BaseURL
	if p.Proxied {
		if s.Proxy == nil {
			return "", ErrProxyMissing
		}
		ctx, cancel := context.WithTimeout(context.Background(), ProxyStartTimeout+5*time.Second)
		defer cancel()
		var err error
		if base, err = s.Proxy.Ensure(ctx); err != nil {
			return "", err
		}
	}
	raw, err := os.ReadFile(hooksPath)
	if err != nil {
		return "", err
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		return "", err
	}
	env := map[string]any{}
	if have, ok := settings["env"].(map[string]any); ok {
		for k, v := range have {
			env[k] = v
		}
	}
	for k, v := range p.ClaudeEnv(m, base) {
		env[k] = v
	}
	settings["env"] = env
	if p.Proxied {
		delete(settings, "apiKeyHelper")
	} else {
		settings["apiKeyHelper"] = "cat " + shellQuote(s.KeyFile(id))
	}
	body, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	target := filepath.Join(s.Dir, id+"."+strings.Trim(slugRe.ReplaceAllString(strings.ToLower(model), "-"), "-")+".settings.json")
	if have, err := os.ReadFile(target); err == nil && string(have) == string(body) {
		return target, nil
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return target, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
