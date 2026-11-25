package designaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

func askKeyed(t *testing.T, r *spaceRig, u store.User, m store.Machine) storeLibrary {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/m/"+m.ID+"/api/platform/design/store-library?caps=designkey", nil)
	w := httptest.NewRecorder()
	if !r.svc.ServeMachine(w, req, u, m) || w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out storeLibrary
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// GitHub #179: a slug held by several machines is several cards, each named by its holder.
func TestSameSlugCardsAreKeyedByHolder(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, false)
	for _, u := range []store.User{r.ownerU, r.mateU, r.strangeU} {
		if err := r.db.S().CreateIdentity(ctx, &store.Identity{UserID: u.ID, Provider: "test", Subject: u.ID, Email: u.Email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, h := range []struct {
		m store.Machine
		u store.User
	}{{r.owner, r.ownerU}, {r.stranger, r.strangeU}, {r.mate, r.mateU}} {
		if _, err := r.svc.registerCreated(ctx, h.m, "site", h.u.Email); err != nil {
			t.Fatal(err)
		}
	}
	expires := r.svc.now().Add(time.Hour)
	grant := func(m store.Machine, owner store.User) {
		if err := r.svc.write(ctx, m, func(tx *txn) error {
			_, err := tx.upsertMember("site", owner.Email, r.mateU.Email, roleView, &expires, "")
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	grant(r.owner, r.ownerU)
	grant(r.stranger, r.strangeU)

	lib := askKeyed(t, r, r.mateU, r.mate)
	keys := map[string]bool{}
	own := 0
	for _, c := range lib.Artifacts {
		if c.ID != "site" {
			continue
		}
		if c.CardKey == "" {
			own++
			continue
		}
		if !cardShape(c.CardKey) || strings.Contains(c.CardKey, r.owner.ID) || strings.Contains(c.CardKey, r.stranger.ID) {
			t.Fatalf("card key leaks or is malformed: %q", c.CardKey)
		}
		keys[c.CardKey] = true
	}
	if own != 1 || len(keys) != 2 || !keys[cardKey(r.owner.ID, "site")] || !keys[cardKey(r.stranger.ID, "site")] {
		t.Fatalf("own=%d keys=%v", own, keys)
	}
	// A page that does not read keys gets one card per slug, its own first.
	plain := askLibrary(t, r, r.mateU, r.mate)
	n := 0
	for _, c := range plain.Artifacts {
		if c.ID == "site" {
			n++
			if c.CardKey != "" {
				t.Fatalf("key to a page without the cap: %+v", c)
			}
		}
	}
	if n != 1 {
		t.Fatalf("plain cards: %d", n)
	}
	// The key resolves back to its holder, and only inside the space of the asking machine.
	anchor := r.mate
	if id, err := r.svc.holderByCard(ctx, anchor, "site", cardKey(r.owner.ID, "site")); err != nil || id != r.owner.ID {
		t.Fatalf("holderByCard: %q %v", id, err)
	}
	if _, err := r.svc.holderByCard(ctx, anchor, "site", cardKey(r.owner.ID, "other")); err == nil {
		t.Fatal("a key of another project resolved")
	}
	// A revoked grant of one copy removes that card only; an expired one too.
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error { return tx.revoke("site", r.ownerU.Email, r.mateU.Email, false) }); err != nil {
		t.Fatal(err)
	}
	after := askKeyed(t, r, r.mateU, r.mate)
	left := map[string]bool{}
	for _, c := range after.Artifacts {
		if c.ID == "site" && c.CardKey != "" {
			left[c.CardKey] = true
		}
	}
	if len(left) != 1 || !left[cardKey(r.stranger.ID, "site")] {
		t.Fatalf("after revoke: %v", left)
	}
	r.svc.now = func() time.Time { return expires.Add(time.Minute) }
	for _, c := range askKeyed(t, r, r.mateU, r.mate).Artifacts {
		if c.ID == "site" && c.CardKey != "" {
			t.Fatalf("an expired grant is shown: %+v", c)
		}
	}
	// An ungranted account sees none of them.
	for _, c := range askKeyed(t, r, r.strangeU, r.stranger).Artifacts {
		if c.CardKey != "" {
			t.Fatalf("ungranted: %+v", c)
		}
	}
}

// A key minted for one holder's copy opens no other copy and no other person's files.
func TestFileKeyCarriesTheCard(t *testing.T) {
	k := newFileKeyer([]byte("secret"))
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	card := cardKey("m1", "site")
	key := k.mintCard("u1", "site", card, now)
	if !storeKeyShape(key) {
		t.Fatalf("shape: %q", key)
	}
	if got, ok := k.checkCard(key, "u1", "site", now); !ok || got != card {
		t.Fatalf("check: %q %v", got, ok)
	}
	if _, ok := k.checkCard(key, "u2", "site", now); ok {
		t.Fatal("another person")
	}
	if _, ok := k.checkCard(key, "u1", "other", now); ok {
		t.Fatal("another project")
	}
	forged := key[:strings.LastIndex(key, ".")+1] + cardKey("m2", "site")
	if _, ok := k.checkCard(forged, "u1", "site", now); ok {
		t.Fatal("a swapped card")
	}
	if got, ok := k.checkCard(k.mint("u1", "site", now), "u1", "site", now); !ok || got != "" {
		t.Fatalf("plain key: %q %v", got, ok)
	}
}

// GitHub #179: the library's poll works with no file store, from the grants; its revision grows when a grant changes
// and a refresh that cannot read the grants is an error, never the revision the page already holds.
func TestChangesWithoutAStoreFollowTheGrants(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, false)
	for _, u := range []store.User{r.ownerU, r.mateU} {
		if err := r.db.S().CreateIdentity(ctx, &store.Identity{UserID: u.ID, Provider: "test", Subject: u.ID, Email: u.Email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.svc.registerCreated(ctx, r.owner, "site", r.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	rev := func() int64 {
		t.Helper()
		got, err := r.svc.grantedRev(ctx, r.mateU, r.mate)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := rev()
	if again := rev(); again != first {
		t.Fatalf("a library that did not change moved the revision: %d -> %d", first, again)
	}
	r.svc.now = func() time.Time { return time.Date(2026, 11, 9, 9, 0, 1, 0, time.UTC) }
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error {
		_, err := tx.upsertMember("site", r.ownerU.Email, r.mateU.Email, roleView, nil, "")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	granted := rev()
	if granted <= first {
		t.Fatalf("a grant did not move the revision: %d -> %d", first, granted)
	}
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error { return tx.revoke("site", r.ownerU.Email, r.mateU.Email, false) }); err != nil {
		t.Fatal(err)
	}
	if revoked := rev(); revoked <= granted {
		t.Fatalf("a revocation did not move the revision: %d -> %d", granted, revoked)
	}
	r.svc.homeOn = func(context.Context, string, string) bool { return true }
	// The poll itself answers at once when the page is behind, and says nothing it cannot read.
	req := httptest.NewRequest(http.MethodGet, "/m/"+r.mate.ID+"/api/platform/design/store-library/changes?since=0", nil)
	w := httptest.NewRecorder()
	if !r.svc.ServeMachine(w, req, r.mateU, r.mate) || w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"rev"`) {
		t.Fatalf("changes: %d %s", w.Code, w.Body.String())
	}
	strange := r.strangeU
	w = httptest.NewRecorder()
	r.svc.ServeMachine(w, httptest.NewRequest(http.MethodGet, "/m/"+r.stranger.ID+"/api/platform/design/store-library/changes?since=0", nil), strange, r.stranger)
	if w.Code != http.StatusOK {
		t.Fatalf("an account with no grants: %d %s", w.Code, w.Body.String())
	}
}
