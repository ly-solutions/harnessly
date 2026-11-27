package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
)

// A watcher's session reads what the watching page shows and nothing else, whatever carried it to the runtime.
func TestWatcherGuard(t *testing.T) {
	secret := []byte("pair")
	// A token as the machine agent signs a watcher's: no audience, a level.
	token := agent.SignWatcher(secret, "w@example.com", time.Hour)
	full := agent.SignHS256(secret, "owner", "", time.Hour)
	guard := watcherGuard(secret, "s")
	for _, tc := range []struct {
		method, path, token, upgrade string
		ok                           bool
	}{
		{"GET", "/api/v2/conversations/tab-1/turns", token, "", true},
		{"GET", "/m/abc/agents/api/layout", token, "", true},
		{"GET", "/api/v2/conversations/tab-_home/download", token, "", false},
		{"GET", "/agents/api/platform/file-preview", token, "", false},
		{"POST", "/api/v2/cmd/send", token, "", false},
		{"GET", "/api/timeline", token, "websocket", true},
		{"GET", "/api/terminal", token, "websocket", false},
		{"GET", "/api/v2/conversations/tab-_home/download", full, "", true},
		{"POST", "/api/v2/cmd/send", full, "", true},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.AddCookie(&http.Cookie{Name: "s", Value: tc.token})
		if tc.upgrade != "" {
			r.Header.Set("Upgrade", tc.upgrade)
		}
		w := httptest.NewRecorder()
		if _, ok := guard(w, r); ok != tc.ok {
			t.Errorf("%s %s: %v (%d)", tc.method, tc.path, ok, w.Code)
		}
	}
}
