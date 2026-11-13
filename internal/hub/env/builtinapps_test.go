package env_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/env"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

var appsAgent = []string{hubproto.FeatureEnvironment, hubproto.FeatureEnvSetup, hubproto.FeatureEnvApps}

// sendBuiltIn is the list a new machine of anna gets from a hub with these platform apps and addresses; ok is false
// when nothing was sent.
func sendBuiltIn(t *testing.T, a acme, apps []env.BuiltInApp, install map[string]string, features []string) (hubproto.EnvSet, bool) {
	t.Helper()
	return sendBuiltInWith(t, a, apps, install, features, []string{"env"})
}

// sendBuiltInWith is sendBuiltIn on a hub whose organizations have these parts of the environment on.
func sendBuiltInWith(t *testing.T, a acme, apps []env.BuiltInApp, install map[string]string, features, on []string) (hubproto.EnvSet, bool) {
	t.Helper()
	m := a.anna.Machine(a.tenant, nil)
	link := &fakeLink{id: m.MachineID, status: "ok"}
	p := env.NewPusher(env.PushDeps{DB: a.s.DB, Reg: link, Features: env.StaticFeatures(on), Now: a.s.Clock.Now, Every: time.Hour,
		Apps: apps, Install: install})
	defer p.Close()
	p.Connected(m.MachineID, hubproto.Hello{Features: features})
	if err := p.Push(context.Background()); err != nil {
		t.Fatal(err)
	}
	if link.count() == 0 {
		return hubproto.EnvSet{}, false
	}
	return link.sent[0], true
}

func appsOf(set hubproto.EnvSet) []hubproto.EnvItem {
	var out []hubproto.EnvItem
	for _, it := range set.Items {
		if it.Kind == hubproto.EnvKindApp {
			out = append(out, it)
		}
	}
	return out
}

func TestBuiltInAppsComeWithTheAddressOfTheirService(t *testing.T) {
	a := newAcme(t)
	apps := env.BuiltInApps(env.BuiltInAppSource{}, env.BuiltInAppSource{RepoURL: "https://git.example.org/mirror/docs", Branch: "v1.4.0"})

	// No address of a service: no app, and nothing to send to a machine of an empty organization.
	if set, ok := sendBuiltIn(t, a, apps, env.InstallValues("", ""), appsAgent); ok {
		t.Fatalf("without addresses: %+v", set.Items)
	}
	// The boards service only: «Доски» for everybody, not pinned, from the public repository.
	set, _ := sendBuiltIn(t, a, apps, env.InstallValues("https://boards.example", ""), appsAgent)
	got := appsOf(set)
	if len(got) != 1 || got[0].ItemID != "builtin-app-boards" || got[0].Name != "Доски" || got[0].Pinned || got[0].Layer != "org" {
		t.Fatalf("boards only: %+v", got)
	}
	if src := string(got[0].Source); src != `{"branch":"main","from":"repo","install":{"boardsUrl":"https://boards.example"},"path":"harnessly-app","repoUrl":"https://github.com/ly-solutions/boards"}` {
		t.Errorf("source = %s", src)
	}
	// Both, the documents' package from the hub's own place.
	set, _ = sendBuiltIn(t, a, apps, env.InstallValues("https://boards.example", "https://docs.example"), appsAgent)
	got = appsOf(set)
	if len(got) != 2 || got[0].Name != "Документы" || !strings.Contains(string(got[0].Source), `"branch":"v1.4.0"`) ||
		!strings.Contains(string(got[0].Source), `"repoUrl":"https://git.example.org/mirror/docs"`) {
		t.Fatalf("both: %+v", got)
	}
	// The hub's switch: no platform apps at all.
	if set, ok := sendBuiltIn(t, a, nil, env.InstallValues("https://boards.example", "https://docs.example"), appsAgent); ok {
		t.Fatalf("with the apps off: %+v", set.Items)
	}
}

// An agent that does not install apps gets the list, and its hash, a hub without the platform's apps would send.
func TestBuiltInAppsDoNotChangeTheListOfAnOlderAgent(t *testing.T) {
	a := newAcme(t)
	skill := a.item(t, "skill", "Ревью кода", map[string]any{"body": "Пиши коротко."})
	if code, _ := a.entry(t, a.anna, "org", a.org, skill, nil); code != 201 {
		t.Fatalf("entry: %d", code)
	}
	install := env.InstallValues("https://boards.example", "https://docs.example")
	apps := env.BuiltInApps(env.BuiltInAppSource{}, env.BuiltInAppSource{})
	for _, old := range [][]string{{hubproto.FeatureEnvironment}, {hubproto.FeatureEnvironment, hubproto.FeatureEnvSetup, hubproto.FeatureEnvClaude}} {
		before, ok := sendBuiltIn(t, a, nil, nil, old)
		after, ok2 := sendBuiltIn(t, a, apps, install, old)
		if !ok || !ok2 || before.Hash != after.Hash || len(after.Items) != 1 || len(appsOf(after)) != 0 {
			t.Fatalf("%v: before %+v, after %+v", old, before, after)
		}
	}
	if set, _ := sendBuiltIn(t, a, apps, install, appsAgent); len(appsOf(set)) != 2 {
		t.Fatalf("a new agent: %+v", set.Items)
	}
}

// The organization's own item of the same package wins: the machine gets one app, with the organization's pin.
func TestTheOrganizationsOwnItemReplacesTheBuiltInApp(t *testing.T) {
	a := newAcme(t)
	install := env.InstallValues("https://boards.example", "https://docs.example")
	apps := env.BuiltInApps(env.BuiltInAppSource{}, env.BuiltInAppSource{})
	own := a.item(t, "app", "Доски команды", map[string]any{"repoUrl": "https://GitHub.com/ly-solutions/boards.git", "branch": "main", "path": "harnessly-app"})
	if code, _ := a.entry(t, a.anna, "org", a.org, own, map[string]any{"pinned": true}); code != 201 {
		t.Fatalf("entry: %d", code)
	}
	// Another folder of the documents' repository is another app: it does not replace «Документы».
	other := a.item(t, "app", "Шаблоны", map[string]any{"repoUrl": "https://github.com/ly-solutions/artifacts", "path": "templates"})
	if code, _ := a.entry(t, a.anna, "org", a.org, other, nil); code != 201 {
		t.Fatalf("entry: %d", code)
	}
	set, _ := sendBuiltIn(t, a, apps, install, appsAgent)
	ids := map[string]bool{}
	for _, it := range appsOf(set) {
		ids[it.ItemID] = it.Pinned
	}
	if len(ids) != 3 || !ids[own] || ids["builtin-app-boards"] {
		t.Fatalf("apps = %+v", appsOf(set))
	}
	if _, docs := ids["builtin-app-docs"]; !docs {
		t.Fatalf("«Документы» left with an item of another folder: %+v", appsOf(set))
	}
	if _, boards := ids["builtin-app-boards"]; boards {
		t.Fatalf("two items of one app: %+v", appsOf(set))
	}
}

func TestTheOrganizationTurnsABuiltInAppOffForEverybody(t *testing.T) {
	a := newAcme(t, hubtest.Env("HUB_BOARDS_URL", "https://boards.example"), hubtest.Env("HUB_BOARDS_TOKEN", "boards-token"))
	install := env.InstallValues("https://boards.example", "")
	apps := env.BuiltInApps(env.BuiltInAppSource{}, env.BuiltInAppSource{})
	row := func() env.EntryItem {
		t.Helper()
		for _, it := range layer(a.layers(t, a.anna, ""), "org", a.org).Items {
			if it.ItemID == "builtin-app-boards" {
				return it
			}
		}
		t.Fatal("no built-in row")
		return env.EntryItem{}
	}
	r := row()
	if !r.BuiltIn || r.Off || !r.Enabled || r.Pinned || r.LockedReason == nil || *r.LockedReason != "built_in" {
		t.Fatalf("row = %+v", r)
	}
	for _, it := range layer(a.layers(t, a.anna, ""), "org", a.org).Items {
		if it.ItemID == "builtin-app-docs" {
			t.Fatal("«Документы» without the address of its service")
		}
	}

	// A person sees it in their environment; the hub's personal switch is not for a platform's item.
	var me env.MeResponse
	a.vera.Do("GET", "/api/me/env?space="+a.tenant, nil).JSON(&me)
	if len(me.Items) != 1 || me.Items[0].ItemID != "builtin-app-boards" || me.Items[0].Off || me.Items[0].CanToggle {
		t.Fatalf("me = %+v", me.Items)
	}
	if r := a.vera.Do("PUT", "/api/me/env/builtin-app-boards?space="+a.tenant, map[string]any{"off": true}); r.Code != 409 || r.Detail("reason") != "built_in" {
		t.Errorf("the personal switch: %s", r)
	}

	off := map[string]any{"source": map[string]any{"off": true}}
	if r := a.boris.Do("PATCH", a.url("/catalog/builtin-app-boards"), off); r.Code != 403 {
		t.Errorf("a team's lead turned it off: %s", r)
	}
	if r := a.anna.Do("PATCH", a.url("/catalog/builtin-app-boards"), map[string]any{"name": "Моё", "source": map[string]any{"off": true}}); r.Code != 409 {
		t.Errorf("a built-in item renamed: %s", r)
	}
	if r := a.anna.Do("PATCH", a.url("/catalog/builtin-app-docs"), off); r.Code != 404 {
		t.Errorf("an app the hub has no service for: %s", r)
	}
	if r := a.anna.Do("PATCH", a.url("/catalog/builtin-app-boards"), off); r.Code != 200 {
		t.Fatalf("off: %s", r)
	}
	if r := row(); !r.Off {
		t.Fatalf("row after off = %+v", r)
	}
	// The machines get the item with `off`, as with an organization's own app: the app stays installed and is not there.
	set, _ := sendBuiltIn(t, a, apps, install, appsAgent)
	if got := appsOf(set); len(got) != 1 || !strings.Contains(string(got[0].Source), `"off":true`) {
		t.Fatalf("off on a machine: %+v", got)
	}
	if r := a.anna.Do("PATCH", a.url("/catalog/builtin-app-boards"), map[string]any{"source": map[string]any{"off": false}}); r.Code != 200 {
		t.Fatalf("on: %s", r)
	}
	set, _ = sendBuiltIn(t, a, apps, install, appsAgent)
	if got := appsOf(set); len(got) != 1 || strings.Contains(string(got[0].Source), "off") || row().Off {
		t.Fatalf("on again: %+v", got)
	}
}

func TestTheHubsSwitchLeavesTheBuiltInAppsOut(t *testing.T) {
	a := newAcme(t, hubtest.Env("HUB_BOARDS_URL", "https://boards.example"), hubtest.Env("HUB_BOARDS_TOKEN", "boards-token"),
		hubtest.Env("HUB_ENV_APPS", "off"))
	if items := layer(a.layers(t, a.anna, ""), "org", a.org).Items; len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

// An organization without «Окружение» gets the platform's apps and nothing else of the environment.
func TestBuiltInAppsReachAnOrganizationWithoutTheEnvironment(t *testing.T) {
	a := newAcme(t)
	skill := a.item(t, "skill", "Ревью кода", map[string]any{"body": "Пиши коротко."})
	if code, _ := a.entry(t, a.anna, "org", a.org, skill, nil); code != 201 {
		t.Fatalf("entry: %d", code)
	}
	install := env.InstallValues("https://boards.example", "https://docs.example")
	apps := env.BuiltInApps(env.BuiltInAppSource{}, env.BuiltInAppSource{})
	set, ok := sendBuiltInWith(t, a, apps, install, appsAgent, nil)
	if !ok || len(set.Items) != 2 || len(appsOf(set)) != 2 {
		t.Fatalf("the apps alone: %+v", set.Items)
	}
	// Nothing for an agent that does not install apps, and nothing without the platform's apps: as before.
	if set, ok := sendBuiltInWith(t, a, apps, install, []string{hubproto.FeatureEnvironment, hubproto.FeatureEnvSetup}, nil); ok {
		t.Fatalf("an older agent: %+v", set.Items)
	}
	if set, ok := sendBuiltInWith(t, a, nil, install, appsAgent, nil); ok {
		t.Fatalf("with the apps off: %+v", set.Items)
	}
}
