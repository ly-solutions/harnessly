package agent

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// A proposal of the runtime goes to the machine's own hub as env.propose and comes back with what the hub answered;
// without the local token nothing leaves.
func TestAnEnvironmentProposalGoesToTheHub(t *testing.T) {
	r := startHubAgent(t, rigOptions{})
	c := r.connect()
	a := r.agent
	waitFor(t, func() bool { return len(a.hub.hubLinks()) > 0 })
	body := `{"conv":"tab-1","convTitle":"Чат","check":true,"provider":{"name":"Шлюз","source":{"api":"anthropic","baseUrl":"https://llm.corp.example","key":"gateway-key"}}}`

	if rec := post(a, "/env/propose", "wrong", body); rec.Code != 403 {
		t.Fatalf("without the token: %d %s", rec.Code, rec.Body)
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- post(a, "/env/propose", a.localToken, body) }()
	ev, _ := c.waitEvent(hubproto.EventEnvPropose, 0, 5*time.Second)
	if ev == nil {
		t.Fatal("no env.propose reached the hub")
	}
	var p hubproto.EnvProposal
	if json.Unmarshal(mustJSON(ev), &p) != nil || p.RequestID == "" || !p.Check || p.Conv != "tab-1" || p.Provider.Name != "Шлюз" ||
		!strings.Contains(string(p.Provider.Source), `"key":"gateway-key"`) {
		t.Fatalf("env.propose: %v", ev)
	}
	c.mustCall(hubproto.OpEnvProposed, map[string]any{"requestId": p.RequestID, "status": 403, "code": "forbidden", "message": "нет права"})
	select {
	case rec := <-done:
		var res hubproto.EnvProposalResult
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.Status != 403 || res.Message != "нет права" {
			t.Fatalf("answer: %d %s", rec.Code, rec.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the proposal did not answer")
	}
}

// A hub that only observes the machine is not the one whose environment the machine has: it is not asked.
func TestAnEnvironmentProposalIsNotSentToAnObservingHub(t *testing.T) {
	r := startHubAgent(t, rigOptions{observe: true})
	c := r.connect()
	a := r.agent
	waitFor(t, func() bool { return len(a.hub.hubLinks()) > 0 })
	rec := post(a, "/env/propose", a.localToken, `{"provider":{"name":"Шлюз","remove":true}}`)
	var res hubproto.EnvProposalResult
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.Status != 409 || res.Code != "no_hub" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if ev, _ := c.waitEvent(hubproto.EventEnvPropose, 0, 300*time.Millisecond); ev != nil {
		t.Fatalf("the observing hub was asked: %v", ev)
	}
}
