package automations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/auth"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// person makes an account of the hub; proved says whether its sign-in vouched for the address.
func person(t *testing.T, s *Service, id, email string, admin, proved bool) *auth.Principal {
	t.Helper()
	ctx := context.Background()
	u := &store.User{ID: id, Email: email, IsAdmin: admin}
	if err := s.db.S().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := s.db.S().CreateIdentity(ctx, &store.Identity{UserID: u.ID, Provider: "email", Subject: email, Email: email, EmailVerified: proved}); err != nil {
		t.Fatal(err)
	}
	return &auth.Principal{User: *u, Admin: admin}
}

// The owner of a run approves it with the hub's GitLab token only as an administrator or a named operator: anybody may
// have a rule and report any merge request of the allowed projects.
func TestOnlyAnOperatorsDecisionReachesGitLab(t *testing.T) {
	for name, c := range map[string]struct {
		action        string
		admin, proved bool
		operators     []string
		writes        int
	}{
		"anybody approves":           {action: "approve", proved: true},
		"anybody rejects":            {action: "reject", proved: true},
		"an operator's unproved one": {action: "approve", operators: []string{"u1@example.com"}},
		"an operator approves":       {action: "approve", proved: true, operators: []string{"u1@example.com"}, writes: 1},
		"an administrator rejects":   {action: "reject", admin: true, proved: true, writes: 1},
	} {
		t.Run(name, func(t *testing.T) {
			s, gl, id := approvable(t, greenMR)
			s.cfg.AutomationsOperators = c.operators
			p := person(t, s, "u1", "u1@example.com", c.admin, c.proved)
			req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"action":"`+c.action+`"}`))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("id", id)
			w := httptest.NewRecorder()
			if err := s.decision(w, req, p); err != nil || w.Code != http.StatusOK {
				t.Fatalf("decision: %v %d %s", err, w.Code, w.Body)
			}
			if len(gl.writes) != c.writes {
				t.Fatalf("writes to GitLab: %v, want %d", gl.writes, c.writes)
			}
			if c.writes == 0 && (!strings.Contains(w.Body.String(), `"reason":"`) || strings.Contains(w.Body.String(), `"reason":""`)) {
				t.Fatalf("no reason given: %s", w.Body)
			}
			if got := statusOf(t, s, id).Status; got == StatusAwaitingApproval {
				t.Fatalf("the decision was not recorded: %q", got)
			}
		})
	}
}

// The deployment's /hook carries the operator's own alerts: they reach the rules of administrators and named operators,
// not of whoever made a rule with a matching source.
func TestDeploymentEventsReachOperatorsRulesOnly(t *testing.T) {
	s := newSvc(t, noon())
	s.cfg.AutomationsOperators = []string{"op@example.com"}
	person(t, s, "admin", "admin@example.com", true, true)
	person(t, s, "op", "op@example.com", false, true)
	person(t, s, "other", "other@example.com", false, true)
	ids := map[string]string{}
	for _, owner := range []string{"admin", "op", "other", "nobody"} {
		r, ok, err := s.addRule(context.Background(), Rule{OwnerID: owner, Name: "a", Trigger: Trigger{Source: "alert"}, PromptTemplate: "p", Model: "sonnet", Enabled: true})
		if err != nil || !ok {
			t.Fatal(ok, err)
		}
		ids[owner] = r.ID
	}
	w := post(s, "s3cret", `{"source":"alert","fingerprint":"f1","event":{}}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("hook: %d %s", w.Code, w.Body)
	}
	for owner, want := range map[string]bool{"admin": true, "op": true, "other": false, "nobody": false} {
		if got := strings.Contains(w.Body.String(), ids[owner]); got != want {
			t.Errorf("%s's rule took the event: %v, want %v", owner, got, want)
		}
	}
}

// Whoever owned a rule when operators' rights came keeps them with no variable set: their rules take the shared /hook's
// events and their decision reaches GitLab. A rule made later gives nothing (the test above).
func TestRuleOwnersFromBeforeStayOperators(t *testing.T) {
	s, gl, id := approvable(t, greenMR)
	p := person(t, s, "u1", "u1@example.com", false, false)
	if _, err := s.db.Exec(context.Background(), `INSERT INTO automation_operators (user_id) VALUES (?)`, "u1"); err != nil {
		t.Fatal(err)
	}
	rule, ok, err := s.addRule(context.Background(), Rule{OwnerID: "u1", Name: "hook", Trigger: Trigger{Source: "alert"}, PromptTemplate: "p", Model: "sonnet", Enabled: true})
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if w := post(s, "s3cret", `{"source":"alert","fingerprint":"f1","event":{}}`); !strings.Contains(w.Body.String(), rule.ID) {
		t.Errorf("the rule did not take the event: %d %s", w.Code, w.Body)
	}
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"action":"approve"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()
	if err := s.decision(w, req, p); err != nil || w.Code != http.StatusOK || len(gl.writes) != 1 {
		t.Fatalf("decision: %v %d %s, writes %v", err, w.Code, w.Body, gl.writes)
	}
}
