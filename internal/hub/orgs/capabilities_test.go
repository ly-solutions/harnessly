package orgs_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/capabilities"
	"github.com/ly-solutions/harnessly/internal/hub/orgs"
)

func capability(t *testing.T, out orgs.CapabilitiesResponse, id string) capabilities.Item {
	t.Helper()
	for _, c := range out.Capabilities {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no capability %q", id)
	return capabilities.Item{}
}

// The list is for the organization's admin (and the installation's): a member and an outsider get nothing, and the list
// says what is on, where it is set and who may toggle it.
func TestCapabilitiesAreTheAdminsAndShowWhereEachIsSet(t *testing.T) {
	a := newAcme(t)
	// A member and an outsider do not read it; the organization's admin and the installation's do.
	if r := a.boris.Do("GET", a.url("/capabilities"), nil); r.Code != http.StatusForbidden {
		t.Errorf("member: %s", r)
	}
	if r := a.s.SignIn("stranger@example.com").Do("GET", a.url("/capabilities"), nil); r.Code != http.StatusNotFound {
		t.Errorf("outsider: %s", r)
	}
	if r := a.s.Browser().Do("GET", a.url("/capabilities"), nil); r.Code != http.StatusUnauthorized {
		t.Errorf("signed out: %s", r)
	}
	var out orgs.CapabilitiesResponse
	r := a.anna.Do("GET", a.url("/capabilities"), nil)
	if r.Code != http.StatusOK {
		t.Fatalf("admin: %s", r)
	}
	r.JSON(&out)
	if out.CanChange || len(out.Capabilities) == 0 || len(out.Groups) == 0 {
		t.Fatalf("admin of the organization: canChange %v, %d items, groups %v", out.CanChange, len(out.Capabilities), out.Groups)
	}
	// Settings of the hub are shown and never toggled; the routines are on by default.
	if c := capability(t, out, "HUB_AUTOMATIONS"); !c.Enabled || c.Source != capabilities.Env || c.CanToggle || c.SetBy != "HUB_AUTOMATIONS" || !c.DefaultOn {
		t.Errorf("automations = %+v", c)
	}
	if c := capability(t, out, "HUB_DOCS"); c.Enabled || c.Needs == "" || c.Why == "" {
		t.Errorf("docs = %+v", c)
	}
	// A rollout switch with no row: on by default for the schedule, off for the rest; neither is toggled by this admin.
	if c := capability(t, out, "automation-triggers"); !c.Enabled || c.Source != capabilities.Rollout || c.CanToggle {
		t.Errorf("triggers = %+v", c)
	}
	if c := capability(t, out, "hub-search"); c.Enabled {
		t.Errorf("search = %+v", c)
	}
	// An organization's flag: the two built-in ones are on, the rest off.
	if c := capability(t, out, "org.events"); !c.Enabled || c.Source != capabilities.Org {
		t.Errorf("org.events = %+v", c)
	}
	if c := capability(t, out, "env"); c.Enabled {
		t.Errorf("env = %+v", c)
	}

	// The administrator of the installation may toggle a flag and a switch; what the instance forces stays shut.
	root := a.s.SignIn("root@example.com")
	var rootOut orgs.CapabilitiesResponse
	root.Do("GET", a.url("/capabilities"), nil).JSON(&rootOut)
	if !rootOut.CanChange || !capability(t, rootOut, "env").CanToggle || !capability(t, rootOut, "automation-triggers").CanToggle || capability(t, rootOut, "HUB_AUTOMATIONS").CanToggle {
		t.Errorf("instance admin: %+v", rootOut.CanChange)
	}
	// What a toggle does through the existing endpoints shows in the list at once.
	if r := root.Do("PUT", a.url("/features/env"), map[string]any{"enabled": true}); r.Code != http.StatusOK {
		t.Fatal(r)
	}
	if r := root.Do("PUT", "/api/admin/switches", map[string]any{"feature": "automation-triggers", "enabled": false}); r.Code != http.StatusNoContent {
		t.Fatal(r)
	}
	a.anna.Do("GET", a.url("/capabilities"), nil).JSON(&out)
	if !capability(t, out, "env").Enabled || capability(t, out, "automation-triggers").Enabled {
		t.Errorf("after the toggles: env %+v, triggers %+v", capability(t, out, "env"), capability(t, out, "automation-triggers"))
	}
}

// The web's fixture of the answer has the keys the hub sends.
func TestCapabilitiesMatchWebFixture(t *testing.T) {
	a := newAcme(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "app", "hub-api", "fixtures", "orgs", "capabilities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	a.anna.Do("GET", a.url("/capabilities"), nil).JSON(&got)
	if keys(got) != keys(want) {
		t.Errorf("keys %s, fixture %s", keys(got), keys(want))
	}
	// An item always has these; the rest (detail, needs, why, forced, setBy) come only when there is something to say.
	always := []string{"canToggle", "defaultOn", "description", "enabled", "group", "id", "name", "source"}
	for _, list := range []map[string]any{got, want} {
		items, _ := list["capabilities"].([]any)
		if len(items) == 0 {
			t.Fatal("no items")
		}
		for _, it := range items {
			m := it.(map[string]any)
			for _, k := range always {
				if _, ok := m[k]; !ok {
					t.Errorf("item %v lacks %s", m["id"], k)
				}
			}
		}
	}
}
