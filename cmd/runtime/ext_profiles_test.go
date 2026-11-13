package main

import (
	"testing"

	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/wire"
)

func TestExtProfileOfAProvider(t *testing.T) {
	p := extproviders.Provider{ID: "corp", Name: "Corp proxy", BaseURL: "https://llm.corp.example/anthropic", API: extproviders.APIAnthropic,
		KeyRef: "provider-corp-key", Models: []extproviders.Model{{ID: "corp-large", Label: "Large"}, {ID: "has space"}, {ID: "corp-small"}},
		Env: []extproviders.EnvVar{{Name: "ANTHROPIC_MODEL", Value: "corp-large"}, {Name: "ANTHROPIC_AUTH_TOKEN", Secret: true, Ref: "provider-corp-env-anthropic-auth-token"}}}
	pr, ok := extProfile(p)
	if !ok {
		t.Fatal("an Anthropic-compatible provider is a profile of Claude Code")
	}
	if pr.ID != "x-corp" || pr.Label != "Corp proxy" || pr.Via != wire.ProviderClaude || !pr.Custom || pr.KeyRef != "provider-corp-key" {
		t.Fatalf("profile: %+v", pr)
	}
	// The typed model is the only one offered, under the name the provider's list gives it.
	if len(pr.Models) != 1 || pr.Models[0].ID != "corp-large" || pr.Models[0].Label != "Large" || pr.Small != "corp-large" {
		t.Fatalf("models: %+v", pr.Models)
	}
	free := p
	free.Env = nil
	if fr, _ := extProfile(free); len(fr.Models) != 2 || fr.Models[1].Label != "corp-small" {
		t.Fatalf("models without a typed one: %+v", fr.Models)
	}
	free.Env = []extproviders.EnvVar{{Name: "ANTHROPIC_MODEL", Value: "corp-other"}}
	if fr, _ := extProfile(free); len(fr.Models) != 1 || fr.Models[0].Label != "corp-other" {
		t.Fatalf("a typed model the list does not have: %+v", fr.Models)
	}
	if pr.Env["ANTHROPIC_MODEL"] != "corp-large" || pr.SecretEnv["ANTHROPIC_AUTH_TOKEN"] != "provider-corp-env-anthropic-auth-token" || len(pr.Env) != 1 {
		t.Fatalf("env: %v, secret %v", pr.Env, pr.SecretEnv)
	}

	p.API = extproviders.APIOpenAI
	if _, ok := extProfile(p); ok {
		t.Fatal("Claude Code does not speak the OpenAI API")
	}
	long := "a-provider-with-a-very-long-id-0123"
	if id := extProfileID(long); !wire.ValidProfileID(id) || id == extProfileID(long+"x") {
		t.Fatalf("the id of a long provider: %q", id)
	}
}
