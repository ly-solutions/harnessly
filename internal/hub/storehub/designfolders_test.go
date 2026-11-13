package storehub

import (
	"context"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/filesync"
)

// GitHub #83: with closed unrecorded projects a folder the hub has no row for reaches no other machine, a new project
// can still start (its owner file records it), and the registration finds every folder with what it needs.
func TestClosedUnrecordedProjectsStayOnTheirMachine(t *testing.T) {
	ctx := context.Background()
	machines := fakeMachines{
		"owner": {ID: "owner", Name: "vm-owner", SpaceID: "sp1"},
		"mate":  {ID: "mate", Name: "vm-mate", SpaceID: "sp1"},
	}
	design := &fakeDesign{closed: true, view: map[string]map[string]Grant{
		"owner": {"secret": {See: true, Edit: true, Own: true}},
		"mate":  {"secret": {See: false}},
	}}
	st, client := designStore(t, machines, design)
	owner, mate := client("owner"), client("mate")
	put := func(c *filesync.Client, path, body string) {
		t.Helper()
		if _, err := c.Put(ctx, path, []byte(body), "t"); err != nil {
			t.Fatalf("put %s: %v", path, err)
		}
	}
	put(owner, "legacy/project/Main.dc.html", "made before access")
	put(owner, "secret/project/Main.dc.html", "v1")

	ch, err := mate.Changes(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ch.Entries {
		if strings.HasPrefix(e.Path, "legacy/") || strings.HasPrefix(e.Path, "secret/") {
			t.Fatalf("the mate's machine received %s", e.Path)
		}
	}
	// A new project still starts on the mate's machine: its owner file records it.
	put(mate, "fresh/.owner.json", `{"v":2,"user":"coder","name":"Mate","person":"mate@example.com","private":true}`)
	design.mu.Lock()
	recorded := append([]string(nil), design.recorded...)
	design.mu.Unlock()
	if len(recorded) != 1 || recorded[0] != "mate:fresh:mate@example.com" {
		t.Fatalf("recorded %v", recorded)
	}

	// Opening the unrecorded projects again changes the scope: the machines read the library again and get them.
	design.mu.Lock()
	design.closed = false
	design.mu.Unlock()
	st.forgetViews("")
	open, err := mate.Changes(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if open.Scope == ch.Scope {
		t.Fatal("the scope did not change with the rule for unrecorded projects")
	}
	found := false
	for _, e := range open.Entries {
		found = found || e.Path == "legacy/project/Main.dc.html"
	}
	if !found {
		t.Fatalf("an open unrecorded project did not reach the machine: %+v", open.Entries)
	}

	spaces, err := st.DesignSpaces(ctx)
	if err != nil || len(spaces) != 1 || spaces[0] != "sp1" {
		t.Fatalf("design spaces %v %v", spaces, err)
	}
	folders, err := st.DesignFolders(ctx, "sp1")
	if err != nil || len(folders) != 3 {
		t.Fatalf("folders %+v %v", folders, err)
	}
	byID := map[string]DesignFolder{}
	for _, f := range folders {
		byID[f.ID] = f
	}
	if f := byID["legacy"]; f.Member != "vm-owner" || f.OwnerFile != nil {
		t.Fatalf("legacy folder %+v", f)
	}
	if f := byID["fresh"]; f.Member != "vm-mate" || !strings.Contains(string(f.OwnerFile), "mate@example.com") {
		t.Fatalf("fresh folder %+v", f)
	}
}
