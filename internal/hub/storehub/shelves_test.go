package storehub

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/filesync"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// A person's shelf names their projects and folders: it reaches the machines it concerns, never the whole space.
func TestShelvesReachOnlyTheirPeople(t *testing.T) {
	ctx := context.Background()
	ownerKey, mateKey, guestKey := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	shelf := func(k string) string { return ".folders/" + k + ".json" }
	machines := fakeMachines{
		"owner":  {ID: "owner", Name: "vm-owner", SpaceID: "sp1"},
		"mate":   {ID: "mate", Name: "vm-mate", SpaceID: "sp1"},
		"shared": {ID: "shared", Name: "vm-shared", SpaceID: "sp1"},
	}
	design := &fakeDesign{view: map[string]map[string]Grant{}, shelves: map[string][]string{
		"owner": {ownerKey}, "mate": {mateKey},
	}}
	_, client := designStore(t, machines, design)
	owner, mate, shared := client("owner"), client("mate"), client("shared")
	put := func(c *filesync.Client, path, body string) error {
		_, err := c.Put(ctx, path, []byte(body), "t")
		return err
	}
	seen := func(c *filesync.Client) string {
		t.Helper()
		ch, err := c.Changes(ctx, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range ch.Entries {
			out = append(out, e.Path)
		}
		return strings.Join(out, " ")
	}
	if err := put(owner, shelf(ownerKey), `{"folders":[]}`); err != nil {
		t.Fatal(err)
	}
	// A shared machine keeps what it writes itself for a person it does not name.
	if err := put(shared, shelf(guestKey), `{"folders":[]}`); err != nil {
		t.Fatal(err)
	}
	if got := seen(mate); got != "" {
		t.Fatalf("the mate's machine received %s", got)
	}
	if got := seen(owner); got != shelf(ownerKey) {
		t.Fatalf("the owner's machine sees %q", got)
	}
	if got := seen(shared); got != shelf(guestKey) {
		t.Fatalf("the shared machine sees %q", got)
	}
	if e, err := mate.Current(ctx, shelf(ownerKey)); err != nil || e != nil {
		t.Fatalf("another's shelf answers %+v %v", e, err)
	}
	for _, p := range []string{shelf(ownerKey), shelf(guestKey)} {
		if err := put(mate, p, "mine now"); !errors.Is(err, filesync.ErrRefused) {
			t.Fatalf("a write over another's shelf %s: %v", p, err)
		}
	}
	// A second machine under the same name proves nothing by it.
	machines["twin"] = store.Machine{ID: "twin", Name: "vm-shared", SpaceID: "sp1"}
	if got := seen(client("twin")); got != "" {
		t.Fatalf("a machine named like the shared one received %s", got)
	}
}

func (f fakeMachines) MachinesBySpace(_ context.Context, space string) ([]store.Machine, error) {
	var out []store.Machine
	for _, m := range f {
		if m.SpaceID == space {
			out = append(out, m)
		}
	}
	return out, nil
}
