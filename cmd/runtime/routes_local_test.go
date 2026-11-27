package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
)

// the pass of «this computer» and the way it opens a session on the laptop's loopback.
func TestLocalPassOpensASessionOnLoopbackOnly(t *testing.T) {
	t.Setenv("RUNTIME_HUB_URL", "https://hub.example.com")
	secret := []byte("0123456789abcdef0123456789abcdef")
	d := deps{Secret: secret, Cookie: "session-token", Claims: func(r *http.Request) map[string]any {
		c, err := r.Cookie("session-token")
		if err != nil {
			return nil
		}
		claims, _ := agent.VerifiedPayload(secret, c.Value, time.Now())
		return claims
	}}
	routes := localRoutes(d, "127.0.0.1:18022")
	serve := func(r *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		for _, rt := range routes {
			if rt.Match(r) {
				rt.Handler.ServeHTTP(w, r)
				return w
			}
		}
		t.Fatalf("no route for %s", r.URL.Path)
		return nil
	}

	// No session: no pass. A watcher's session: no pass either.
	if w := serve(httptest.NewRequest(http.MethodGet, platformAPI+"/local-pass", nil)); w.Code != http.StatusForbidden {
		t.Fatalf("pass without a session: %d", w.Code)
	}
	ask := httptest.NewRequest(http.MethodGet, platformAPI+"/local-pass", nil)
	ask.AddCookie(&http.Cookie{Name: "session-token", Value: agent.SignSession(secret, "owner@example.com", true, time.Hour)})
	w := serve(ask)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"port":18022`) {
		t.Fatalf("pass: %d %s", w.Code, w.Body.String())
	}
	pass := between(w.Body.String(), `"pass":"`, `"`)

	enter := func(remote, host, origin, site, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, localEnterPath, strings.NewReader(body))
		r.RemoteAddr, r.Host = remote, host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if site != "" {
			r.Header.Set("Sec-Fetch-Site", site)
		}
		return serve(r)
	}
	ping := `{"pass":"` + pass + `"}`
	in := `{"pass":"` + pass + `","enter":true}`

	// The hub's page asks whether this is the computer: yes, with CORS for the hub alone.
	w = enter("127.0.0.1:5000", "127.0.0.1:18022", "https://hub.example.com", "cross-site", ping)
	if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != "https://hub.example.com" ||
		w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("ping from the hub: %d %v", w.Code, w.Header())
	}
	// The hub's page cannot open the session itself, and no other site can.
	if w = enter("127.0.0.1:5000", "127.0.0.1:18022", "https://hub.example.com", "cross-site", in); w.Code != http.StatusForbidden {
		t.Fatalf("enter from the hub: %d", w.Code)
	}
	if w = enter("127.0.0.1:5000", "127.0.0.1:18022", "https://evil.example", "cross-site", in); w.Code != http.StatusForbidden {
		t.Fatalf("enter from another site: %d", w.Code)
	}
	// Another device, or a name that is not loopback (DNS rebinding): refused.
	if w = enter("192.168.1.5:5000", "127.0.0.1:18022", "", "same-origin", in); w.Code != http.StatusForbidden {
		t.Fatalf("enter from another device: %d", w.Code)
	}
	if w = enter("127.0.0.1:5000", "evil.example:18022", "", "same-origin", in); w.Code != http.StatusForbidden {
		t.Fatalf("enter by a rebound name: %d", w.Code)
	}
	// A forged pass: refused.
	if w = enter("127.0.0.1:5000", "localhost:18022", "", "same-origin", `{"pass":"x.y.z","enter":true}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("forged pass: %d", w.Code)
	}
	// The page of this origin: a session of the same person and rights.
	w = enter("127.0.0.1:5000", "localhost:18022", "", "same-origin", in)
	if w.Code != http.StatusOK {
		t.Fatalf("enter: %d %s", w.Code, w.Body.String())
	}
	cookie := w.Header().Get("Set-Cookie")
	token := between(cookie, "session-token=", ";")
	if agent.VerifySession(secret, token, time.Now()) != nil {
		t.Fatalf("the session does not verify: %s", cookie)
	}
	claims, _ := agent.VerifiedPayload(secret, token, time.Now())
	if claims["sub"] != "owner@example.com" || claims["verifiedOwner"] != true {
		t.Fatalf("session claims: %v", claims)
	}
	// A pass is no session itself.
	if agent.VerifySession(secret, pass, time.Now()) == nil {
		t.Fatal("a pass passes as a page session")
	}
}

func TestLocalPortIsLoopbackOnly(t *testing.T) {
	for addr, want := range map[string]int{"127.0.0.1:18022": 18022, ":9000": 9000, "0.0.0.0:7": 7, "[::1]:8": 8,
		"10.0.0.5:18022": 0, "example.com:1": 0} {
		if got := localPort(addr); got != want {
			t.Errorf("localPort(%q) = %d, want %d", addr, got, want)
		}
	}
}

func between(s, from, to string) string {
	i := strings.Index(s, from)
	if i < 0 {
		return ""
	}
	s = s[i+len(from):]
	if j := strings.Index(s, to); j >= 0 {
		return s[:j]
	}
	return s
}
