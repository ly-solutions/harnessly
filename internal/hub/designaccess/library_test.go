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

// the library comes from the hub's store with every machine off, with the access of design access: a
// personal project of someone else is not listed, the person's own shelves are.

type fakeLibrary struct {
	items []LibraryItem
	files map[string][]byte
	// asked is the projects the last listing was asked for.
	asked []string
	// heads is the newest revision of the projects the store has.
	heads map[string]int64
}

func (f *fakeLibrary) Head(_ context.Context, _, project string) (int64, error) {
	return f.heads[project], nil
}
func (f *fakeLibrary) Wait(context.Context, string, string, int64) (int64, error) {
	return 0, nil
}
func (f *fakeLibrary) Serve(http.ResponseWriter, *http.Request, string, string, int64, []string, []string, bool) error {
	return nil
}
func (f *fakeLibrary) Write(context.Context, string, string, string, int64, []byte, string) (Written, error) {
	return Written{}, nil
}
func (f *fakeLibrary) Edit(context.Context, http.ResponseWriter, *http.Request, string, string, string, string) error {
	return nil
}
func (f *fakeLibrary) Library(_ context.Context, _ string, also []string) ([]LibraryItem, error) {
	f.asked = also
	return f.items, nil
}
func (f *fakeLibrary) ReadFile(_ context.Context, _ string, rel string, _ int64) ([]byte, error) {
	return f.files[rel], nil
}

func askLibrary(t *testing.T, r *spaceRig, u store.User, m store.Machine) storeLibrary {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/m/"+m.ID+"/api/platform/design/store-library", nil)
	w := httptest.NewRecorder()
	if !r.svc.ServeMachine(w, req, u, m) {
		t.Fatal("the hub left the library to the machine")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out storeLibrary
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLibraryFromTheStoreKeepsAccess(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	if done, err := r.svc.registerCreated(ctx, r.owner, "secret", r.ownerU.Email); err != nil || !done {
		t.Fatalf("registerCreated: %v %v", done, err)
	}
	if err := r.svc.write(ctx, r.owner, func(t *txn) error { return t.setTeam("secret", r.ownerU.Email, levelNone) }); err != nil {
		t.Fatal(err)
	}
	if done, err := r.svc.registerCreated(ctx, r.owner, "shared", r.ownerU.Email); err != nil || !done {
		t.Fatalf("registerCreated: %v %v", done, err)
	}
	lib := &fakeLibrary{
		items: []LibraryItem{
			{ID: "shared", Title: "Сайт команды", UpdatedAt: 2000, Kind: "site", Boards: []LibraryBoard{{Title: "Главная", Path: "index.dc.html"}}, Rev: 7},
			{ID: "secret", Title: "Личное", UpdatedAt: 1000, Kind: "slides", Rev: 5},
			{ID: "legacy", Title: "", UpdatedAt: 500, Kind: "site", Rev: 3},
		},
		files: map[string][]byte{
			".folders/" + layoutKey(r.ownerU.Email) + ".json": []byte(`{"folders":[{"id":"f-0000aaaa","name":"Клиенты","parent":""}],"placed":{"secret":"f-0000aaaa","gone":"f-0000aaaa"}}`),
		},
	}
	r.svc.UseView(lib)

	// The owner's machine is the one asked about, and no machine is on: the owner sees both recorded projects and their shelf.
	own := askLibrary(t, r, r.ownerU, r.owner)
	ids := map[string]storeArtifact{}
	for _, a := range own.Artifacts {
		ids[a.ID] = a
	}
	if len(ids) != 2 || ids["secret"].Role != roleOwner || ids["secret"].Owner == nil || !ids["secret"].Owner.Me {
		t.Fatalf("owner's library: %+v", own.Artifacts)
	}
	// An unrecorded project is still the machines': its access is theirs, so the hub's copy leaves it out.
	if _, ok := ids["legacy"]; ok {
		t.Fatalf("an unrecorded project listed: %+v", ids["legacy"])
	}
	if len(own.Layout.Folders) != 1 || own.Layout.Placed["secret"] != "f-0000aaaa" || own.Layout.Placed["gone"] != "" {
		t.Fatalf("owner's shelves: %+v", own.Layout)
	}
	if len(own.Missing) == 0 {
		t.Fatal("the answer does not say what only the machine knows")
	}

	// The stranger, under their own machine (off too): the personal project is not there, nor the owner's shelves.
	other := askLibrary(t, r, r.strangeU, r.stranger)
	// a project closed to the person is not read from the store for them at all.
	if len(lib.asked) != 1 || lib.asked[0] != "shared" {
		t.Fatalf("the store was asked for projects closed to the stranger: %v", lib.asked)
	}
	for _, a := range other.Artifacts {
		if a.ID == "secret" {
			t.Fatalf("a stranger sees someone else's personal project: %+v", other.Artifacts)
		}
	}
	if len(other.Artifacts) != 1 || len(other.Layout.Folders) != 0 {
		t.Fatalf("stranger's library: %+v", other)
	}
	if other.Artifacts[0].Owner == nil || other.Artifacts[0].Owner.Me || other.Artifacts[0].Owner.Name == "" {
		t.Fatalf("the owner of a team project: %+v", other.Artifacts[0])
	}
}

func TestLibraryWithoutStoreListsOnlyEffectiveGrants(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, false)
	for _, u := range []store.User{r.ownerU, r.mateU, r.strangeU} {
		if err := r.db.S().CreateIdentity(ctx, &store.Identity{UserID: u.ID, Provider: "test", Subject: u.ID, Email: u.Email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
	}
	team := store.Team{SpaceID: r.owner.SpaceID, Slug: "readers", Name: "Readers"}
	if err := r.db.S().CreateTeam(ctx, &team); err != nil {
		t.Fatal(err)
	}
	if err := r.db.S().AddTeamMember(ctx, team.ID, r.mateU.ID, false, r.svc.now()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"personal", "direct", "team", "invited", "ambiguous"} {
		if _, err := r.svc.registerCreated(ctx, r.owner, id, r.ownerU.Email); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.svc.registerCreated(ctx, r.stranger, "ambiguous", r.strangeU.Email); err != nil {
		t.Fatal(err)
	}
	var invite, token string
	expires := r.svc.now().Add(time.Hour)
	write := func(fn func(*txn) error) {
		t.Helper()
		if err := r.svc.write(ctx, r.owner, fn); err != nil {
			t.Fatal(err)
		}
	}
	write(func(tx *txn) error {
		for _, id := range []string{"direct", "ambiguous"} {
			if _, err := tx.upsertMember(id, r.ownerU.Email, r.mateU.Email, roleEdit, &expires, ""); err != nil {
				return err
			}
		}
		if err := tx.grantTeam("team", r.ownerU.Email, team.ID, roleView, false, ttl{none: true}); err != nil {
			return err
		}
		var err error
		invite, token, _, err = tx.createInvite("invited", r.ownerU.Email, inviteInput{Role: roleComment, TTL: ttl{step: time.Hour}, RoleTTL: ttl{none: true}})
		return err
	})
	cards := func(u store.User, m store.Machine) map[string]storeArtifact {
		t.Helper()
		out := askLibrary(t, r, u, m)
		if !out.MetadataOnly {
			t.Fatal("the library promised a stored copy")
		}
		res := map[string]storeArtifact{}
		for _, c := range out.Artifacts {
			res[c.ID] = c
			if c.WorkMachine != "" || len(c.Boards) != 0 {
				t.Fatalf("machine details: %+v", c)
			}
		}
		return res
	}
	// The stranger's own project of the same slug is its own card (the copy it holds); the owner's is not shown.
	if got := cards(r.strangeU, r.stranger); len(got) != 1 || got["ambiguous"].CardKey != "" || got["ambiguous"].Role != "owner" {
		t.Fatalf("stranger: %v", got)
	}
	unverified := r.mateU
	unverified.ID = r.strangeU.ID
	if got := cards(unverified, r.stranger); len(got) != 0 {
		t.Fatalf("mismatched verified subject saw cards: %v", got)
	}
	got := cards(r.mateU, r.mate)
	if len(got) != 3 || got["ambiguous"].Role != roleEdit || got["ambiguous"].CardKey != "" || got["direct"].Role != roleEdit || got["direct"].AccessSource != "person" || got["direct"].AccessUntil == nil || !got["direct"].AccessUntil.Equal(expires) || got["team"].AccessSource != "team" {
		t.Fatalf("before acceptance: %v", got)
	}
	write(func(tx *txn) error { _, _, _, err := tx.acceptInvite(token, r.mateU.Email); return err })
	got = cards(r.mateU, r.mate)
	if len(got) != 4 || got["invited"].AccessSource != "link" || got["invited"].Role != roleComment || got["invited"].AccessUntil == nil {
		t.Fatalf("accepted: %v", got)
	}
	write(func(tx *txn) error { return tx.revokeInvite("invited", r.ownerU.Email, invite) })
	if err := r.db.S().RemoveTeamMember(ctx, team.ID, r.mateU.ID); err != nil {
		t.Fatal(err)
	}
	r.svc.now = func() time.Time { return expires }
	if got := cards(r.mateU, r.mate); len(got) != 0 {
		t.Fatalf("expired, revoked and removed team grants: %v", got)
	}
	if got := cards(r.ownerU, r.owner); len(got) != 5 || got["personal"].Role != roleOwner {
		t.Fatalf("owner: %v", got)
	}
}

// the hub counts the recorded projects by whether the store has a revision of them, and names the missing.
func TestCoverageCountsTheProjectsTheStoreLacks(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	for _, id := range []string{"instore", "onmachine"} {
		if done, err := r.svc.registerCreated(ctx, r.owner, id, r.ownerU.Email); err != nil || !done {
			t.Fatalf("registerCreated %s: %v %v", id, done, err)
		}
	}
	r.svc.UseView(&fakeLibrary{heads: map[string]int64{"instore": 4}})
	c, missing, err := r.svc.countCoverage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c.Stored != 1 || c.Missing != 1 || len(missing) != 1 || !strings.HasSuffix(missing[0], "/onmachine") {
		t.Fatalf("coverage: %+v %v", c, missing)
	}
	if got := r.svc.Coverage(); !got.At.IsZero() {
		t.Fatalf("a count nobody made is said: %+v", got)
	}
}

// a project recorded on a machine the person may not open (a private one) is not the space's: its team level
// does not let them in, while the owner and a grant of their own still do.
func TestLibraryLeavesOutProjectsOfAClosedMachine(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	if done, err := r.svc.registerCreated(ctx, r.owner, "team-site", r.ownerU.Email); err != nil || !done {
		t.Fatalf("registerCreated: %v %v", done, err)
	}
	r.svc.UseView(&fakeLibrary{items: []LibraryItem{{ID: "team-site", Title: "Сайт", UpdatedAt: 1000, Kind: "site", Rev: 1}}})
	listed := func(u store.User, m store.Machine) string {
		for _, a := range askLibrary(t, r, u, m).Artifacts {
			if a.ID == "team-site" {
				return a.Role
			}
		}
		return ""
	}
	// An open machine: the team level counts for the space.
	if role := listed(r.mateU, r.mate); role == "" {
		t.Fatal("a team project of an open machine is not listed for a mate")
	}
	// The owner's machine turns private: only its owner opens it.
	asked := 0
	r.svc.opens = func(_ context.Context, userID string, m store.Machine) bool {
		asked++
		return m.ID != r.owner.ID || userID == r.ownerU.ID
	}
	if role := listed(r.mateU, r.mate); role != "" {
		t.Fatalf("a private machine's project is listed for a mate with the role %q", role)
	}
	if asked != 1 {
		t.Fatalf("the machine's access was asked %d times for one person's list, want 1", asked)
	}
	if role := listed(r.ownerU, r.owner); role != roleOwner {
		t.Fatalf("the owner's own project: role %q", role)
	}
	// A grant of the mate's own opens it again: privacy closes the space's door, not the ones given by name.
	if err := r.svc.write(ctx, r.owner, func(t *txn) error {
		return t.grant("team-site", r.ownerU.Email, r.mateU.Email, roleView, ttl{none: true}, false)
	}); err != nil {
		t.Fatal(err)
	}
	if role := listed(r.mateU, r.mate); role != roleView {
		t.Fatalf("a project granted by name: role %q", role)
	}
}

// a team level its owner chose in the access dialog shares the project with the team even when its machine is
// private: the hub serves the project, not the machine. A level it got by default still stays behind the machine.
func TestChosenTeamLevelPassesAClosedMachine(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	if done, err := r.svc.registerCreated(ctx, r.owner, "team-site", r.ownerU.Email); err != nil || !done {
		t.Fatalf("registerCreated: %v %v", done, err)
	}
	r.svc.UseView(&fakeLibrary{items: []LibraryItem{{ID: "team-site", Title: "Сайт", UpdatedAt: 1000, Kind: "site", Rev: 1}}})
	r.svc.opens = func(_ context.Context, userID string, m store.Machine) bool {
		return m.ID != r.owner.ID || userID == r.ownerU.ID
	}
	listed := func() string {
		for _, a := range askLibrary(t, r, r.mateU, r.mate).Artifacts {
			if a.ID == "team-site" {
				return a.Role
			}
		}
		return ""
	}
	if role := listed(); role != "" {
		t.Fatalf("a default team level of a private machine let the mate in: %q", role)
	}
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error { return tx.setTeam("team-site", r.ownerU.Email, roleComment) }); err != nil {
		t.Fatal(err)
	}
	if role := listed(); role != roleComment {
		t.Fatalf("the team level the owner chose: role %q, want %q", role, roleComment)
	}
}

// Sinopia's home in one answer goes on to the machine only for whom hub-design-home is on; for the rest the
// hub answers 404 home_off and the page asks part by part, as before.
func TestHomeInOneAnswerIsBehindItsSwitch(t *testing.T) {
	r := newSpaceRig(t, true)
	ask := func() (bool, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodGet, "/m/"+r.owner.ID+"/api/platform/design/home?machine="+r.owner.ID, nil)
		w := httptest.NewRecorder()
		return r.svc.ServeMachine(w, req, r.ownerU, r.owner), w
	}
	if served, w := ask(); !served || w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "home_off") {
		t.Fatalf("switch off: served %v, %d %s", served, w.Code, w.Body.String())
	}
	var asked string
	r.svc.homeOn = func(_ context.Context, email, machineID string) bool {
		asked = email + " " + machineID
		return true
	}
	if served, _ := ask(); served {
		t.Fatal("switch on: the hub kept the home from the machine")
	}
	if asked != normEmail(r.ownerU.Email)+" "+r.owner.ID {
		t.Fatalf("the switch was asked for %q", asked)
	}
}
