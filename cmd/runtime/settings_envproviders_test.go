package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/extproviders"
	"github.com/ly-solutions/harnessly/internal/hubproto"
	"github.com/ly-solutions/harnessly/internal/settingsctl"
)

func TestTheOrganizationsProvidersAreProposedToTheHub(t *testing.T) {
	svc := &extproviders.Service{Store: &extproviders.Store{Dir: t.TempDir()}}
	var asked []hubproto.EnvProposal
	answer := hubproto.EnvProposalResult{Status: 200}
	var down error
	it := envProvidersItem(svc, func(p hubproto.EnvProposal) (hubproto.EnvProposalResult, error) {
		asked = append(asked, p)
		return answer, down
	})
	good := `{"name":"Шлюз","api":"anthropic","baseUrl":"https://llm.corp.example/anthropic","key":"gateway-key","secretEnv":["ANTHROPIC_AUTH_TOKEN=vault:team/llm#token"],"env":["ANTHROPIC_MODEL=corp-large"]}`

	// What cannot be a provider is refused here, in words, and the hub is not asked.
	for _, bad := range []string{`not json`, `{"api":"anthropic","baseUrl":"https://llm.corp.example"}`, `{"name":"X","api":"anthropic","baseUrl":"ftp://x"}`,
		`{"name":"X","api":"anthropic","baseUrl":"https://llm.corp.example","env":["HTTPS_PROXY=http://elsewhere.example"]}`} {
		if it.Free(bad) || it.Why(bad) == "" {
			t.Errorf("%s: taken", bad)
		}
	}
	if len(asked) != 0 {
		t.Fatalf("the hub was asked about a value that is not a provider: %+v", asked)
	}

	// The check asks the hub once, changing nothing; its refusal is said in its words.
	answer = hubproto.EnvProposalResult{Status: 403, Message: "У владельца этой машины нет права менять окружение организации"}
	if it.Free(good) || !strings.Contains(it.Why(good), "нет права") {
		t.Fatal("a refusal of the hub was not said")
	}
	if len(asked) != 1 || !asked[0].Check || asked[0].Provider.Name != "Шлюз" {
		t.Fatalf("%+v", asked)
	}

	// The card says whom the change is for, where the secrets go and which variables a launch gets, and no value.
	label := it.Label(good)
	for _, want := range []string{"предложить админу организации", "Шлюз → https://llm.corp.example/anthropic", "gateway-key", "vault:team/llm#token", "ANTHROPIC_MODEL"} {
		if !strings.Contains(label, want) {
			t.Errorf("the card lacks %q: %s", want, label)
		}
	}
	if strings.Contains(label, "corp-large") {
		t.Errorf("the card shows a variable's value: %s", label)
	}

	// The change itself goes with the chat it comes from; what the hub says on a refusal is the error's words.
	asked, answer = nil, hubproto.EnvProposalResult{Status: 202, ProposalID: "p1"}
	by := settingsctl.Actor{Kind: settingsctl.ByAgent, Conv: "tab-1", Title: "Чат"}
	if note, err := it.SetBy(good, by); err != nil || !strings.Contains(note, "ждёт админа организации") {
		t.Fatalf("%q %v", note, err)
	}
	if len(asked) != 1 || asked[0].Check || asked[0].Conv != "tab-1" || asked[0].ConvTitle != "Чат" || !strings.Contains(string(asked[0].Provider.Source), `"key":"gateway-key"`) {
		t.Fatalf("%+v", asked)
	}
	answer = hubproto.EnvProposalResult{Status: 403, Message: "нет права"}
	var said *settingsctl.Said
	if _, err := it.SetBy(good, by); !errors.As(err, &said) || said.Message != "нет права" {
		t.Fatalf("%v", err)
	}
	// A hub that says «done» keeps nothing by this protocol: it is not taken for a yes.
	answer = hubproto.EnvProposalResult{Status: 200}
	if _, err := it.SetBy(good, by); err == nil {
		t.Fatal("an answer other than «waits» was taken for a kept proposal")
	}
	down = errors.New("no agent")
	if _, err := it.SetBy(good, by); !errors.As(err, &said) || !strings.Contains(said.Message, "Хаб не ответил") {
		t.Fatalf("%v", err)
	}
	if _, err := it.SetBy(`{"name":"Шлюз","remove":true}`, by); err == nil {
		t.Fatal("a hub that is down took a removal")
	}

	// What it reads is the environment's providers of this machine, never the person's own.
	if v, err := it.Get(); err != nil || v != "[]" {
		t.Fatalf("%q %v", v, err)
	}
}
