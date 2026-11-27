package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/gate"
)

// A write that rides the person's cookie comes from the runtime's own page: another port of localhost is the same
// site to the browser, and its page would otherwise type into a pane.
func TestSessionWritesFromOwnPageOnly(t *testing.T) {
	secret := []byte("pair")
	g, err := gate.New(gate.Config{Upstream: "127.0.0.1:1", DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	g.Add(gate.Route{Match: func(*http.Request) bool { return true }, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })})
	g.Protect(sessionWriteGuard(secret, "s", func(r *http.Request) bool { return r.Header.Get("x-tok") == "t" }))
	full := agent.SignHS256(secret, "owner", "", time.Hour)
	for name, tc := range map[string]struct {
		method, path string
		cookie       bool
		h            map[string]string
		want         int
	}{
		"another port":               {"POST", "/api/platform/send-input", true, map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		"another port, no metadata":  {"POST", "/agents/api/tmux/send-input", true, map[string]string{"Origin": "http://localhost:3000"}, 403},
		"the own page":               {"POST", "/api/platform/send-input", true, map[string]string{"Sec-Fetch-Site": "same-origin"}, 204},
		"the own origin":             {"DELETE", "/api/workspace/w", true, map[string]string{"Origin": "http://localhost:8022"}, 204},
		"no browser":                 {"POST", "/api/layout/pane", true, nil, 204},
		"a read":                     {"GET", "/api/layout", true, map[string]string{"Sec-Fetch-Site": "same-site"}, 204},
		"no cookie":                  {"POST", "/api/status/hook", false, map[string]string{"Sec-Fetch-Site": "cross-site"}, 204},
		"the cli-token":              {"POST", "/api/cli/open", true, map[string]string{"Sec-Fetch-Site": "cross-site", "x-tok": "t"}, 204},
		"not the API (a dev server)": {"POST", "/__pv/1/form", true, map[string]string{"Sec-Fetch-Site": "same-site"}, 204},
	} {
		r := httptest.NewRequest(tc.method, "http://localhost:8022"+tc.path, nil)
		if tc.cookie {
			r.AddCookie(&http.Cookie{Name: "s", Value: full})
		}
		for k, v := range tc.h {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s: %d, want %d", name, w.Code, tc.want)
		}
	}
}
