package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/apps"
	"github.com/ly-solutions/harnessly/internal/gate"
)

// A chat's agent reaches the secrets of the apps that are on in its chat, and no other's.
func TestAppSecretsNeedTheChatsGrant(t *testing.T) {
	t.Setenv("HARNESSLY_UI_SLOTS", "on")
	data := t.TempDir()
	cli := &gate.CLIToken{DataDir: data, Headers: []string{"x-cli-token"}}
	d := deps{DataDir: data, Home: t.TempDir(), CLI: cli}
	reg := apps.Open(data)
	dir := filepath.Join(data, "app-launch", "bbbbbbbbbbbbbbbb")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "grant.json"), []byte(`{"apps":["acme.other"]}`), 0o600)
	token, _ := cli.IssueAgent("bbbbbbbbbbbbbbbb", 1e12)
	for _, route := range appExtRoutes(d, reg, nil, "test") {
		r := httptest.NewRequest(http.MethodPost, "/api/cli/apps/secrets/use", strings.NewReader(`{"id":"acme.pay","names":["key"]}`))
		r.Header.Set("x-cli-token", token)
		if !route.Match(r) {
			continue
		}
		w := httptest.NewRecorder()
		route.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "not-on") {
			t.Fatalf("an app that is not on in the chat: %d %s", w.Code, w.Body)
		}
		return
	}
	t.Fatal("no route")
}
