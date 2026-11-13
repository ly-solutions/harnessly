package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/codexaccounts"
	"github.com/ly-solutions/harnessly/internal/providers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

func TestASignInOfCodexIsALaunchProfileWithItsHome(t *testing.T) {
	dir, main := t.TempDir(), filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(main, 0o700); err != nil {
		t.Fatal(err)
	}
	st := &codexaccounts.Store{Dir: dir, Main: main}
	a, err := st.Add("Второй")
	if err != nil {
		t.Fatal(err)
	}
	syncCodexLoginProfiles(st)
	t.Cleanup(func() { providers.SetCodexLogins(nil); codexLoginStore.Store(nil) })
	p, ok := providers.Find(codexLoginProfileID(a.ID))
	plan, _ := providers.Find("gpt")
	if !ok || p.Via != wire.ProviderCodex || p.Home != st.Home(a.ID) || p.Label != "Второй" || len(p.Models) != len(plan.Models) {
		t.Fatalf("%+v %v", p, ok)
	}

	// Before a chat starts, the home gains what the main one gained since.
	codexLoginStore.Store(st)
	if err := os.WriteFile(filepath.Join(main, "config.toml"), []byte("model = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syncCodexLoginHome(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Home(a.ID), "config.toml")); err != nil {
		t.Fatalf("the home was not brought up to date: %v", err)
	}
	if err := syncCodexLoginHome("xai"); err != nil {
		t.Fatalf("a profile that is no sign-in: %v", err)
	}

	// A removed sign-in is no profile any more.
	if err := st.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	syncCodexLoginProfiles(st)
	if _, ok := providers.Find(p.ID); ok {
		t.Fatal("the profile of a removed sign-in stayed")
	}
}

type noLogins struct{ wire.LoginSource }

func TestTheSignInOfAnAccountRunsInItsHomeAndTakesNoKey(t *testing.T) {
	dir, main := t.TempDir(), filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(main, 0o700); err != nil {
		t.Fatal(err)
	}
	st := &codexaccounts.Store{Dir: dir, Main: main}
	a, err := st.Add("Второй")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { providers.SetCodexLogins(nil) })
	var homes []string
	told := 0
	h := &codexLoginFlows{store: st, changed: func() { told++ },
		make: func(home string, changed func(wire.Provider)) wire.LoginSource {
			homes = append(homes, home)
			changed(wire.ProviderCodex)
			return noLogins{}
		}}
	src, ok := h.of(wire.ProviderCodex, a.ID)
	if !ok || len(homes) != 1 || homes[0] != st.Home(a.ID) || told != 1 {
		t.Fatalf("%v %v %d", ok, homes, told)
	}
	if err := src.SaveAPIKey(t.Context(), wire.ProviderCodex, "k"); err != wire.ErrLoginUnsupported {
		t.Fatalf("an account took an API key: %v", err)
	}
	// The same account is the same service; another CLI and an unknown account have none.
	if _, ok := h.of(wire.ProviderCodex, a.ID); !ok || len(homes) != 1 {
		t.Fatalf("a second service for one account: %v", homes)
	}
	if _, ok := h.of(wire.ProviderClaude, a.ID); ok {
		t.Fatal("Claude has no accounts here")
	}
	if _, ok := h.of(wire.ProviderCodex, "../../etc"); ok {
		t.Fatal("an account the registry does not have")
	}
	if _, ok := (&codexLoginFlows{store: st}).of(wire.ProviderCodex, a.ID); ok {
		t.Fatal("a runtime that cannot sign in answered")
	}
}

// The routes: a sign-in is made the one a new chat is offered first and gives that back; one a chat still runs on is
// not removed.
func TestTheRoutesOfCodexSignInsKeepTheMainOneAndRefuseABusyOne(t *testing.T) {
	d := deps{DataDir: t.TempDir(), Home: t.TempDir(), SessionOK: func(*http.Request) bool { return true }}
	routes := codexLoginRoutes(d)
	busy := ""
	hub := &codexLoginFlows{store: &codexaccounts.Store{Dir: d.DataDir, Main: providers.CodexHome(d.Home)},
		inUse: func(profile string) bool { return profile == busy }}
	codexLoginHub.Store(hub)
	t.Cleanup(func() { codexLoginHub.Store(nil); providers.SetCodexLogins(nil) })
	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		r := httptest.NewRequest(method, platformAPI+"/codex-logins"+path, strings.NewReader(body))
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		for _, route := range routes {
			if route.Match(r) {
				route.Handler.ServeHTTP(w, r)
				var out map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &out)
				return w.Code, out
			}
		}
		t.Fatalf("no route for %s %s", method, path)
		return 0, nil
	}
	add := func(name string) string {
		t.Helper()
		code, out := call(http.MethodPost, "", `{"name":"`+name+`"}`)
		login, _ := out["login"].(map[string]any)
		id, _ := login["id"].(string)
		if code != http.StatusCreated || id == "" {
			t.Fatalf("add: %d %v", code, out)
		}
		return id
	}
	mains := func() map[string]bool {
		t.Helper()
		_, out := call(http.MethodGet, "", "")
		got := map[string]bool{}
		list, _ := out["logins"].([]any)
		for _, raw := range list {
			l, _ := raw.(map[string]any)
			got[l["id"].(string)] = l["main"] == true
		}
		return got
	}
	a, b := add("Первый"), add("Второй")

	if code, _ := call(http.MethodPut, "/"+a, `{"main":true}`); code != http.StatusOK {
		t.Fatalf("main: %d", code)
	}
	if code, _ := call(http.MethodPut, "/"+b, `{"main":true}`); code != http.StatusOK {
		t.Fatalf("main: %d", code)
	}
	if got := mains(); got[a] || !got[b] {
		t.Fatalf("one main sign-in: %v", got)
	}
	// Giving back what is not its own changes nothing; its own gives the place to the CLI's home.
	if code, _ := call(http.MethodPut, "/"+a, `{"main":false}`); code != http.StatusOK || !mains()[b] {
		t.Fatalf("another's main was taken: %d %v", code, mains())
	}
	if code, _ := call(http.MethodPut, "/"+b, `{"main":false}`); code != http.StatusOK || mains()[b] {
		t.Fatalf("main was not given back: %d %v", code, mains())
	}
	if code, _ := call(http.MethodPut, "/"+b, `{}`); code != http.StatusBadRequest {
		t.Fatalf("a body without main: %d", code)
	}
	if code, _ := call(http.MethodPut, "/nosuch1", `{"main":true}`); code != http.StatusNotFound {
		t.Fatalf("no such sign-in: %d", code)
	}

	busy = codexLoginProfileID(a)
	if code, out := call(http.MethodDelete, "/"+a, ""); code != http.StatusConflict || out["error"] != "in-use" {
		t.Fatalf("a busy sign-in: %d %v", code, out)
	}
	if _, has := mains()[a]; !has {
		t.Fatal("a busy sign-in was removed")
	}
	busy = ""
	if code, _ := call(http.MethodDelete, "/"+a, ""); code != http.StatusOK {
		t.Fatalf("a free sign-in: %d", code)
	}
	if _, has := mains()[a]; has {
		t.Fatal("the sign-in stayed")
	}
}

func TestAChatThatStillHasItsCLIKeepsItsSignIn(t *testing.T) {
	on := func(profile string, k wire.AgentStateKind) wire.Conversation {
		return wire.Conversation{Model: &wire.Model{Profile: profile}, State: wire.AgentState{K: k}}
	}
	if liveOn("cx-a", []wire.Conversation{{}, on("cx-b", wire.StateWorking), on("cx-a", wire.StateStopped), on("cx-a", wire.StateGone), on("cx-a", wire.StateLaunchFailed)}) {
		t.Fatal("chats without a CLI hold the sign-in")
	}
	for _, k := range []wire.AgentStateKind{wire.StateStarting, wire.StateWorking, wire.StateIdle, wire.StateReady, wire.StateUnknown} {
		if !liveOn("cx-a", []wire.Conversation{on("cx-a", k)}) {
			t.Fatalf("a chat that is %s does not hold the sign-in", k)
		}
	}
}
