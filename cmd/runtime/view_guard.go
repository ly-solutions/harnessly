package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/sameorigin"
	"github.com/ly-solutions/harnessly/internal/viewonly"
)

// watcherGuard holds a session the hub asked for as a watcher's (docs/hub/own-work.md, «Levels») to what a watcher
// reads: the hub draws the same line before, and this one stands if something reaches the runtime past it. Sockets go
// on marked, so that they drop the messages that act.
func watcherGuard(secret []byte, cookie string) func(http.ResponseWriter, *http.Request) (*http.Request, bool) {
	return func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		c, err := r.Cookie(cookie)
		if err != nil || !agent.WatcherSession(secret, c.Value, time.Now()) {
			return r, true
		}
		path := runtimeRoute(r.URL.Path)
		read := r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions
		allowed := read && viewonly.Readable(path, r.URL.EscapedPath())
		if isUpgrade(r) {
			allowed = r.Method == http.MethodGet && viewonly.Socket(path)
		}
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"This machine is open to you to watch only","code":"view_only"}`))
			return r, false
		}
		return r.WithContext(viewonly.With(r.Context())), true
	}
}

// sessionWriteGuard refuses a write of the API that rides the person's session cookie from a page of another origin.
// SameSite=Lax does not tell ports apart: the page of any other local server would send the cookie along. A request
// without the cookie, or with the cli-token, is not the browser acting for the person and passes to its route's own rule.
func sessionWriteGuard(secret []byte, cookie string, cliToken func(*http.Request) bool) func(http.ResponseWriter, *http.Request) (*http.Request, bool) {
	return func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return r, true
		}
		path := runtimeRoute(r.URL.Path)
		if path != "/api" && !strings.HasPrefix(path, "/api/") || !sameorigin.CrossSite(r) || cliToken(r) {
			return r, true
		}
		c, err := r.Cookie(cookie)
		if err != nil || agent.VerifySession(secret, c.Value, time.Now()) != nil {
			return r, true
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Forbidden: a page of another origin"}`))
		return r, false
	}
}

// runtimeRoute is a request's path without the runtime's mounts: /m/<id> and /agents.
func runtimeRoute(path string) string {
	if rest, ok := strings.CutPrefix(path, "/m/"); ok {
		_, tail, _ := strings.Cut(rest, "/")
		path = "/" + tail
	}
	if rest, ok := strings.CutPrefix(path, "/agents"); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
		path = "/" + strings.TrimPrefix(rest, "/")
	}
	return path
}
