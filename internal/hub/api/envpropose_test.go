package api_test

import (
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// Over the machine's link: the hub answers an agent's env.propose with env.proposed, by the rights of the account that
// connected the machine.
func TestAMachinesEnvironmentProposalIsAnsweredOverItsLink(t *testing.T) {
	s := hubtest.New(t, hubtest.Env("HUB_ORGS", "on"), hubtest.Env("HUB_ENV_FEATURES", "all"), hubtest.Env("HUB_ENV_RECOMMENDED", "off"))
	s.SignIn("root@example.com") // the first account administers the instance
	admin := s.SignIn("anna@example.com")
	loner := s.SignIn("vera@example.com")
	space, _ := admin.NewSpace("Платформа")
	own, _ := loner.NewSpace("Своё")
	if r := admin.Do("POST", "/api/orgs", map[string]any{"name": "Acme", "spaceIds": []string{space}}); r.Code != 201 {
		t.Fatalf("org: %s", r)
	}
	answer := func(p *hubtest.Person, in, rid string) hubtest.Call {
		t.Helper()
		m := p.Machine(in, nil)
		m.Handle("env.proposed", func(hubtest.Call) (any, error) { return map[string]bool{"settled": true}, nil })
		m.Emit(map[string]any{"event": "env.propose", "requestId": rid, "conv": "tab-1", "convTitle": "Чат",
			"provider": map[string]any{"name": "Шлюз", "source": map[string]any{"api": "anthropic", "baseUrl": "https://llm.corp.example", "key": "gateway-key"}}})
		var res hubtest.Call
		s.Eventually("env.proposed "+rid, func() bool {
			for _, c := range m.Calls("env.proposed") {
				if c.Str("requestId") == rid {
					res = c
					return true
				}
			}
			return false
		})
		return res
	}
	// A machine of a space that is in no organization has no environment to propose for.
	if res := answer(loner, own, "r1"); string(res.Params["status"]) != "409" || !strings.Contains(string(res.Params["message"]), "не в организации") {
		t.Fatalf("a machine outside an organization: %v", res.Params)
	}
	// The machine of the organization's own admin changes nothing either: the proposal waits for the admin's yes in the
	// hub, under their session.
	if res := answer(admin, space, "r2"); string(res.Params["status"]) != "202" || len(res.Params["proposalId"]) < 4 {
		t.Fatalf("the admin's machine: %v", res.Params)
	}
	org := orgID(t, admin)
	has := func() bool {
		var env struct {
			Layers []struct{ Items []struct{ Kind, Name string } }
		}
		admin.Do("GET", "/api/orgs/"+org+"/env", nil).JSON(&env)
		for _, l := range env.Layers {
			for _, it := range l.Items {
				if it.Kind == "provider" && it.Name == "Шлюз" {
					return true
				}
			}
		}
		return false
	}
	if has() {
		t.Fatal("a machine's proposal changed the environment")
	}
	var list struct {
		Proposals []struct{ ID, Machine, ConvTitle string }
	}
	admin.Do("GET", "/api/orgs/"+org+"/env/proposals", nil).JSON(&list)
	if len(list.Proposals) != 1 || list.Proposals[0].ConvTitle != "Чат" || list.Proposals[0].Machine == "" {
		t.Fatalf("the waiting proposals: %+v", list)
	}
	if r := admin.Do("POST", "/api/orgs/"+org+"/env/proposals/"+list.Proposals[0].ID+"/apply", map[string]any{}); r.Code != 200 {
		t.Fatalf("apply: %s", r)
	}
	if !has() {
		t.Fatal("the applied provider is not in the environment")
	}
}

func orgID(t *testing.T, p *hubtest.Person) string {
	t.Helper()
	var out struct{ Orgs []struct{ ID string } }
	p.Do("GET", "/api/orgs", nil).JSON(&out)
	if len(out.Orgs) == 0 {
		t.Fatal("no organization")
	}
	return out.Orgs[0].ID
}
