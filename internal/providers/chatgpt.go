package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ChatGPTAuthFile is where the proxy keeps its ChatGPT sign-in for a user's home: <config>/codex/auth.json, the config
// folder being $CCP_CONFIG_DIR, else $XDG_CONFIG_HOME/claude-code-proxy, else ~/.config/claude-code-proxy. (On macOS
// the proxy keeps it in the Keychain unless CCP_CONFIG_DIR is set, and the file is not there.)
func ChatGPTAuthFile(home string, getenv func(string) string) string {
	if dir := getenv("CCP_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "codex", "auth.json")
	}
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, ProxyName, "codex", "auth.json")
}

// ChatGPTLogin is the proxy's sign-in of a ChatGPT account. The runtime reads the file and never writes it: the proxy
// renews the token itself, so a sign-in with a refresh token stays usable after its access token runs out. The token
// a chat sends is ProxyToken; the proxy puts the real one on the request.
type ChatGPTLogin struct {
	// Path is the proxy's codex/auth.json (ChatGPTAuthFile).
	Path string
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

type chatgptAuth struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	// Expires is when the access token runs out, in Unix milliseconds.
	Expires int64 `json:"expires"`
}

func (c *ChatGPTLogin) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *ChatGPTLogin) Account() (string, bool) {
	raw, err := os.ReadFile(c.Path)
	if err != nil {
		return "", false
	}
	var a chatgptAuth
	if json.Unmarshal(raw, &a) != nil || a.Access == "" {
		return "", false
	}
	if a.Refresh == "" && a.Expires <= c.now().UnixMilli() {
		return "", false
	}
	return emailOf(a.Access), true
}

func (c *ChatGPTLogin) Token(context.Context) (string, error) {
	if _, ok := c.Account(); !ok {
		return "", ErrNoLogin
	}
	return ProxyToken, nil
}

// emailOf is the email an OpenAI access token names (its profile claim); "" when it names none. The token is only
// read, not verified: it says whom the sign-in is of to the person, and authorises nothing here.
func emailOf(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims struct {
		Email   string `json:"email"`
		Profile struct {
			Email string `json:"email"`
		} `json:"https://api.openai.com/profile"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return ""
	}
	if claims.Profile.Email != "" {
		return claims.Profile.Email
	}
	return claims.Email
}
