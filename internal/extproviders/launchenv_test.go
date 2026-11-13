package extproviders

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func str(s string) *string { return &s }

func TestLaunchVariables(t *testing.T) {
	svc, sec := newService(t)
	p, err := svc.Create(Input{Name: "Corp", BaseURL: "https://llm.corp.example/anthropic", API: APIAnthropic, Preset: PresetCustom})
	if err != nil {
		t.Fatal(err)
	}
	const token = "corp-token-value"
	env := []EnvInput{
		{Name: "ANTHROPIC_BASE_URL", Value: str("https://proxy.corp.example/v2")},
		{Name: "ANTHROPIC_MODEL", Value: str("  ")}, // left empty: as the launch makes it
		{Name: "ANTHROPIC_AUTH_TOKEN", Value: str(token)},
		{Name: "CORP_FLAG", Value: str("hidden"), Secret: true},
		{Name: "NODE_EXTRA_CA_CERTS", Value: str("/etc/ssl/corp-ca.pem")},
	}
	view, err := svc.Update(p.ID, Patch{Env: &env})
	if err != nil {
		t.Fatal(err)
	}
	want := []EnvView{
		{Name: "ANTHROPIC_BASE_URL", Value: "https://proxy.corp.example/v2", Set: true},
		{Name: "ANTHROPIC_AUTH_TOKEN", Secret: true, Set: true}, // a credential by its name, whatever the form said
		{Name: "CORP_FLAG", Secret: true, Set: true},
		{Name: "NODE_EXTRA_CA_CERTS", Value: "/etc/ssl/corp-ca.pem", Set: true},
	}
	if len(view.Env) != len(want) {
		t.Fatalf("env: %+v", view.Env)
	}
	for i := range want {
		if view.Env[i] != want[i] {
			t.Errorf("env[%d] = %+v, want %+v", i, view.Env[i], want[i])
		}
	}
	// The values of the secret ones are in the safe and nowhere else.
	if sec["provider-corp-env-anthropic-auth-token"] != token || sec["provider-corp-env-corp-flag"] != "hidden" {
		t.Fatalf("the safe: %v", sec)
	}
	file, _ := os.ReadFile(filepath.Join(svc.Store.Dir, "providers.json"))
	answer, _ := json.Marshal(view)
	for name, raw := range map[string][]byte{"file": file, "answer": answer} {
		if strings.Contains(string(raw), token) || strings.Contains(string(raw), "hidden") {
			t.Errorf("the %s holds a secret value", name)
		}
	}

	// A secret sent without a value keeps the one the safe has; one that is left out of the list is removed from it.
	keep := []EnvInput{{Name: "ANTHROPIC_AUTH_TOKEN", Secret: true}}
	if view, err = svc.Update(p.ID, Patch{Env: &keep}); err != nil || len(view.Env) != 1 || !view.Env[0].Set {
		t.Fatalf("keep: %+v %v", view.Env, err)
	}
	if sec["provider-corp-env-anthropic-auth-token"] != token {
		t.Fatal("the kept secret changed")
	}
	if _, has := sec["provider-corp-env-corp-flag"]; has {
		t.Fatal("a removed variable left its secret in the safe")
	}
	// The provider removed: nothing of it is left in the safe.
	if err := svc.Remove(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(sec) != 0 {
		t.Fatalf("the safe after the removal: %v", sec)
	}
}

func TestLaunchVariablesRefused(t *testing.T) {
	svc, sec := newService(t)
	p, _ := svc.Create(Input{Name: "Corp", BaseURL: "https://llm.corp.example", API: APIAnthropic, Preset: PresetCustom})
	for name, c := range map[string]struct {
		env    []EnvInput
		reason string
	}{
		"the shell's own":       {[]EnvInput{{Name: "PATH", Value: str("/tmp")}}, ReasonEnvReserved},
		"the pane's":            {[]EnvInput{{Name: "TMUX_PANE", Value: str("1")}}, ReasonEnvReserved},
		"harnessly's":           {[]EnvInput{{Name: "HARNESSLY_DATA_DIR", Value: str("/x")}}, ReasonEnvReserved},
		"harnessly's, any case": {[]EnvInput{{Name: "harnessly_token", Value: str("x")}}, ReasonEnvReserved},
		"the agent's home":      {[]EnvInput{{Name: "CLAUDE_CONFIG_DIR", Value: str("/x")}}, ReasonEnvReserved},
		"the session's":         {[]EnvInput{{Name: "CLAUDE_CODE_SESSION_ID", Value: str("x")}}, ReasonEnvReserved},
		"a preload":             {[]EnvInput{{Name: "LD_PRELOAD", Value: str("/x.so")}}, ReasonEnvReserved},
		"the runtime's":         {[]EnvInput{{Name: "RUNTIME_CORPORATE_ENV", Value: str("on")}}, ReasonEnvReserved},
		"no identifier":         {[]EnvInput{{Name: "A-B", Value: str("x")}}, ReasonBadEnv},
		"a command in a name":   {[]EnvInput{{Name: "A=1;rm", Value: str("x")}}, ReasonBadEnv},
		"twice":                 {[]EnvInput{{Name: "A", Value: str("1")}, {Name: "A", Value: str("2")}}, ReasonBadEnv},
		"two lines":             {[]EnvInput{{Name: "A", Value: str("1\n2")}}, ReasonBadEnv},
		"a secret without one":  {[]EnvInput{{Name: "ANTHROPIC_API_KEY", Secret: true}}, ReasonBadEnv},
	} {
		env := c.env
		if _, err := svc.Update(p.ID, Patch{Env: &env}); ReasonOf(err) != c.reason {
			t.Errorf("%s: %v, want %s", name, err, c.reason)
		}
	}
	// A list that is refused changes nothing in the safe.
	bad := []EnvInput{{Name: "ANTHROPIC_AUTH_TOKEN", Value: str("typed")}, {Name: "PATH", Value: str("/tmp")}}
	if _, err := svc.Update(p.ID, Patch{Env: &bad}); ReasonOf(err) != ReasonEnvReserved || len(sec) != 0 {
		t.Fatalf("a refused list: %v, safe %v", err, sec)
	}
}
