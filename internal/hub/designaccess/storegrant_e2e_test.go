package designaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/design"
	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/filesync"
	"github.com/ly-solutions/harnessly/internal/hub/config"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hub/storehub"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// GitHub #193: A (owner) gives B (same space, own machine) direct Edit on a project with the hub's store on. B's machine
// must get the project's files through the store and its runtime library must list it, editable.

type e2eMachines struct{ db *store.DB }

func (e e2eMachines) MachineByID(ctx context.Context, id string) (store.Machine, error) {
	return e.db.S().MachineByID(ctx, id)
}
func (e e2eMachines) SpaceBySlug(context.Context, string) (store.Space, error) {
	return store.Space{}, store.ErrNotFound
}
func (e e2eMachines) MachinePerson(context.Context, store.Machine) (store.PersonState, error) {
	var z store.PersonState
	return z, nil
}

type e2eDesign struct{ s *Service }

func (d e2eDesign) StoreView(ctx context.Context, m store.Machine) (map[string]storehub.Grant, error) {
	v, err := d.s.StoreView(ctx, m)
	if err != nil {
		return nil, err
	}
	out := map[string]storehub.Grant{}
	for id, g := range v {
		out[id] = storehub.Grant{See: g.See, Edit: g.Edit, Own: g.Own}
	}
	return out, nil
}
func (d e2eDesign) RecordFromStore(ctx context.Context, m store.Machine, p string, f []byte) (bool, error) {
	return d.s.RecordFromStore(ctx, m, p, f)
}
func (d e2eDesign) ClosedUnrecorded() bool                                     { return false }
func (d e2eDesign) ShelfKeys(context.Context, store.Machine) ([]string, error) { return nil, nil }

func TestGrantedEditorsMachineGetsTheProjectAndListsIt(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	var skew atomic.Int64 // seconds added to the store's clock
	clock := func() time.Time { return time.Now().Add(time.Duration(skew.Load()) * time.Second) }

	dir := t.TempDir()
	meta, err := filestore.OpenMetaStore(ctx, "sqlite://"+dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { meta.Close() })
	blobs, err := filestore.NewFileBlobs(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blobs.Close() })
	next := r.owner
	st, err := storehub.Open(ctx, storehub.Options{Config: &config.Config{}, DB: e2eMachines{r.db},
		Key: []byte("0123456789abcdef0123456789abcdef"), Meta: meta, Blobs: blobs, Design: e2eDesign{r.svc},
		Admit: func(*http.Request) (store.Machine, error) { return next, nil }, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	r.svc.OnAccessChanged(st.AccessChanged)
	mux := http.NewServeMux()
	st.Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := func(m store.Machine) *filesync.Client {
		next = m
		resp, err := http.Post(srv.URL+hubproto.StorePath, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var s hubproto.StoreSession
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			t.Fatal(err)
		}
		return filesync.NewClient(srv.URL+hubproto.StoreAPIPath, filestore.KindDesign,
			func(context.Context, bool) (string, error) { return s.Token, nil }, nil)
	}
	a, b := client(r.owner), client(r.mate)

	// Hop 1: A makes the project (owner file first, then files); the hub records it as A's; A makes it personal.
	const id = "plan-x"
	own := `{"v":2,"user":"coder","name":"Owner","person":"owner@example.com","private":true}`
	if _, err := a.Put(ctx, id+"/.owner.json", []byte(own), "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Put(ctx, id+"/project/Main.dc.html", []byte("<main>v1</main>"), "t"); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.write(ctx, r.owner, func(t *txn) error { return t.setTeam(id, r.ownerU.Email, levelNone) }); err != nil {
		t.Fatal(err)
	}

	// B's machine: a real filesync agent over its own empty library folder.
	bdir := t.TempDir()
	agent, err := filesync.New(filesync.Config{Root: bdir, StateDir: t.TempDir(), Store: b, Machine: "vm-mate",
		Policy: filestore.PolicyFor(filestore.KindDesign), KeepUnseen: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(agent.Close)
	if _, err := agent.Pull(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bdir, id)); err == nil {
		t.Fatal("before the grant B's machine already holds A's personal project")
	}

	// Hop 2/3: A grants B Edit (as the access API does) while B's machine holds a long poll, as its loop does. The grant
	// wakes the poll with a new scope, and B's next pull brings the files; the 15 s views' cache plays no part.
	woke := make(chan time.Duration, 1)
	go func() {
		t0 := time.Now()
		_, _ = agent.Pull(ctx, 10*time.Second)
		woke <- time.Since(t0)
	}()
	time.Sleep(300 * time.Millisecond)
	if err := r.svc.write(ctx, r.owner, func(t *txn) error {
		return t.grant(id, r.ownerU.Email, r.mateU.Email, roleEdit, ttl{none: true}, false)
	}); err != nil {
		t.Fatal(err)
	}
	if d := <-woke; d > 3*time.Second {
		t.Fatalf("the grant did not wake B's held pull: it came back after %v", d)
	}
	if _, err := agent.Pull(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bdir, id, "project", "Main.dc.html")); err != nil {
		t.Fatalf("the project's files did not land on B's machine right after the grant: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bdir, id, ".owner.json")); err != nil {
		t.Fatalf("owner file did not land: %v", err)
	}

	// Hop 4: B's runtime library, with the snapshot the hub sends B's machine.
	snap, err := r.svc.build(ctx, r.mate)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snap)
	var as design.AccessSnapshot
	if err := json.Unmarshal(raw, &as); err != nil {
		t.Fatal(err)
	}
	lib := design.New(design.Config{LibraryDir: bdir, DataDir: t.TempDir(), OwnerEmail: r.mateU.Email, Passive: true,
		Texts: design.Texts{Tag: "test-design", CLI: "acme"}})
	t.Cleanup(lib.StopWatching)
	if err := lib.SetAccess(as); err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, x := range lib.VisibleTo(lib.ListArtifacts()) {
		listed = listed || x.ID == id
	}
	role := lib.RoleNow(id)
	t.Logf("B's library: listed=%v role=%q (owner file person = owner@example.com)", listed, role)
	if !listed || !role.AtLeast(design.RoleEdit) {
		t.Fatalf("B's runtime library does not offer the granted project: listed=%v role=%q", listed, role)
	}
}
