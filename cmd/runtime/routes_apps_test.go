package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/apps"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/wire"
)

func appRig(t *testing.T) (func(method, path, body string, h map[string]string) (int, string), *wire.Conversation) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cli-token"), []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := deps{DataDir: dir, Home: t.TempDir(), CLI: &gate.CLIToken{DataDir: dir, Headers: []string{"x-cli-token"}},
		SessionOK: func(r *http.Request) bool { return r.Header.Get("X-Test-Session") == "yes" }}
	reg := apps.Open(dir)
	conv := &wire.Conversation{ID: "tab-1"}
	chats := appChats{
		ofSession: func(session string) string {
			if session == "hs-agent" {
				return "tab-1"
			}
			return ""
		},
		record: func(id string) (wire.Conversation, bool) { return *conv, id == conv.ID },
		do: func(_ context.Context, cmd wire.Command) wire.CmdResult {
			m := cmd.(wire.ConversationAppsCmd)
			if bad := appsOK(reg)(m.Apps); len(m.Apps) > 0 && bad != "" {
				return wire.Refused(wire.RefuseNotFound)
			}
			conv.Apps = m.Apps
			return wire.Accepted(wire.ChannelConvs, 1)
		},
	}
	routes := appRouteList(d, reg, "0.17.0", chats)
	return func(method, path, body string, h map[string]string) (int, string) {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		for k, v := range h {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		for _, rt := range routes {
			if rt.Match(req) {
				rt.Handler.ServeHTTP(w, req)
				return w.Code, w.Body.String()
			}
		}
		t.Fatalf("no route for %s", path)
		return 0, ""
	}, conv
}

func TestAppRoutes(t *testing.T) {
	call, conv := appRig(t)
	cli := map[string]string{"x-cli-token": "tok"}
	pkg := filepath.Join(t.TempDir(), "demo")
	if err := apps.Scaffold(pkg, "acme.demo", "Демо", "chat"); err != nil {
		t.Fatal(err)
	}

	if code, _ := call("GET", "/api/cli/apps", "", nil); code != http.StatusForbidden {
		t.Fatalf("no token: %d", code)
	}
	if code, body := call("POST", "/api/cli/apps/install", `{"dir":"demo"}`, cli); code != http.StatusBadRequest {
		t.Fatalf("relative folder: %d %s", code, body)
	}
	code, body := call("POST", "/api/cli/apps/install", `{"dir":"`+pkg+`"}`, cli)
	if code != http.StatusOK || !strings.Contains(body, `"id":"acme.demo"`) {
		t.Fatalf("install: %d %s", code, body)
	}

	// The page lists it without the folder.
	code, body = call("GET", platformAPI+"/installed-apps", "", map[string]string{"X-Test-Session": "yes"})
	var page struct {
		Apps []appView `json:"apps"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &page) != nil || len(page.Apps) != 1 || page.Apps[0].Dir != "" || page.Apps[0].Name != "Демо" {
		t.Fatalf("page list: %d %s", code, body)
	}
	if code, _ := call("GET", platformAPI+"/installed-apps", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("page without a session: %d", code)
	}

	// On and off in the chat of the calling tab.
	if code, body := call("POST", "/api/cli/apps/enable", `{"session":"hs-agent","id":"acme.demo","on":true}`, cli); code != http.StatusOK {
		t.Fatalf("enable: %d %s", code, body)
	}
	if len(conv.Apps) != 1 || conv.Apps[0] != "acme.demo" {
		t.Fatalf("chat apps: %v", conv.Apps)
	}
	if code, _ := call("POST", "/api/cli/apps/enable", `{"session":"hs-agent","id":"acme.nope","on":true}`, cli); code != http.StatusNotFound {
		t.Fatalf("enable an app not installed: %d", code)
	}
	if code, body := call("POST", "/api/cli/apps/enable", `{"session":"other","id":"acme.demo","on":true}`, cli); code != http.StatusNotFound || !strings.Contains(body, "no-chat") {
		t.Fatalf("enable outside a chat: %d %s", code, body)
	}
	if code, _ := call("POST", "/api/cli/apps/enable", `{"session":"hs-agent","id":"acme.demo","on":false}`, cli); code != http.StatusOK || len(conv.Apps) != 0 {
		t.Fatalf("disable: %d %v", code, conv.Apps)
	}

	// A broken package is refused with its problems.
	if err := os.WriteFile(filepath.Join(pkg, apps.ManifestPath), []byte(`{"id":"acme.demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, body = call("POST", "/api/cli/apps/install", `{"dir":"`+pkg+`"}`, cli)
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, `"path":"name"`) {
		t.Fatalf("broken package: %d %s", code, body)
	}

	if code, _ := call("POST", "/api/cli/apps/remove", `{"id":"acme.demo"}`, cli); code != http.StatusOK {
		t.Fatalf("remove: %d", code)
	}
	if code, _ := call("POST", "/api/cli/apps/remove", `{"id":"acme.demo"}`, cli); code != http.StatusNotFound {
		t.Fatalf("remove twice: %d", code)
	}
}
