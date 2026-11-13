package designaccess

import (
	"context"
	"strings"
	"testing"
	"time"
)

// GitHub #83: a deployment whose people expect personal isolation makes new projects personal
// (HUB_DESIGN_DEFAULT_TEAM_LEVEL=none), registers the projects the hub has no row for, and then closes them
// (HUB_DESIGN_UNRECORDED=closed).
func TestClosedDeploymentRegistersAndClosesUnrecordedProjects(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	now := func() time.Time { return time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC) }
	open := r.svc
	r.svc = New(Deps{DB: r.db, Now: now, Shared: true, TeamLevel: "none", ClosedUnrecorded: true})

	// A new project is personal: the mate's machine does not get it.
	if done, err := r.svc.registerCreated(ctx, r.owner, "fresh", r.ownerU.Email); err != nil || !done {
		t.Fatalf("registerCreated: %v %v", done, err)
	}
	if v, err := r.svc.StoreView(ctx, r.mate); err != nil || v["fresh"].See {
		t.Fatalf("the mate's view of a new project: %+v %v", v, err)
	}
	if v, err := r.svc.StoreView(ctx, r.owner); err != nil || !v["fresh"].Own {
		t.Fatalf("the owner's view of a new project: %+v %v", v, err)
	}

	// A project without a row gives nobody a role; the open hub gives everyone «edit».
	roleOf := func(svc *Service, id, email string) string {
		t.Helper()
		tx, err := svc.read(ctx, r.mate)
		if err != nil {
			t.Fatal(err)
		}
		p, stored, err := tx.effective(id)
		if err != nil || stored {
			t.Fatalf("effective %s: %v %v", id, stored, err)
		}
		f, err := tx.fold(p, email)
		if err != nil {
			t.Fatal(err)
		}
		return f.Role
	}
	if got := roleOf(r.svc, "legacy", r.mateU.Email); got != "" {
		t.Fatalf("closed: an unrecorded project gives %q", got)
	}
	if got := roleOf(open, "legacy", r.mateU.Email); got != roleEdit {
		t.Fatalf("open: an unrecorded project gives %q", got)
	}
	if snap, err := r.svc.build(ctx, r.mate); err != nil || snap.Unrecorded != "closed" {
		t.Fatalf("the snapshot does not tell the runtime: %q %v", snap.Unrecorded, err)
	}
	if snap, err := open.build(ctx, r.mate); err != nil || snap.Unrecorded != "" {
		t.Fatalf("an open hub's snapshot: %q %v", snap.Unrecorded, err)
	}

	folders := []Folder{
		{ID: "legacy", Member: "vm-owner"},
		{ID: "byfile", Member: "vm-owner", OwnerFile: []byte(`{"v":1,"user":"x","person":"Mate@Example.com"}`)},
		{ID: "gone", Member: "vm-gone"},
		{ID: "fresh", Member: "vm-owner"},
	}
	dry, err := r.svc.RegisterUnrecorded(ctx, r.owner.SpaceID, folders, true)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ProjectRegister{}
	for _, p := range dry.Projects {
		got[p.ID] = p
	}
	if p := got["legacy"]; p.Outcome != "registered" || p.Owner != r.ownerU.Email || p.Source != "machine" || !p.Personal() {
		t.Fatalf("legacy: %+v", p)
	}
	if p := got["byfile"]; p.Outcome != "registered" || p.Owner != r.mateU.Email || p.Source != "owner file" {
		t.Fatalf("byfile: %+v", p)
	}
	if p := got["gone"]; p.Outcome != "skipped" {
		t.Fatalf("gone: %+v", p)
	}
	if p := got["fresh"]; p.Outcome != "recorded" {
		t.Fatalf("fresh: %+v", p)
	}
	if !dry.Failed() || !strings.Contains(dry.Summary(), "would register legacy: owner owner@example.com (from the machine), personal") ||
		!strings.Contains(dry.Summary(), "2 personal, 0 team, 1 already recorded, 1 skipped") {
		t.Fatalf("dry-run report:\n%s", dry.Summary())
	}
	if tx, err := r.svc.read(ctx, r.owner); err != nil {
		t.Fatal(err)
	} else if p, err := tx.getProject("legacy", false); err != nil || p != nil {
		t.Fatalf("a dry run wrote a row: %+v %v", p, err)
	}

	applied, err := r.svc.RegisterUnrecorded(ctx, r.owner.SpaceID, folders[:2], false)
	if err != nil || applied.Failed() {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	v, err := r.svc.StoreView(ctx, r.mate)
	if err != nil {
		t.Fatal(err)
	}
	if v["legacy"].See || !v["byfile"].Own {
		t.Fatalf("the mate's view after registering: %+v", v)
	}
	again, err := r.svc.RegisterUnrecorded(ctx, r.owner.SpaceID, folders[:2], false)
	if err != nil || again.count("recorded") != 2 {
		t.Fatalf("a second run: %+v %v", again, err)
	}
}

// The default keeps today's behaviour: a new project opens to its maker's team at «edit».
func TestDefaultTeamLevelIsEdit(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	if _, err := r.svc.registerCreated(ctx, r.owner, "fresh", r.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	tx, err := r.svc.read(ctx, r.owner)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := tx.getProject("fresh", false); err != nil || p == nil || p.TeamLevel != roleEdit {
		t.Fatalf("a new project: %+v %v", p, err)
	}
	if New(Deps{TeamLevel: "bogus"}).teamLevel != roleEdit {
		t.Fatal("an unknown level is not edit")
	}
}
