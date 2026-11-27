package designaccess

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// the render worker opens a board in the store under a live key of the project's owner, the address the
// proxy signs a keyed request in by.
func TestRenderLocationIsAKeyedAddressOfTheOwner(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := &Service{files: newFileKeyer([]byte("master")), now: func() time.Time { return now }}
	loc := s.RenderLocation("m1", "u1", "/api/x", "p1abc", "pages/my home.html", 0)
	if !strings.HasPrefix(loc, "/m/m1/api/x/design/store/p1abc/k/") || !strings.HasSuffix(loc, "/f/pages/my%20home.html") {
		t.Fatalf("location: %s", loc)
	}
	r := httptest.NewRequest("GET", loc, nil)
	if who := s.KeyedFileUser(r); who != "u1" {
		t.Fatalf("the keyed address signs in %q, want the owner", who)
	}
	for _, bad := range []struct{ prefix, project, board string }{
		{"/x", "p1abc", "a.html"}, {"/api/x", "../p", "a.html"}, {"/api/x", "p1abc", "../a.html"}, {"/api/x", "p1abc", "a//b.html"},
	} {
		if loc := s.RenderLocation("m1", "u1", bad.prefix, bad.project, bad.board, 0); loc != "" {
			t.Fatalf("%+v gave %s", bad, loc)
		}
	}
	// A snapshot's page: the board at a pinned revision, under the same key.
	at := s.RenderLocation("m1", "u1", "/api/x", "p1abc", "pages/my home.html", 5)
	if !strings.Contains(at, "/v/5/f/pages/my%20home.html") {
		t.Fatalf("pinned location: %s", at)
	}
	if who := s.KeyedFileUser(httptest.NewRequest("GET", at, nil)); who != "u1" {
		t.Fatalf("the pinned keyed address signs in %q, want the owner", who)
	}
	if loc := (&Service{now: time.Now}).RenderLocation("m1", "u1", "/api/x", "p1abc", "a.html", 0); loc != "" {
		t.Fatalf("a hub without a key gave %s", loc)
	}
}
