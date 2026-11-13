package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// The guest links of a design live in the hub's one registry of guest links: a link is entered when it is made, a
// link the registry lost (an older revision of the hub made it, or the migration was taken back) opens again once the
// hub starts, and a revoked one is gone there too.
func TestDesignGuestLinksLiveInTheOneRegistry(t *testing.T) {
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost))
	owner := s.SignIn("owner@example.com")
	space, _ := owner.NewSpace("Studio")
	a, _ := designMachine(t, s, owner, space)
	created(a, "shop", "owner@example.com")
	s.Eventually("the project recorded", func() bool { r, ok := snapshotFor(a, "owner@example.com", "shop"); return ok && r == "owner" })
	ctx := context.Background()
	base := "/m/" + a.MachineID + "/api/platform/design/access"
	r := owner.Do("POST", base+"/shop/shares", map[string]any{"rights": "view"})
	if r.Code != 201 {
		t.Fatalf("create link: %s", r)
	}
	var l struct{ ID, URL string }
	r.JSON(&l)
	path := strings.TrimPrefix(l.URL, "http://"+shareHost)
	guest := s.Browser()
	open := func() int { t.Helper(); return guest.DoAt(s.URL, shareHost, "GET", path+"/index.json", nil).Code }
	entry := func() (kind, id string, revoked bool, n int) {
		t.Helper()
		rows, err := s.DB.S().Q().Query(ctx, `SELECT kind, id, revoked_at IS NOT NULL FROM resource_guest_links WHERE link_id = ?`, l.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			if err := rows.Scan(&kind, &id, &revoked); err != nil {
				t.Fatal(err)
			}
			n++
		}
		return
	}
	if kind, id, revoked, n := entry(); n != 1 || kind != "design" || id != a.MachineID+"/shop" || revoked {
		t.Fatalf("the link in the registry: %d %q %q %v", n, kind, id, revoked)
	}
	alive := open()
	if alive == http.StatusNotFound || alive == http.StatusGone {
		t.Fatalf("a live link: %d", alive)
	}

	// The entries taken out, as going back to an older revision would have it: the registry decides, so the link does
	// not open, and the link's own row is untouched.
	if _, err := s.DB.S().Q().Exec(ctx, `DELETE FROM resource_guest_links WHERE kind = 'design'`); err != nil {
		t.Fatal(err)
	}
	if c := open(); c != http.StatusNotFound {
		t.Fatalf("a link the registry does not know: %d", c)
	}
	s.Restart()
	owner = s.SignIn("owner@example.com")
	a.Connect()
	s.Eventually("the address a person holds opens again after the hub started", func() bool { return open() == alive })
	if _, _, _, n := entry(); n != 1 {
		t.Fatalf("the link entered again: %d", n)
	}

	if r := owner.Do("DELETE", base+"/shop/shares/"+l.ID, nil); r.Code >= 300 {
		t.Fatalf("revoke: %s", r)
	}
	if c := open(); c != http.StatusGone {
		t.Fatalf("a revoked link: %d", c)
	}
	if _, _, revoked, _ := entry(); !revoked {
		t.Fatal("the registry still holds the revoked link as alive")
	}
}
