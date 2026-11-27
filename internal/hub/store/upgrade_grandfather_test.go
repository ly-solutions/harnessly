package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// An installation upgraded to the stricter rules of HRN-1785 and HRN-1784 goes on as it was: the migrations write down
// who may keep doing what, and only what comes afterwards is asked.
func TestUpgradeKeepsWhatAnInstallationHadSQLite(t *testing.T) {
	url := "sqlite://" + filepath.Join(t.TempDir(), "hub.db")
	upgradeKeepsWhatItHad(t, url, Options{})
}

func TestUpgradeKeepsWhatAnInstallationHadPostgres(t *testing.T) {
	dsn := envURL(t, "HUB_TEST_POSTGRES_URL")
	_, schema := freshSchema(t, dsn)
	upgradeKeepsWhatItHad(t, dsn, Options{Schema: schema, Check: CheckStrict})
}

func upgradeKeepsWhatItHad(t *testing.T, url string, opts Options) {
	t.Helper()
	ctx := context.Background()
	before := opts
	before.Last, before.Check = 89, ""
	old, err := OpenWith(ctx, url, before)
	if err != nil {
		t.Fatal(err)
	}
	s := old.S()
	for _, id := range []string{"admin", "alice", "bob", "ruler", "late"} {
		if err := s.CreateUser(ctx, &User{ID: id, Email: id + "@example.com"}); err != nil {
			t.Fatal(err)
		}
	}
	org := &Org{Slug: "acme", Name: "Acme"}
	if err := s.CreateOrg(ctx, org); err != nil {
		t.Fatal(err)
	}
	inside := &Space{Slug: "inside", Name: "Inside", CreatedBy: "admin"}
	outside := &Space{Slug: "outside", Name: "Outside", CreatedBy: "admin"}
	for _, sp := range []*Space{inside, outside} {
		if err := s.CreateSpace(ctx, sp); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetSpaceOrg(ctx, inside.ID, org.ID); err != nil {
		t.Fatal(err)
	}
	// As the releases before wrote them: a machine of its connector, one handed over, one handed over and nobody's now.
	for _, m := range []Machine{
		{ID: "own", CreatedBy: "alice"},
		{ID: "given", CreatedBy: "alice", ConnectedBy: "admin"},
		{ID: "back", CreatedBy: "admin", ConnectedBy: "admin"},
		{ID: "orphan", ConnectedBy: "admin"},
	} {
		m.SpaceID, m.Name, m.KeyPub = inside.ID, m.ID, "key-"+m.ID
		if err := s.CreateMachine(ctx, &m); err != nil {
			t.Fatal(err)
		}
	}
	for i, owner := range []string{"ruler", "ruler", "bob"} {
		if _, err := old.Exec(ctx, `INSERT INTO automation_rules (id, owner_id, name, trigger, prompt_template, created_at, updated_at) VALUES (?, ?, ?, '{}', 'p', 1, 1)`,
			[]string{"r1", "r2", "r3"}[i], owner, []string{"a", "b", "c"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenWith(ctx, url, opts)
	if err != nil {
		t.Fatalf("the upgrade: %v", err)
	}
	defer db.Close()
	s = db.S()

	// Machines: whoever owns one now keeps their data on it; nobody's gets no holder who could be revoked by leaving.
	for id, holder := range map[string]string{"own": "alice", "given": "alice", "back": "admin", "orphan": "-"} {
		m, err := s.MachineByID(ctx, id)
		if err != nil || m.Holder() != holder {
			t.Errorf("%s is held by %q, want %q (%v)", id, m.Holder(), holder, err)
		}
		if m.CreatedBy != "" {
			if st, err := s.MachinePerson(ctx, m); err != nil || st != PersonOK {
				t.Errorf("%s: its owner's data: %v (%v)", id, st, err)
			}
		}
	}
	// The old connector leaving takes neither the machine handed over before the upgrade nor the ownerless one.
	gone, err := s.RevokeMachinesOf(ctx, inside.ID, "admin", time.Now())
	if err != nil || len(gone) != 1 || gone[0].ID != "back" {
		t.Errorf("revoked with the old connector: %+v (%v), want only their own", gone, err)
	}
	// A transfer after the upgrade asks the new owner.
	if err := s.SetMachineOwner(ctx, "given", "bob"); err != nil {
		t.Fatal(err)
	}
	m, err := s.MachineByID(ctx, "given")
	if st, serr := s.MachinePerson(ctx, m); err != nil || serr != nil || st != PersonRelink || m.Holder() != "alice" {
		t.Errorf("handed over after the upgrade: %v held by %q (%v %v)", st, m.Holder(), err, serr)
	}
	if ok, err := s.AcceptMachineKeys(ctx, "given", "alice"); err != nil || ok {
		t.Errorf("somebody who does not own the machine accepted it: %v %v", ok, err)
	}

	for id, want := range map[string]bool{"ruler": true, "bob": true, "alice": false, "late": false} {
		if got, err := s.AutomationOperator(ctx, id); err != nil || got != want {
			t.Errorf("%s keeps the operator's rights: %v, want %v (%v)", id, got, want, err)
		}
	}

	later := &Org{Slug: "later", Name: "Later"}
	if err := s.CreateOrg(ctx, later); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{org.ID: true, "space:" + outside.ID: true, "space:" + inside.ID: false, later.ID: false} {
		if listed, any, err := s.SafeRefTenant(ctx, key); err != nil || listed != want || !any {
			t.Errorf("%s keeps the reference folders: %v (any %v), want %v (%v)", key, listed, any, want, err)
		}
	}
}
