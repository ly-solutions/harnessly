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

// foreignRig is the space rig and a second space (tenant) with a person of its own, who is no member of the first.
type foreignRig struct {
	*spaceRig
	outU store.User
	out  store.Machine
	lib  *fakeLibrary
}

func newForeignRig(t *testing.T) *foreignRig {
	t.Helper()
	ctx := context.Background()
	r := newSpaceRig(t, true)
	st := r.db.S()
	f := &foreignRig{spaceRig: r, outU: store.User{Email: "client@other.example", Name: "Client"}}
	if err := st.CreateUser(ctx, &f.outU); err != nil {
		t.Fatal(err)
	}
	sp := &store.Space{Slug: "other", Name: "Other", CreatedBy: f.outU.ID}
	if err := st.CreateSpace(ctx, sp); err != nil {
		t.Fatal(err)
	}
	f.out = store.Machine{SpaceID: sp.ID, Name: "vm-client", Profile: "p", Via: "v", KeyPub: "k9", EncPub: "k9e", CreatedBy: f.outU.ID}
	if err := st.CreateMachine(ctx, &f.out); err != nil {
		t.Fatal(err)
	}
	for _, u := range []store.User{r.ownerU, f.outU} {
		if err := st.CreateIdentity(ctx, &store.Identity{UserID: u.ID, Provider: "test", Subject: u.ID, Email: u.Email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
	}
	f.lib = &fakeLibrary{items: []LibraryItem{{ID: "plan", Title: "План", UpdatedAt: 2000, Kind: "site", Rev: 3}, {ID: "hidden", Title: "Секрет", UpdatedAt: 1000, Kind: "site", Rev: 2}},
		heads: map[string]int64{"plan": 3, "hidden": 2}}
	r.svc.UseView(f.lib)
	for _, id := range []string{"plan", "hidden"} {
		if _, err := r.svc.registerCreated(ctx, r.owner, id, r.ownerU.Email); err != nil {
			t.Fatal(err)
		}
		if err := r.svc.write(ctx, r.owner, func(tx *txn) error { return tx.setTeam(id, r.ownerU.Email, levelNone) }); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *foreignRig) serve(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/m/"+f.out.ID+"/api/platform"+path, strings.NewReader("{}"))
	w := httptest.NewRecorder()
	if !f.svc.ServeMachine(w, req, f.outU, f.out) {
		t.Fatalf("the hub left %s to the machine", path)
	}
	return w
}

func (f *foreignRig) cards(t *testing.T) map[string]storeArtifact {
	t.Helper()
	w := f.serve(t, http.MethodGet, "/design/store-library?caps=designkey")
	if w.Code != http.StatusOK {
		t.Fatalf("library: %d %s", w.Code, w.Body.String())
	}
	var lib storeLibrary
	if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil {
		t.Fatal(err)
	}
	out := map[string]storeArtifact{}
	for _, a := range lib.Artifacts {
		out[a.ID] = a
	}
	return out
}

func (f *foreignRig) grant(t *testing.T, role string) {
	t.Helper()
	if err := f.svc.write(context.Background(), f.owner, func(tx *txn) error {
		_, err := tx.upsertMember("plan", f.ownerU.Email, f.outU.Email, role, nil, "")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// a project shared by email with a person of another space is a card of their library, read from the hub's
// store read-only; nothing else of the owner's space is open to them, and a revocation closes it.
func TestForeignGrantReadsFromTheStoreOnly(t *testing.T) {
	f := newForeignRig(t)
	if got := f.cards(t); len(got) != 0 {
		t.Fatalf("before the grant: %v", got)
	}
	f.grant(t, roleEdit)
	got := f.cards(t)
	card := got["plan"]
	if len(got) != 1 || card.CardKey != cardKey(f.owner.ID, "plan") || card.Role != roleView || card.AccessSource != "person" || card.Title != "План" {
		t.Fatalf("after the grant: %+v", got)
	}
	// A page that does not read card keys could not open it: not listed.
	w := f.serve(t, http.MethodGet, "/design/store-library")
	if strings.Contains(w.Body.String(), `"plan"`) {
		t.Fatalf("listed without card keys: %s", w.Body.String())
	}

	// Its head is read by its card, and by nothing else.
	if w := f.serve(t, http.MethodGet, "/design/store/plan/head?card="+card.CardKey); w.Code != http.StatusOK {
		t.Fatalf("head by card: %d %s", w.Code, w.Body.String())
	}
	if w := f.serve(t, http.MethodGet, "/design/store/plan/head"); w.Code != http.StatusNotFound {
		t.Fatalf("head without the card: %d %s", w.Code, w.Body.String())
	}
	if w := f.serve(t, http.MethodGet, "/design/store/hidden/head?card="+cardKey(f.owner.ID, "hidden")); w.Code != http.StatusNotFound {
		t.Fatalf("a project not shared with them: %d %s", w.Code, w.Body.String())
	}
	// Read-only, whatever the owner gave.
	if w := f.serve(t, http.MethodPut, "/design/store/plan/file/Main.dc.html?card="+card.CardKey); w.Code != http.StatusForbidden {
		t.Fatalf("a write: %d %s", w.Code, w.Body.String())
	}
	f.grant(t, roleManage)
	w = f.serve(t, http.MethodGet, "/design/access/plan?card="+card.CardKey)
	var view map[string]any
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &view) != nil {
		t.Fatalf("access view: %d %s", w.Code, w.Body.String())
	}
	if view["role"] != roleView || view["canManage"] != false || view["people"] != nil || view["invites"] != nil {
		t.Fatalf("access view of another space's project: %v", view)
	}

	if err := f.svc.write(context.Background(), f.owner, func(tx *txn) error { return tx.revoke("plan", f.ownerU.Email, f.outU.Email, false) }); err != nil {
		t.Fatal(err)
	}
	if got := f.cards(t); len(got) != 0 {
		t.Fatalf("after the revoke: %v", got)
	}
	if w := f.serve(t, http.MethodGet, "/design/store/plan/head?card="+card.CardKey); w.Code != http.StatusNotFound {
		t.Fatalf("head after the revoke: %d %s", w.Code, w.Body.String())
	}
}

// A team level of the owner's space, its whole space and an unverified account cross no space.
func TestForeignOnlyByAPersonalGrant(t *testing.T) {
	f := newForeignRig(t)
	if err := f.svc.write(context.Background(), f.owner, func(tx *txn) error { return tx.setTeam("plan", f.ownerU.Email, roleEdit) }); err != nil {
		t.Fatal(err)
	}
	if got := f.cards(t); len(got) != 0 {
		t.Fatalf("the owner's team level reached another space: %v", got)
	}
	f.grant(t, roleView)
	if _, err := f.db.S().Q().Exec(context.Background(), `UPDATE identities SET email_verified=? WHERE user_id=?`, false, f.outU.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.cards(t); len(got) != 0 {
		t.Fatalf("an unverified account: %v", got)
	}
}

// An invitation of another space is accepted from a machine of the person's own: the answer names the card to open.
func TestForeignInviteAccept(t *testing.T) {
	f := newForeignRig(t)
	var token string
	if err := f.svc.write(context.Background(), f.owner, func(tx *txn) error {
		var err error
		_, token, _, err = tx.createInvite("plan", f.ownerU.Email, inviteInput{Role: roleEdit, TTL: ttl{step: time.Hour}, RoleTTL: ttl{none: true}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w := f.serve(t, http.MethodPost, "/design/access/invites/"+token+"/accept")
	var got struct{ Project, Role, CardKey string }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatalf("accept: %d %s", w.Code, w.Body.String())
	}
	if got.Project != "plan" || got.Role != roleView || got.CardKey != cardKey(f.owner.ID, "plan") {
		t.Fatalf("accept answer: %+v", got)
	}
	if card := f.cards(t)["plan"]; card.CardKey != got.CardKey || card.AccessSource != "link" {
		t.Fatalf("the library after accepting: %+v", card)
	}
}
