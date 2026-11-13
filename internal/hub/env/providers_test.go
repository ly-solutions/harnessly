package env_test

import (
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/env"
)

func TestCleanSourceOfAProvider(t *testing.T) {
	good := map[string]any{"api": "anthropic", "baseUrl": "https://llm.corp.example/anthropic/", "models": []string{"corp-large"},
		"key": "vault:shared/llm/gateway#key", "env": []string{"ANTHROPIC_MODEL=corp-large"},
		"secretEnv": []string{"ANTHROPIC_AUTH_TOKEN=gateway-token"}}
	out, err := env.CleanSource("provider", raw(good))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); !strings.Contains(s, `"baseUrl":"https://llm.corp.example/anthropic"`) || !strings.Contains(s, `"key":"vault:shared/llm/gateway#key"`) {
		t.Fatalf("canonical: %s", s)
	}
	for name, edit := range map[string]func(map[string]any){
		"a token as a value":     func(m map[string]any) { m["env"] = []string{"ANTHROPIC_AUTH_TOKEN=sk-live-123"} },
		"a key that is a value":  func(m map[string]any) { m["key"] = "sk live 123" },
		"the launch's variable":  func(m map[string]any) { m["env"] = []string{"RUNTIME_CORPORATE_ENV=1"} },
		"no address":             func(m map[string]any) { delete(m, "baseUrl") },
		"another kind's key":     func(m map[string]any) { m["run"] = "bash x" },
		"an unknown kind of API": func(m map[string]any) { m["api"] = "grpc" },
	} {
		src := map[string]any{}
		for k, v := range good {
			src[k] = v
		}
		edit(src)
		if _, err := env.CleanSource("provider", raw(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Only an organization's admin adds a provider: a team or a tenant is refused.
	if err := env.CheckConsent("provider", out); err == nil {
		t.Fatal("a provider of a team's own was accepted")
	}
}
