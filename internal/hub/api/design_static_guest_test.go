package api_test

import (
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
	"os"
	"strings"
	"testing"
)

// Run with RUNTIME_DESIGN_CHROME set to exercise the complete owner runtime and Hub with Chrome.
func TestSingleHostnamePrototypeSnapshotWithOwnerOffline(t *testing.T) {
	if os.Getenv("RUNTIME_DESIGN_CHROME") == "" {
		t.Skip("set RUNTIME_DESIGN_CHROME to the installed browser")
	}
	s := hubtest.New(t, hubtest.Env("HUB_DESIGN_SNAPSHOT_LINKS", "on"))
	owner := s.SignIn("owner@example.com")
	outsider := s.SignIn("outsider@example.com")
	space, _ := owner.NewSpace("Studio")
	a, _ := designMachine(t, s, owner, space)
	created(a, "shop", owner.Email)
	s.Eventually("runtime enforces ownership", func() bool { role, ok := snapshotFor(a, owner.Email, "shop"); return ok && role == "owner" })
	base := "/m/" + a.MachineID + "/api/platform/access/design/shop/links"
	input := map[string]any{"forWhom": "Preview", "rights": "view", "ttl": "1d", "version": "pinned", "pages": []string{"Main.dc.html", "Secret.dc.html"}}
	if other := outsider.Do("POST", base, input); other.Code == 201 {
		t.Fatal("unrelated publisher")
	}
	made := owner.Do("POST", base, input)
	if made.Code != 201 {
		t.Fatalf("publish %s", made)
	}
	var link struct {
		URL  string
		Link accessLink
	}
	made.JSON(&link)
	if !strings.HasPrefix(link.URL, s.URL+"/guest/design/snap-") {
		t.Fatalf("unexpected guest origin")
	}
	a.Close()
	guest := s.Browser()
	path := strings.TrimPrefix(link.URL, s.URL)
	for _, page := range []string{"", "?page=1"} {
		v := guest.Do("GET", path+page, nil)
		if v.Code != 200 || !strings.Contains(string(v.Body), "data:image/jpeg;base64,") || strings.Contains(string(v.Body), "<script") {
			t.Fatalf("offline page %s: HTTP %d", page, v.Code)
		}
	}
	if head := guest.Do("HEAD", path, nil); head.Code != 200 || len(head.Body) > 0 {
		t.Fatal("HEAD")
	}
	if other := guest.Do("GET", path+"?page=2", nil); other.Code != 404 {
		t.Fatal("unknown page")
	}
	if r := owner.Do("PATCH", base+"/"+link.Link.ID, map[string]any{"disabled": true}); r.Code != 200 {
		t.Fatalf("revoke: %s", r)
	}
	if v := guest.Do("GET", path, nil); v.Code != 404 {
		t.Fatalf("revoked link: HTTP %d", v.Code)
	}
}
