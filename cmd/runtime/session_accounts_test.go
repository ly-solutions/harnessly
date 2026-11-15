package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// A chat on another sign-in of the Codex CLI is counted to that sign-in: under the name the person gave it, and for the
// hub as the account its own home is signed in with — the id of the account, nothing of its tokens.
func TestASessionOnAnotherCodexSignInIsThatAccounts(t *testing.T) {
	data, home := t.TempDir(), filepath.Join(t.TempDir(), "codex-work")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	token := "ey" + "J-not-a-real-token"
	auth := `{"auth_mode":"chatgpt","tokens":{"account_id":"acc-work-1","access_token":"` + token + `"}}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	providers.SetCodexLogins([]providers.Preset{{ID: codexLoginProfileID("work"), Label: "Codex рабочий", Vendor: wire.VendorGPT,
		Via: wire.ProviderCodex, BaseURL: "https://chatgpt.com/backend-api/codex", Host: "chatgpt.com",
		Models: []providers.Model{{ID: "gpt-test", Label: "GPT test"}}, Small: "gpt-test", Test: "gpt-test", Home: home}})
	t.Cleanup(func() { providers.SetCodexLogins(nil) })
	store := &providers.Store{Dir: filepath.Join(data, "providers")}

	noteSessionAccount(data, store)(wire.ProviderCodex, "sess-codex-1", codexLoginProfileID("work"))

	e, ok := sessionAccountsOf(data).Of("codex", "sess-codex-1")
	if !ok || e.Label != "Codex рабочий" || e.Ref != "codex:acc-work-1" || e.Host != "" {
		t.Fatalf("noted %+v %v", e, ok)
	}
	if got := accountOfSession(data)("codex", "sess-codex-1"); got != "Codex рабочий" {
		t.Fatalf("the report names the session %q", got)
	}
	raw, _ := os.ReadFile(filepath.Join(data, "v2", "session-accounts.jsonl"))
	if len(raw) == 0 || strings.Contains(string(raw), token) {
		t.Fatalf("the note is empty or holds the token")
	}
}
