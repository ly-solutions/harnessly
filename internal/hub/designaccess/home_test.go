package designaccess

import (
	"context"
	"encoding/json"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// with hub-design-home on, the hub answers Sinopia's home with the machine's answer when it comes within the
// budget and with its store's copy of the library when the machine is slow, off or answers no home.
func TestHomeFromTheMachineOrTheStore(t *testing.T) {
	old := homeBudget
	homeBudget = 50 * time.Millisecond
	t.Cleanup(func() { homeBudget = old })

	ctx := context.Background()
	r := newSpaceRig(t, true)
	if done, err := r.svc.registerCreated(ctx, r.owner, "shared", r.ownerU.Email); err != nil || !done {
		t.Fatalf("registerCreated: %v %v", done, err)
	}
	r.svc.UseView(&fakeLibrary{items: []LibraryItem{{ID: "shared", Title: "Сайт команды", UpdatedAt: 2000, Kind: "site", Rev: 7}}})

	serveOn := func(r *spaceRig, ask func(ctx context.Context, w http.ResponseWriter)) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/m/"+r.owner.ID+"/api/platform/design/home?machine="+r.owner.ID, nil)
		w := httptest.NewRecorder()
		if !r.svc.ServeHome(w, req, r.ownerU, r.owner, ask) {
			t.Fatal("the hub did not take the home")
		}
		return w
	}
	serve := func(ask func(ctx context.Context, w http.ResponseWriter)) *httptest.ResponseRecorder {
		t.Helper()
		return serveOn(r, ask)
	}
	machine := func(delay time.Duration, code int, body string) func(ctx context.Context, w http.ResponseWriter) {
		return func(ctx context.Context, w http.ResponseWriter) {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
	}

	// The switch is off: 404 home_off, the machine is not asked.
	asked := false
	if w := serve(func(context.Context, http.ResponseWriter) { asked = true }); w.Code != http.StatusNotFound || asked {
		t.Fatalf("switch off: %d, machine asked %v", w.Code, asked)
	}
	r.svc.homeOn = func(context.Context, string, string) bool { return true }

	// A fast machine: its own answer.
	w := serve(machine(0, http.StatusOK, `{"parts":{"library":{"designs":[]}},"failed":[],"tookMs":3}`))
	if w.Code != http.StatusOK || w.Header().Get(HomeFromHeader) != "machine" || !strings.Contains(w.Body.String(), `"library"`) {
		t.Fatalf("fast machine: %d %s %s", w.Code, w.Header().Get(HomeFromHeader), w.Body.String())
	}

	storeCopy := func(w *httptest.ResponseRecorder, what string) {
		t.Helper()
		if w.Code != http.StatusOK || w.Header().Get(HomeFromHeader) != "store" {
			t.Fatalf("%s: %d %s %s", what, w.Code, w.Header().Get(HomeFromHeader), w.Body.String())
		}
		var out struct {
			From  string                  `json:"from"`
			Parts map[string]storeLibrary `json:"parts"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.From != "store" || len(out.Parts["storeLibrary"].Artifacts) != 1 {
			t.Fatalf("%s: %s", what, w.Body.String())
		}
	}
	// A slow machine: the store's copy once the budget is spent, without waiting for the machine.
	start := time.Now()
	storeCopy(serve(machine(5*time.Second, http.StatusOK, `{}`)), "slow machine")
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the hub waited for the slow machine: %v", took)
	}
	// A machine that answers no home (an old runtime's 404, an error): the store's copy at once.
	storeCopy(serve(machine(0, http.StatusNotFound, `not found`)), "no home on the machine")

	// No store: the machine's answer, however late. A hub of its own: the view is set once before the hub serves, and
	// switching it here raced with the store reads the asks above left running.
	bare := newSpaceRig(t, true)
	bare.svc.homeOn = r.svc.homeOn
	serve = func(ask func(ctx context.Context, w http.ResponseWriter)) *httptest.ResponseRecorder {
		t.Helper()
		return serveOn(bare, ask)
	}
	w = serve(machine(150*time.Millisecond, http.StatusOK, `{"parts":{},"failed":[],"tookMs":150}`))
	if w.Code != http.StatusOK || w.Header().Get(HomeFromHeader) != "machine" {
		t.Fatalf("no store, late machine: %d %s", w.Code, w.Body.String())
	}
	// Neither: 503, not 404 which would turn the home off on the page.
	w = serve(machine(0, http.StatusBadGateway, `down`))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "home_unavailable") {
		t.Fatalf("neither: %d %s", w.Code, w.Body.String())
	}
}

type waitingLibrary struct {
	fakeLibrary
	head  int64
	space string
}

func (f *waitingLibrary) WaitLibrary(ctx context.Context, spaceID string, since int64) (int64, error) {
	f.space = spaceID
	if f.head > since {
		return f.head, nil
	}
	<-ctx.Done()
	return f.head, nil
}

// the page's long poll of the library answers the newest revision at once when it is past since, holds the
// ask until the wait ends otherwise, and is behind hub-design-home.
func TestLibraryChangesLongPoll(t *testing.T) {
	old := libraryWait
	libraryWait = 50 * time.Millisecond
	t.Cleanup(func() { libraryWait = old })

	r := newSpaceRig(t, true)
	lib := &waitingLibrary{head: 7}
	r.svc.UseView(lib)
	ask := func(since string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/m/"+r.owner.ID+"/api/platform/design/store-library/changes?since="+since, nil)
		w := httptest.NewRecorder()
		if !r.svc.ServeMachine(w, req, r.ownerU, r.owner) {
			t.Fatal("the hub left the long poll to the machine")
		}
		return w
	}
	if w := ask("0"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "home_off") {
		t.Fatalf("switch off: %d %s", w.Code, w.Body.String())
	}
	r.svc.homeOn = func(context.Context, string, string) bool { return true }
	revOf := func(w *httptest.ResponseRecorder) int64 {
		t.Helper()
		var got struct{ Rev int64 }
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Rev <= 0 {
			t.Fatalf("answer: %d %s", w.Code, w.Body.String())
		}
		return got.Rev
	}
	first := revOf(ask("0"))
	if lib.space != r.owner.SpaceID {
		t.Fatalf("space %q", lib.space)
	}
	start := time.Now()
	if got := revOf(ask(strconv.FormatInt(first, 10))); got != first {
		t.Fatalf("nothing new moved the revision: %d -> %d", first, got)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("the hub did not hold the ask")
	}
}

// with the store on, a grant changes no file, yet the poll of the person it is given to wakes with a newer
// revision well before the wait ends; so do a revocation and the project's team level.
func TestLibraryChangesWakeOnAccess(t *testing.T) {
	old, oldPoll := libraryWait, grantedPoll
	libraryWait, grantedPoll = 5*time.Second, 20*time.Millisecond
	t.Cleanup(func() { libraryWait, grantedPoll = old, oldPoll })

	ctx := context.Background()
	r := newSpaceRig(t, true)
	for _, u := range []store.User{r.ownerU, r.mateU} {
		if err := r.db.S().CreateIdentity(ctx, &store.Identity{UserID: u.ID, Provider: "test", Subject: u.ID, Email: u.Email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
	}
	lib := &waitingLibrary{head: 3}
	r.svc.UseView(lib)
	r.svc.homeOn = func(context.Context, string, string) bool { return true }
	if _, err := r.svc.registerCreated(ctx, r.owner, "site", r.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	// Closed to the team: the mate sees it only by what each step below gives.
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error { return tx.setTeam("site", r.ownerU.Email, levelNone) }); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	// The held poll and the writes read the clock from two goroutines.
	var clockMu sync.Mutex
	r.svc.now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		clock = clock.Add(time.Millisecond)
		return clock
	}
	ask := func(since int64) (int64, time.Duration) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/m/"+r.mate.ID+"/api/platform/design/store-library/changes?since="+strconv.FormatInt(since, 10), nil)
		w := httptest.NewRecorder()
		start := time.Now()
		if !r.svc.ServeMachine(w, req, r.mateU, r.mate) {
			t.Fatal("the hub left the long poll to the machine")
		}
		var got struct{ Rev int64 }
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil {
			t.Fatalf("answer: %d %s", w.Code, w.Body.String())
		}
		return got.Rev, time.Since(start)
	}
	// Wakes a poll held at rev with change() made while it waits; the answer comes long before the wait's end.
	wakes := func(name string, rev int64, change func(tx *txn) error) int64 {
		t.Helper()
		done := make(chan struct{})
		var got int64
		var took time.Duration
		go func() { defer close(done); got, took = ask(rev) }()
		time.Sleep(60 * time.Millisecond)
		if err := r.svc.write(ctx, r.owner, change); err != nil {
			t.Fatal(err)
		}
		<-done
		if got <= rev || took > 2*time.Second {
			t.Fatalf("%s: rev %d -> %d after %v", name, rev, got, took)
		}
		return got
	}
	rev, _ := ask(0)
	rev = wakes("grant", rev, func(tx *txn) error {
		_, err := tx.upsertMember("site", r.ownerU.Email, r.mateU.Email, roleView, nil, "")
		return err
	})
	rev = wakes("revoke", rev, func(tx *txn) error { return tx.revoke("site", r.ownerU.Email, r.mateU.Email, false) })
	wakes("team level", rev, func(tx *txn) error { return tx.setTeam("site", r.ownerU.Email, roleView) })
}
