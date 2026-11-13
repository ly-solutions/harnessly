package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// with HUB_DESIGN_DERIVED the hub keeps what it worked out from a revision in its store and database, and a
// guest link reads the same with the machine off: the same answers again, a page outside the link still hidden, a new
// revision followed, and the rows of old revisions dropped.
func TestDesignGuestLinkReadsTheKeptAnswers(t *testing.T) {
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost), hubtest.Env("HUB_DESIGN_DERIVED", "on"))
	if s.Hub.Store == nil {
		t.Skip("the hub runs without a store")
	}
	owner := s.SignIn("owner@example.com")
	space, _ := owner.NewSpace("Studio")
	a, _ := designMachine(t, s, owner, space)
	created(a, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool { r, ok := snapshotFor(a, "owner@example.com", "shop"); return ok && r == "owner" })
	ctx := context.Background()
	room, err := s.Hub.Store.DesignRoom(ctx, space)
	if err != nil {
		t.Fatal(err)
	}
	put := func(path, body string) {
		t.Helper()
		sum := sha256.Sum256([]byte(body))
		sha := hex.EncodeToString(sum[:])
		if err := room.PutBlob(ctx, sha, bytes.NewReader([]byte(body))); err != nil {
			t.Fatal(err)
		}
		var base int64
		if cur, err := room.Current(ctx, path); err == nil && cur != nil {
			base = cur.Rev
		}
		if _, conflict, err := room.Commit(ctx, filestore.CommitRequest{Path: path, Base: base, Sha: sha, Size: int64(len(body)), Author: "owner"}, "vm-owner"); err != nil || conflict {
			t.Fatalf("commit %s: %v %v", path, err, conflict)
		}
	}
	put("shop/artifact.json", `{"title":"Shop"}`)
	put("shop/project/Secret.dc.html", `<html><body>store secret</body></html>`)
	put("shop/project/Main.dc.html", `<html><body>store v1</body></html>`)
	r := owner.Do("POST", "/m/"+a.MachineID+"/api/platform/design/access/shop/shares", map[string]any{"rights": "view", "pages": []string{"Main.dc.html"}})
	if r.Code != 201 {
		t.Fatalf("create link: %s", r)
	}
	var l struct{ URL string }
	r.JSON(&l)
	live := strings.TrimPrefix(l.URL, "http://"+shareHost)
	a.Close()
	s.Eventually("the machine offline", func() bool { return s.Hub.Registry.Get(a.MachineID) == nil })

	guest := s.Browser()
	open := func(p string) hubtest.Resp { t.Helper(); return guest.DoAt(s.URL, shareHost, "GET", p, nil) }
	rows := func() int {
		t.Helper()
		var n int
		if err := s.DB.S().Q().QueryRow(ctx, `SELECT COUNT(*) FROM design_derived`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for i, name := range []string{"worked out", "kept"} {
		if r := open(live + "/index.json"); r.Code != 200 || !strings.Contains(string(r.Body), "Main.dc.html") || strings.Contains(string(r.Body), "Secret.dc.html") {
			t.Fatalf("%s index: %s", name, r)
		}
		if r := open(live + "/f/Main.dc.html"); r.Code != 200 || !strings.Contains(string(r.Body), "store v1") {
			t.Fatalf("%s page: %s", name, r)
		}
		if r := open(live + "/f/Secret.dc.html"); r.Code == 200 {
			t.Fatalf("%s: a page outside the link: %s", name, r)
		}
		if n := rows(); n != 2 {
			t.Fatalf("after pass %d: %d kept answers, want 2", i, n)
		}
	}
	// New revisions are followed, and the answers of the revisions long passed do not pile up.
	for i := 0; i < 12; i++ {
		body := "store v" + string(rune('a'+i))
		put("shop/project/Main.dc.html", `<html><body>`+body+`</body></html>`)
		if r := open(live + "/f/Main.dc.html"); r.Code != 200 || !strings.Contains(string(r.Body), body) {
			t.Fatalf("revision %d: %s", i, r)
		}
	}
	var revs int
	if err := s.DB.S().Q().QueryRow(ctx, `SELECT COUNT(DISTINCT rev) FROM design_derived`).Scan(&revs); err != nil || revs > 8 {
		t.Fatalf("answers of old revisions pile up: %d revisions kept (%v)", revs, err)
	}
}
