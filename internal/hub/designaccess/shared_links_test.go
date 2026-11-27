package designaccess

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// A link belongs to the project, not to the machine its maker's page ran over: with a shared library the owner sees,
// changes and revokes it from any machine of the space.
func TestALinkMadeOverAnotherMachineIsTheProjects(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	if done, err := r.svc.registerCreated(ctx, r.owner, "site", r.ownerU.Email); err != nil || !done {
		t.Fatal(done, err)
	}
	var link shareRow
	if err := r.svc.write(ctx, r.mate, func(tx *txn) error {
		if _, err := tx.upsertMember("site", r.ownerU.Email, r.mateU.Email, roleEdit, nil, ""); err != nil {
			return err
		}
		var err error
		link, _, err = tx.createShare("site", r.mateU.Email, shareInput{Rights: roleView, Version: "latest", TTL: ttl{none: true}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read, err := r.svc.read(ctx, r.owner)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := read.shareRows("site")
	if err != nil || len(rows) != 1 || rows[0].ID != link.ID {
		t.Fatalf("the owner's machine lists %+v %v", rows, err)
	}
	if _, err := read.shareByID("site", link.ID); err != nil {
		t.Errorf("by id: %v", err)
	}
	if _, _, err := read.shareAddress("site", link.ID); err != nil {
		t.Errorf("address: %v", err)
	}
	if _, err := read.shareViews("site", link.ID); err != nil {
		t.Errorf("views: %v", err)
	}
	if _, err := read.listGuestComments("site"); err != nil {
		t.Errorf("comments: %v", err)
	}
	title := "renamed"
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error {
		if _, _, _, err := tx.shareState("site", link.ID); err != nil {
			return err
		}
		if _, err := tx.updateShare("site", r.ownerU.Email, link.ID, shareChange{Title: &title}); err != nil {
			return err
		}
		if _, err := tx.reissueShare("site", r.ownerU.Email, link.ID); err != nil {
			return err
		}
		if err := tx.revokeShare("site", r.ownerU.Email, link.ID); err != nil {
			return err
		}
		return tx.enableShare("site", r.ownerU.Email, link.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error { return tx.revokeShare("site", r.ownerU.Email, link.ID) }); err != nil {
		t.Fatalf("revoking the enabled link: %v", err)
	}
}

// A person of another space reads a variant's missing files only from a source that names them: a source's team level
// is not theirs.
func TestAForeignVariantTakesNoFilesThroughATeamLevel(t *testing.T) {
	ctx := context.Background()
	f := newForeignRig(t)
	if _, err := f.svc.registerCreated(ctx, f.owner, "plan-variant", f.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.write(ctx, f.owner, func(tx *txn) error {
		_, err := tx.upsertMember("plan-variant", f.ownerU.Email, f.outU.Email, roleView, nil, "")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.S().Q().Exec(ctx, `UPDATE design_projects SET home_team = NULL, team_level = 'view' WHERE project_id = ?`, "plan"); err != nil {
		t.Fatal(err)
	}
	f.svc.UseView(&fileLibrary{
		fakeLibrary: fakeLibrary{
			heads: map[string]int64{"plan": 3, "plan-variant": 4},
			files: map[string][]byte{"plan-variant/variant.json": []byte(`{"v":1,"of":"plan"}`)},
		},
		served: map[string]string{"plan/support.js": "orig()", "plan-variant/own.js": "mine()"},
	})
	card := "?card=" + cardKey(f.owner.ID, "plan-variant")
	get := func(file string) (int, string) {
		w := f.serve(t, http.MethodGet, "/design/store/plan-variant/f/"+file+card)
		return w.Code, w.Body.String()
	}
	if code, body := get("own.js"); code != http.StatusOK {
		t.Fatalf("the variant itself: %d %s", code, body)
	}
	if code, body := get("support.js"); code != http.StatusNotFound || strings.Contains(body, "orig") {
		t.Errorf("the source's file: %d %q", code, body)
	}
	if err := f.svc.write(ctx, f.owner, func(tx *txn) error { return tx.setTeam("plan", f.ownerU.Email, levelNone) }); err != nil {
		t.Fatal(err)
	}
	f.grant(t, roleView)
	if code, body := get("support.js"); code != http.StatusOK || body != "orig()" {
		t.Errorf("a source that names them: %d %q", code, body)
	}
}
