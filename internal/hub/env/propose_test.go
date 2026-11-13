package env_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

func (a acme) propose(t *testing.T, owner string, p hubproto.EnvProposal) hubproto.EnvProposalResult {
	t.Helper()
	p.RequestID = "r1"
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return a.s.Hub.Env.Proposed(context.Background(), store.Machine{ID: "m1", Name: "ноутбук", SpaceID: a.tenant, CreatedBy: owner}, raw)
}

func (a acme) providers(t *testing.T) (items []string, pinned int) {
	t.Helper()
	rs := a.layers(t, a.anna, "")
	for _, l := range rs.Layers {
		for _, it := range l.Items {
			if it.Kind == "provider" {
				items = append(items, it.Name)
				if l.Layer == "org" && it.Pinned {
					pinned++
				}
			}
		}
	}
	return items, pinned
}

type proposalRow struct {
	ID, Change, Name, Hint, Machine, Owner, Conv, ConvTitle string
	Source, Before                                          map[string]any
	CreatedAt, ExpiresAt                                    int64
}

func (a acme) waiting(t *testing.T) []proposalRow {
	t.Helper()
	var out struct{ Proposals []proposalRow }
	if r := a.anna.Do("GET", a.url("/env/proposals"), nil); r.Code != 200 {
		t.Fatalf("proposals: %s", r)
	} else {
		r.JSON(&out)
	}
	return out.Proposals
}

func TestAnAgentsProposalChangesNothingUntilAnAdministratorAppliesIt(t *testing.T) {
	a := newAcme(t)
	src := json.RawMessage(`{"api":"anthropic","baseUrl":"https://llm.corp.example/anthropic","key":"gateway-key","env":["ANTHROPIC_MODEL=corp-large"]}`)
	gateway := hubproto.EnvProviderProposal{Name: "Шлюз", Hint: "Модели через общий шлюз", Source: src}

	// Whoever the machine belongs to — the organization's admin too — its proposal is only kept: a check keeps nothing,
	// the proposal itself is answered «waits» and the environment stays as it was.
	if res := a.propose(t, a.anna.ID, hubproto.EnvProposal{Check: true, Provider: gateway}); res.Status != 200 {
		t.Fatalf("check: %+v", res)
	}
	if got := a.waiting(t); len(got) != 0 {
		t.Fatalf("a check kept a proposal: %+v", got)
	}
	for _, who := range []string{a.anna.ID, a.dina.ID} {
		res := a.propose(t, who, hubproto.EnvProposal{Conv: "tab-1", ConvTitle: "Настройка шлюза", Provider: gateway})
		if res.Status != 202 || res.ProposalID == "" || !strings.Contains(res.Message, "админу организации") || res.RequestID != "r1" {
			t.Fatalf("%s: %+v", who, res)
		}
	}
	if items, _ := a.providers(t); len(items) != 0 {
		t.Fatalf("a proposal changed the environment: %v", items)
	}

	// The same machine proposing the same name again replaces its proposal: one waits, with what changes, where it
	// came from and when its term ends.
	got := a.waiting(t)
	if len(got) != 1 {
		t.Fatalf("waiting: %+v", got)
	}
	one := got[0]
	if one.Change != "add" || one.Name != "Шлюз" || one.Machine != "ноутбук" || one.ConvTitle != "Настройка шлюза" || one.Conv != "tab-1" || one.Owner == "" ||
		one.Source["baseUrl"] != "https://llm.corp.example/anthropic" || one.Source["key"] != "gateway-key" || one.ExpiresAt <= one.CreatedAt {
		t.Fatalf("the proposal as shown: %+v", one)
	}

	// Only an administrator of the organization sees and decides them.
	for _, r := range []int{
		a.dina.Do("GET", a.url("/env/proposals"), nil).Code,
		a.dina.Do("POST", a.url("/env/proposals/"+one.ID+"/apply"), map[string]any{}).Code,
		a.dina.Do("POST", a.url("/env/proposals/"+one.ID+"/decline"), map[string]any{}).Code,
		a.mallory.Do("POST", a.url("/env/proposals/"+one.ID+"/apply"), map[string]any{}).Code,
	} {
		if r != 403 && r != 404 {
			t.Fatalf("someone who does not administer the organization got %d", r)
		}
	}
	if items, _ := a.providers(t); len(items) != 0 || len(a.waiting(t)) != 1 {
		t.Fatalf("a refused decision changed something: %v", items)
	}

	// The administrator applies it under their own session: the item is added and pinned in the organization, the
	// proposal no longer waits and cannot be decided twice.
	var applied struct{ ItemID string }
	if r := a.anna.Do("POST", a.url("/env/proposals/"+one.ID+"/apply"), map[string]any{}); r.Code != 200 {
		t.Fatalf("apply: %s", r)
	} else {
		r.JSON(&applied)
	}
	if items, pinned := a.providers(t); len(items) != 1 || items[0] != "Шлюз" || pinned != 1 || applied.ItemID == "" {
		t.Fatalf("after the apply: %v, %d pinned", items, pinned)
	}
	if got := a.waiting(t); len(got) != 0 {
		t.Fatalf("an applied proposal still waits: %+v", got)
	}
	for _, act := range []string{"apply", "decline"} {
		if r := a.anna.Do("POST", a.url("/env/proposals/"+one.ID+"/"+act), map[string]any{}); r.Code != 409 {
			t.Fatalf("%s twice: %s", act, r)
		}
	}

	// The journal names the administrator as the author, says it was an agent's proposal, from which chat and machine,
	// and names no secret.
	chs, err := a.s.DB.S().ChangesOn(context.Background(), a.org, []store.ChangeScope{{Kind: store.ScopeOrg, ID: a.org}}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var said bool
	for _, c := range chs {
		if strings.Contains(c.Title, "по предложению агента из чата «Настройка шлюза» на машине «ноутбук»") && c.AuthorID == a.anna.ID {
			said = true
		}
		if strings.Contains(c.Title, "gateway-key") || strings.Contains(c.Title, "vault:") {
			t.Errorf("a secret's name in the journal: %s", c.Title)
		}
	}
	if !said {
		t.Errorf("the journal does not say the change was an agent's proposal: %+v", chs)
	}

	// A proposal for a name the organization has is a change of that item, shown with what it is now; declined, it
	// changes nothing.
	gateway.Source = json.RawMessage(`{"api":"anthropic","baseUrl":"https://llm2.corp.example/anthropic","key":"vault:team/llm#key"}`)
	if res := a.propose(t, a.dina.ID, hubproto.EnvProposal{Provider: gateway}); res.Status != 202 {
		t.Fatalf("change: %+v", res)
	}
	got = a.waiting(t)
	if len(got) != 1 || got[0].Change != "update" || got[0].Before["baseUrl"] != "https://llm.corp.example/anthropic" || got[0].Source["baseUrl"] != "https://llm2.corp.example/anthropic" {
		t.Fatalf("a change as shown: %+v", got)
	}
	if r := a.anna.Do("POST", a.url("/env/proposals/"+got[0].ID+"/decline"), map[string]any{}); r.Code != 204 {
		t.Fatalf("decline: %s", r)
	}
	if len(a.waiting(t)) != 0 {
		t.Fatal("a declined proposal still waits")
	}
	var cat struct {
		Items []struct {
			ID     string
			Source map[string]any
		}
	}
	a.anna.Do("GET", a.url("/catalog"), nil).JSON(&cat)
	for _, c := range cat.Items {
		if c.ID == applied.ItemID && c.Source["baseUrl"] != "https://llm.corp.example/anthropic" {
			t.Fatalf("a declined proposal changed the source: %v", c.Source)
		}
	}

	// A value in place of a secret's name, and a variable that sends the key elsewhere, are not kept at all.
	for name, bad := range map[string]string{
		"a value as a secret variable": `{"api":"anthropic","baseUrl":"https://llm.corp.example","secretEnv":["ANTHROPIC_AUTH_TOKEN=not a name of a secret"]}`,
		"a proxy":                      `{"api":"anthropic","baseUrl":"https://llm.corp.example","env":["HTTPS_PROXY=http://elsewhere.example"]}`,
	} {
		r := a.propose(t, a.anna.ID, hubproto.EnvProposal{Provider: hubproto.EnvProviderProposal{Name: "Другой", Source: json.RawMessage(bad)}})
		if r.Status != 400 {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if r := a.propose(t, a.anna.ID, hubproto.EnvProposal{Provider: hubproto.EnvProviderProposal{Name: "Нет такого", Remove: true}}); r.Status != 400 {
		t.Fatalf("remove of none: %+v", r)
	}
	if len(a.waiting(t)) != 0 {
		t.Fatal("a refused proposal waits")
	}

	// Taking it out is proposed and applied the same way.
	if r := a.propose(t, a.dina.ID, hubproto.EnvProposal{Provider: hubproto.EnvProviderProposal{Name: "Шлюз", Remove: true}}); r.Status != 202 {
		t.Fatalf("remove: %+v", r)
	}
	got = a.waiting(t)
	if len(got) != 1 || got[0].Change != "remove" {
		t.Fatalf("a removal as shown: %+v", got)
	}
	if items, _ := a.providers(t); len(items) != 1 {
		t.Fatalf("a proposed removal took the provider out: %v", items)
	}
	if r := a.anna.Do("POST", a.url("/env/proposals/"+got[0].ID+"/apply"), map[string]any{}); r.Code != 200 {
		t.Fatalf("apply the removal: %s", r)
	}
	if items, pinned := a.providers(t); len(items) != 0 || pinned != 0 {
		t.Fatalf("after the removal: %v, %d", items, pinned)
	}
}

func TestOneMachineDoesNotFillTheWaitingProposals(t *testing.T) {
	a := newAcme(t)
	src := json.RawMessage(`{"api":"anthropic","baseUrl":"https://llm.corp.example"}`)
	for i, want := range []int{202, 202, 202, 202, 202, 429} {
		name := "Шлюз " + string(rune('A'+i))
		if r := a.propose(t, a.dina.ID, hubproto.EnvProposal{Provider: hubproto.EnvProviderProposal{Name: name, Source: src}}); r.Status != want {
			t.Fatalf("%s: %+v, want %d", name, r, want)
		}
	}
	if got := a.waiting(t); len(got) != 5 {
		t.Fatalf("%d wait", len(got))
	}
}

func TestAProposalStopsWaitingWhenItsTermEnds(t *testing.T) {
	a := newAcme(t)
	st := a.s.DB.S()
	ctx := context.Background()
	now := time.Now()
	old, err := st.FileEnvProposal(ctx, store.EnvProposal{OrgID: a.org, MachineID: "m1", MachineName: "ноутбук", Name: "Шлюз", Source: `{"api":"anthropic","baseUrl":"https://llm.corp.example"}`,
		CreatedAt: now.Add(-8 * 24 * time.Hour), ExpiresAt: now.Add(-time.Hour)}, now.Add(-8*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := a.waiting(t); len(got) != 0 {
		t.Fatalf("a proposal past its term is shown: %+v", got)
	}
	if r := a.anna.Do("POST", a.url("/env/proposals/"+old.ID+"/apply"), map[string]any{}); r.Code != 409 {
		t.Fatalf("a proposal past its term was applied: %s", r)
	}
	if items, _ := a.providers(t); len(items) != 0 {
		t.Fatalf("%v", items)
	}
}
