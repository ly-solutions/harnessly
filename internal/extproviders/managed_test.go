package extproviders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sharedKey = "vault:shared/llm/gateway#key"

func gateway() ManagedSource {
	return ManagedSource{API: APIAnthropic, BaseURL: "https://llm.corp.example/anthropic/", Models: []string{"corp-large", "corp-small"},
		Key: sharedKey, Env: []string{"ANTHROPIC_MODEL=corp-large"}, SecretEnv: []string{"ANTHROPIC_AUTH_TOKEN=gateway-token"}}
}

func TestASourceNamesItsSecretsAndHoldsNone(t *testing.T) {
	src, err := gateway().Clean()
	if err != nil {
		t.Fatal(err)
	}
	// The same server by another path is still the provider.
	path := gateway()
	path.Env = append(path.Env, "ANTHROPIC_BASE_URL=https://llm.corp.example/v2")
	if _, err := path.Clean(); err != nil {
		t.Fatalf("another path of the same server: %v", err)
	}
	if src.BaseURL != "https://llm.corp.example/anthropic" || strings.Join(src.SecretRefs(), " ") != sharedKey+" gateway-token" {
		t.Fatalf("clean: %+v, refs %v", src, src.SecretRefs())
	}
	for name, c := range map[string]struct {
		edit   func(*ManagedSource)
		reason string
	}{
		"another server by a variable": {func(m *ManagedSource) { m.Env = []string{"ANTHROPIC_BASE_URL=https://collect.example/a"} }, ReasonEnvReserved},
		"a proxy in between":           {func(m *ManagedSource) { m.Env = []string{"https_proxy=http://collect.example:8080"} }, ReasonEnvReserved},
		"no check of who answers":      {func(m *ManagedSource) { m.Env = []string{"NODE_TLS_REJECT_UNAUTHORIZED=0"} }, ReasonEnvReserved},
		"another cloud":                {func(m *ManagedSource) { m.Env = []string{"CLAUDE_CODE_USE_BEDROCK=1"} }, ReasonEnvReserved},
		"what git runs":                {func(m *ManagedSource) { m.Env = []string{"GIT_SSH_COMMAND=/tmp/x"} }, ReasonEnvReserved},
		"what node loads":              {func(m *ManagedSource) { m.Env = []string{"NODE_OPTIONS=--require=/tmp/x.js"} }, ReasonEnvReserved},
		"a token typed as a value":     {func(m *ManagedSource) { m.Env = []string{"ANTHROPIC_AUTH_TOKEN=sk-live-123"} }, ReasonSecretInSource},
		"a key typed as a value":       {func(m *ManagedSource) { m.Env = []string{"OPENAI_API_KEY=sk-live-123"} }, ReasonSecretInSource},
		"the launch's own":             {func(m *ManagedSource) { m.Env = []string{"RUNTIME_CORPORATE_ENV=1"} }, ReasonEnvReserved},
		"the pane's own":               {func(m *ManagedSource) { m.SecretEnv = []string{"HARNESSLY_TOKEN=some-secret"} }, ReasonEnvReserved},
		"a secret that is a value":     {func(m *ManagedSource) { m.Key = "sk live 123" }, ReasonBadSecretRef},
		"a reference out of a folder":  {func(m *ManagedSource) { m.Key = "vault:shared/../other#key" }, ReasonBadSecretRef},
		"a variable twice":             {func(m *ManagedSource) { m.SecretEnv = []string{"ANTHROPIC_MODEL=gateway-token"} }, ReasonBadEnv},
		"no address":                   {func(m *ManagedSource) { m.BaseURL = "" }, ReasonBadURL},
		"another protocol":             {func(m *ManagedSource) { m.API = "grpc" }, ReasonBadAPI},
	} {
		m := gateway()
		c.edit(&m)
		if _, err := m.Clean(); ReasonOf(err) != c.reason {
			t.Errorf("%s: %v, want %s", name, err, c.reason)
		}
	}
}

func TestAProviderOfTheEnvironmentIsKeptBesideThePersonsOwn(t *testing.T) {
	svc, sec := newService(t)
	own, err := svc.Create(Input{Name: "Шлюз", BaseURL: "https://api.deepseek.com/v1", API: APIOpenAI, Key: "own-key"})
	if err != nil {
		t.Fatal(err)
	}
	sec["gateway-token"] = "token-value"
	sec[sharedKey] = "shared-key-value"
	bound := map[string]string{}
	svc.Bound = func(name, origin string) bool { return bound[name] == origin }

	// The person has a secret of that name, but did not allow it for this server: the provider is kept without it,
	// and nothing of the person's goes to a server an item of the environment named.
	changed, waiting, err := svc.ApplyManaged("item-1", "Шлюз", gateway())
	if err != nil || !changed || len(waiting) != 1 || waiting[0] != "gateway-token" {
		t.Fatalf("apply: %v %v %v", changed, waiting, err)
	}
	if held, _ := svc.Get(managedID("item-1")); len(held.Env) != 1 || held.Env[0].Secret {
		t.Fatalf("kept with a secret that was not allowed: %+v", held.Env)
	}
	bound["gateway-token"] = "https://llm.corp.example"
	if changed, waiting, err = svc.ApplyManaged("item-1", "Шлюз", gateway()); err != nil || !changed || len(waiting) != 0 {
		t.Fatalf("after the person allowed it: %v %v %v", changed, waiting, err)
	}
	if again, _, err := svc.ApplyManaged("item-1", "Шлюз", gateway()); err != nil || again {
		t.Fatalf("the same source is no change: %v %v", again, err)
	}
	p, err := svc.Get(managedID("item-1"))
	if err != nil {
		t.Fatal(err)
	}
	// The person's account keeps its name; the environment's one is told apart.
	if p.Name != "Шлюз 2" || p.Origin != "item-1" || p.KeyRef != sharedKey || len(p.Models) != 2 {
		t.Fatalf("kept: %+v", p)
	}
	v := svc.ViewOf(p)
	if !v.Managed || !v.HasKey || len(v.Env) != 2 || !v.Env[1].Secret || !v.Env[1].Set || v.Env[1].Value != "" {
		t.Fatalf("view: %+v", v)
	}
	if key, err := svc.Key(p); err != nil || key != "shared-key-value" {
		t.Fatalf("the organization's key is read by its reference: %q %v", key, err)
	}
	// Neither the file nor an answer holds a value of a secret.
	raw, _ := os.ReadFile(filepath.Join(svc.Store.Dir, "providers.json"))
	if strings.Contains(string(raw), "token-value") || strings.Contains(string(raw), "shared-key-value") {
		t.Fatalf("a secret in the file: %s", raw)
	}

	// It is changed in the hub, not here; which account is the main one stays the person's choice.
	name, yes := "Мой", true
	if _, err := svc.Update(p.ID, Patch{Name: &name}); ReasonOf(err) != ReasonManaged {
		t.Fatalf("rename: %v", err)
	}
	if err := svc.Remove(p.ID); ReasonOf(err) != ReasonManaged {
		t.Fatalf("remove: %v", err)
	}
	if _, err := svc.Update(p.ID, Patch{Default: &yes}); err != nil {
		t.Fatalf("default: %v", err)
	}

	// A new source of the item replaces the old one.
	next := gateway()
	next.BaseURL, next.Models = "https://llm2.corp.example/anthropic", nil
	// The server moved: the secret the person allowed for the old one does not follow it.
	if changed, waiting, err := svc.ApplyManaged("item-1", "Шлюз", next); err != nil || !changed || len(waiting) != 1 {
		t.Fatalf("update: %v %v %v", changed, waiting, err)
	}
	if p, _ = svc.Get(p.ID); p.BaseURL != "https://llm2.corp.example/anthropic" || len(p.Models) != 0 || len(p.Env) != 1 {
		t.Fatalf("updated: %+v", p)
	}

	// The item left the list: the provider goes, the secrets stay (they are the person's and the organization's).
	gone, err := svc.PruneManaged([]string{"item-2"})
	if err != nil || len(gone) != 1 || gone[0] != "item-1" {
		t.Fatalf("prune: %v %v", gone, err)
	}
	if svc.HasManaged("item-1") || sec["gateway-token"] == "" || sec[sharedKey] == "" {
		t.Fatalf("after the prune: %v", sec)
	}
	if _, err := svc.Get(own.ID); err != nil || sec["provider-"+own.ID+"-key"] != "own-key" {
		t.Fatalf("the person's own account: %v %v", err, sec)
	}
}

func TestAnAgentDeclaresAccountsByTheNamesOfSecrets(t *testing.T) {
	svc, sec := newService(t)
	typed, err := svc.Create(Input{Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", API: APIOpenAI, Preset: "deepseek", Key: "typed-key"})
	if err != nil {
		t.Fatal(err)
	}
	sec[sharedKey] = "shared"
	bound := map[string]string{}
	svc.Bound = func(name, origin string) bool { return bound[name] == origin }
	if _, _, err := svc.ApplyManaged("item-1", "Окружение", gateway()); err != nil {
		t.Fatal(err)
	}
	corp := Declared{Name: "Corp", ManagedSource: ManagedSource{API: APIAnthropic, BaseURL: "https://llm.corp.example/a", Key: "corp-key",
		Env: []string{"ANTHROPIC_MODEL=corp-large"}, SecretEnv: []string{"ANTHROPIC_AUTH_TOKEN=corp-token"}}}

	// The secrets come first: an agent asks the person for them, and cannot write a value instead.
	if err := svc.Declare([]Declared{corp}, false, false); ReasonOf(err) != ReasonNoSecret {
		t.Fatalf("without the secrets: %v", err)
	}
	leak := corp
	leak.Env = []string{"ANTHROPIC_AUTH_TOKEN=sk-live-123"}
	if err := svc.Declare([]Declared{leak}, false, false); ReasonOf(err) != ReasonSecretInSource {
		t.Fatalf("a token as a value: %v", err)
	}
	sec["corp-key"], sec["corp-token"], sec["github-token"] = "k", "tok", "gh"
	// A secret the person has is not an agent's to send anywhere: only one allowed for this very server is taken.
	if err := svc.Declare([]Declared{corp}, false, false); ReasonOf(err) != ReasonSecretUnbound {
		t.Fatalf("secrets that were not allowed for the server: %v", err)
	}
	bound["corp-key"], bound["corp-token"] = "https://llm.corp.example", "https://llm.corp.example"
	for name, edit := range map[string]func(*Declared){
		"another secret of the safe":   func(d *Declared) { d.Key = "github-token" },
		"another server":               func(d *Declared) { d.BaseURL = "https://collect.example/a" },
		"the organization's secret":    func(d *Declared) { d.Key = sharedKey },
		"another account's own secret": func(d *Declared) { d.Key = "provider-" + typed.ID + "-key" },
	} {
		theft := corp
		edit(&theft)
		if err := svc.Declare([]Declared{theft}, false, false); ReasonOf(err) != ReasonSecretUnbound {
			t.Errorf("%s: %v", name, err)
		}
	}
	// An account's own key does not follow the account to another server either.
	moved := Declared{ID: typed.ID, Name: "DeepSeek", ManagedSource: ManagedSource{API: APIOpenAI, BaseURL: "https://collect.example/v1", Key: "provider-" + typed.ID + "-key"}}
	if err := svc.Declare([]Declared{moved}, false, false); ReasonOf(err) != ReasonSecretUnbound {
		t.Fatalf("a typed key sent to another server: %v", err)
	}
	if err := svc.Declare([]Declared{corp}, false, true); err != nil {
		t.Fatalf("a check: %v", err)
	}
	if list, _ := svc.Declared(); len(list) != 1 {
		t.Fatalf("a check wrote: %+v", list)
	}
	if err := svc.Declare([]Declared{corp}, false, false); err != nil {
		t.Fatal(err)
	}
	list, err := svc.Declared()
	if err != nil || len(list) != 2 || list[1].ID != "corp" || list[1].Key != "corp-key" || list[0].Key != "provider-"+typed.ID+"-key" {
		t.Fatalf("declared (the environment's one is not listed): %+v %v", list, err)
	}
	if p, _ := svc.Get("corp"); !p.ClaudeCLI() || len(p.Env) != 2 || p.Env[1].Ref != "corp-token" {
		t.Fatalf("kept: %+v", p)
	}

	// The environment's provider is not an agent's to write.
	if err := svc.Declare([]Declared{{ID: managedID("item-1"), Remove: true}}, false, false); ReasonOf(err) != ReasonManaged {
		t.Fatalf("the environment's: %v", err)
	}
	// A key typed on the Settings screen over a named one goes under the registry's own name: the named secret is not overwritten.
	newKey := "typed-over"
	if _, err := svc.Update("corp", Patch{Key: &newKey}); err != nil {
		t.Fatal(err)
	}
	if sec["corp-key"] != "k" || sec["provider-corp-key"] != "typed-over" {
		t.Fatalf("a named secret was overwritten: %v", sec)
	}
	// Removing an account takes its own secrets and leaves the named ones.
	if err := svc.Declare([]Declared{{ID: "corp", Remove: true}}, false, false); err != nil {
		t.Fatal(err)
	}
	if _, has := sec["provider-corp-key"]; has || sec["corp-token"] != "tok" || sec["corp-key"] != "k" {
		t.Fatalf("secrets after a removal: %v", sec)
	}
	// The whole list: the person's accounts become exactly these; the environment's one stays.
	if err := svc.Declare([]Declared{}, true, false); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.Declared(); len(list) != 0 || !svc.HasManaged("item-1") {
		t.Fatalf("after an empty list: %+v", list)
	}
	if _, has := sec["provider-"+typed.ID+"-key"]; has {
		t.Fatalf("the typed key of a removed account stayed: %v", sec)
	}
}
