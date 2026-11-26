package api_test

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ly-solutions/harnessly/internal/design"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// GitHub issue 30: a machine whose filesystem fails the share probe (a sandbox that does not keep lookups beneath a
// folder) makes no guest link and serves no guest, and both say why and what to run instead; once the probe holds the
// same machine shares as usual.
func TestDesignShareFailsClosedOnAnUnsafeRuntime(t *testing.T) {
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost))
	owner := s.SignIn("owner@example.com")
	space, _ := owner.NewSpace("Studio")

	var unsafe atomic.Bool
	unsafe.Store(true)
	a, _ := designMachineWithProbe(t, s, owner, space, func(dir string) design.ShareFSReport {
		if unsafe.Load() {
			return design.ShareFSReport{Steps: []design.ShareFSStep{{ID: "dotdot", Code: design.ShareFSDotDot}}, Hint: design.ShareFSHint}
		}
		return design.ProbeShareFS(dir)
	})
	base := "/m/" + a.MachineID + "/api/platform/design/access"
	created(a, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool {
		role, ok := snapshotFor(a, "owner@example.com", "shop")
		return ok && role == "owner"
	})

	// HRN-1755, GitHub #187: with a store a guest never reads the runtime, so its probe does not stop a link. A pinned
	// link needs the project in the store; a latest one is made at once and its guest is told «not published» until
	// the project arrives there.
	if r := owner.Do("POST", base+"/shop/shares", map[string]any{"rights": "view", "version": "pinned"}); r.Code != 409 || !strings.Contains(r.String(), "not_published") {
		t.Fatalf("a pinned link to a project not in the store: %s", r)
	}
	r := owner.Do("POST", base+"/shop/shares", map[string]any{"rights": "view", "version": "latest"})
	if r.Code != 201 {
		t.Fatalf("a latest link on an unsafe runtime with the store on: %s", r)
	}
	var list []map[string]any
	owner.Do("GET", base+"/shop/shares", nil).JSON(&list)
	if len(list) != 1 {
		t.Fatalf("links kept: %+v", list)
	}
	var link struct{ URL string }
	r.JSON(&link)
	path := strings.TrimPrefix(link.URL, "http://"+shareHost)
	guest := s.Browser()
	open := func(p string) hubtest.Resp { t.Helper(); return guest.DoAt(s.URL, shareHost, "GET", p, nil) }
	if r := open(path + "/f/Main.dc.html"); r.Code != 404 || !strings.Contains(string(r.Body), "не опубликован") {
		t.Fatalf("a guest before the project reached the store: %s", r)
	}
	putStore(t, s, space, "shop", shopFiles)
	if r := open(path + "/f/Main.dc.html"); r.Code != 200 || !strings.Contains(string(r.Body), "shop main") {
		t.Fatalf("guest page from the store, the runtime unsafe: %s", r)
	}
	if r := owner.Do("POST", base+"/shop/shares", map[string]any{"rights": "view", "version": "pinned"}); r.Code != 201 {
		t.Fatalf("a pinned link once the project is in the store, the runtime unsafe: %s", r)
	}

	unsafe.Store(false)
	if r := open(path + "/f/Main.dc.html"); r.Code != 200 {
		t.Fatalf("guest page once the probe holds: %s", r)
	}

	// The runtime turns unsafe (moved into a sandbox): a guest reads the store, which does not depend on the machine.
	unsafe.Store(true)
	for _, p := range []string{"/index.json", "/f/Main.dc.html"} {
		if r := open(path + p); r.Code != 200 {
			t.Fatalf("guest %s with the owner's runtime unsafe: %s", p, r)
		}
	}
}

// GitHub #187: without the store the machine serves the guest itself, so a runtime that fails the share probe still
// gets no link, and the owner is told what to run instead.
func TestDesignShareWithoutAStoreStaysOffOnAnUnsafeRuntime(t *testing.T) {
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost), hubtest.Env("HUB_STORE", "off"))
	owner := s.SignIn("owner@example.com")
	space, _ := owner.NewSpace("Studio")
	a, _ := designMachineWithProbe(t, s, owner, space, func(string) design.ShareFSReport {
		return design.ShareFSReport{Steps: []design.ShareFSStep{{ID: "dotdot", Code: design.ShareFSDotDot}}, Hint: design.ShareFSHint}
	})
	base := "/m/" + a.MachineID + "/api/platform/design/access"
	created(a, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool {
		role, ok := snapshotFor(a, "owner@example.com", "shop")
		return ok && role == "owner"
	})
	if r := owner.Do("POST", base+"/shop/shares", map[string]any{"rights": "view", "version": "latest"}); r.Code != 409 || !strings.Contains(r.String(), "share_unsafe_runtime") || !strings.Contains(r.String(), "harnessly doctor share") {
		t.Fatalf("a latest link on an unsafe runtime without a store: %s", r)
	}
}
