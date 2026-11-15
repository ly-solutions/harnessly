package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/settingsctl"
)

// mapSecrets is a safe of a test: names of the person's safe and vault: references alike.
type mapSecrets map[string]string

func (m mapSecrets) Put(name, value string) error { m[name] = value; return nil }
func (m mapSecrets) Reveal(name string) (string, error) {
	v, ok := m[name]
	if !ok {
		return "", errors.New("no secret")
	}
	return v, nil
}
func (m mapSecrets) Remove(name string) error { delete(m, name); return nil }
func (m mapSecrets) Has(name string) bool     { _, ok := m[name]; return ok }

// gatewayToken is a value only the safe has: joined from parts so that no scanner takes it for a key.
const gatewayToken = "gw-" + "token-0000-abcdef"

// A machine that was never set up by hand (a new profile, a second machine) gets a provider from the environment list
// and starts a Claude Code chat on it: the address and the variables come from the item, the token from the safe by
// the reference the item names. Nothing here depends on what this machine has.
func TestAProviderOfTheEnvironmentLaunchesAChatOnAFreshMachine(t *testing.T) {
	data := t.TempDir()
	sec := mapSecrets{"vault:shared/llm/gateway#token": gatewayToken}
	svc := &extproviders.Service{Store: &extproviders.Store{Dir: filepath.Join(data, "v2", "ext-providers")}, Secrets: sec}
	store := &providers.Store{Dir: filepath.Join(data, "providers"), Safe: sec.Reveal}
	t.Cleanup(func() { providers.SetCustom(nil) })

	source := json.RawMessage(`{"api":"anthropic","baseUrl":"https://llm.corp.example/anthropic","models":["corp-large","corp-small"],
		"env":["ANTHROPIC_MODEL=corp-large","NODE_EXTRA_CA_CERTS=/etc/ssl/corp-ca.pem"],
		"secretEnv":["ANTHROPIC_AUTH_TOKEN=vault:shared/llm/gateway#token"]}`)
	setup := setupProviders{svc: svc}
	changed, waiting, shared, err := setup.Apply("item-1", "Шлюз моделей", source)
	if err != nil || !changed || len(waiting) != 0 || len(shared) != 1 || shared[0] != "vault:shared/llm/gateway#token" {
		t.Fatalf("apply: %v %v %v %v", changed, waiting, shared, err)
	}
	if _, _, _, err := setup.Apply("item-2", "С токеном", json.RawMessage(`{"api":"anthropic","baseUrl":"https://x.example","env":["ANTHROPIC_AUTH_TOKEN=sk-live"]}`)); err == nil {
		t.Fatal("a source with a token typed as a value was taken")
	}

	syncExtProfiles(svc, store)
	id := extProfileID("env-item-1")
	if !store.Ready(id) {
		t.Fatal("a provider with a token of its own is ready without a key typed on this machine")
	}
	hooks := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(hooks, []byte(`{"hooks":{"Stop":[]},"env":{"HARNESSLY_DATA_DIR":"/data"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := store.SettingsFile(id, "corp-large", hooks)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"ANTHROPIC_BASE_URL":   "https://llm.corp.example/anthropic",
		"ANTHROPIC_MODEL":      "corp-large",
		"NODE_EXTRA_CA_CERTS":  "/etc/ssl/corp-ca.pem",
		"ANTHROPIC_AUTH_TOKEN": gatewayToken,
		"HARNESSLY_DATA_DIR":   "/data",
	} {
		if settings.Env[k] != want {
			t.Errorf("%s = %q, want %q", k, settings.Env[k], want)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the file with the token is not the person's alone: %v %v", info, err)
	}
	// The registry and what the page is told hold the reference, never the value.
	reg, _ := os.ReadFile(filepath.Join(data, "v2", "ext-providers", "providers.json"))
	views, _ := svc.List()
	page, _ := json.Marshal(views)
	if strings.Contains(string(reg), gatewayToken) || strings.Contains(string(page), gatewayToken) {
		t.Fatal("the token is in the registry or in an answer")
	}
	if len(views) != 1 || !views[0].Managed {
		t.Fatalf("views: %+v", views)
	}

	// The item left the list: the profile goes with it.
	if gone, err := setup.Prune(nil); err != nil || len(gone) != 1 {
		t.Fatalf("prune: %v %v", gone, err)
	}
	syncExtProfiles(svc, store)
	if _, ok := providers.Find(id); ok {
		t.Fatal("the profile of a provider that left the list stayed")
	}
}

// An agent sets up the person's own providers with `harnessly settings`: it reads the list, writes one account and
// names the secrets; it is refused a value of a token and a secret the safe does not have.
func TestAnAgentSetsUpAProviderThroughTheSettings(t *testing.T) {
	sec := mapSecrets{}
	allowed := map[string]string{}
	svc := &extproviders.Service{Store: &extproviders.Store{Dir: t.TempDir()}, Secrets: sec,
		Bound: func(name, origin string) bool { return allowed[name] == origin }}
	it := providersItem(svc)
	ctl := &settingsctl.Service{Items: []settingsctl.Item{it}}
	by := settingsctl.Actor{}

	account := `{"name":"Corp","api":"anthropic","baseUrl":"https://llm.corp.example/a","env":["ANTHROPIC_MODEL=corp-large"],"secretEnv":["ANTHROPIC_AUTH_TOKEN=corp-token"]}`
	_, err := ctl.Apply("providers", account, by, "cli", "по просьбе")
	if r := settingsctl.RefusalOf(err); r == nil || r.Reason != settingsctl.ReasonBadValue || !strings.Contains(r.Message, "harnessly secret ask") {
		t.Fatalf("without the secret: %+v", r)
	}
	leak := `{"name":"Corp","api":"anthropic","baseUrl":"https://llm.corp.example/a","env":["ANTHROPIC_AUTH_TOKEN=sk-live-123"]}`
	if _, err := ctl.Apply("providers", leak, by, "cli", ""); settingsctl.RefusalOf(err) == nil {
		t.Fatal("a token typed as a value was taken")
	}
	sec["corp-token"] = gatewayToken
	// The secret is there, but the person allowed it for no server: it goes nowhere.
	_, err = ctl.Apply("providers", account, by, "cli", "по просьбе")
	if r := settingsctl.RefusalOf(err); r == nil || !strings.Contains(r.Message, "--origin") {
		t.Fatalf("a secret that was not allowed for the server: %+v", r)
	}
	allowed["corp-token"] = "https://llm.corp.example"
	ch, err := ctl.Apply("providers", account, by, "cli", "по просьбе")
	if err != nil {
		t.Fatal(err)
	}
	// The journal keeps what the list was and became (names of secrets, no values), so that «Вернуть» works.
	if ch.Old != "[]" || !strings.Contains(ch.New, `"id":"corp"`) || strings.Contains(ch.New, gatewayToken) || ch.NewLabel != "добавить Corp → https://llm.corp.example/a; секреты: corp-token; переменные: ANTHROPIC_MODEL, ANTHROPIC_AUTH_TOKEN" {
		t.Fatalf("change: %+v", ch)
	}
	if _, err := ctl.Apply("providers", ch.Old, by, "revert", ""); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if list, _ := svc.Declared(); len(list) != 0 || sec["corp-token"] == "" {
		t.Fatalf("after the revert: %+v %v", list, sec)
	}
}
