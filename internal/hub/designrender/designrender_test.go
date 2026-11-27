package designrender

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/store/storetest"
)

type fakeRoom struct {
	mu      sync.Mutex
	blobs   map[string][]byte
	entries map[string]filestore.Entry
}

func (r *fakeRoom) Current(_ context.Context, path string) (*filestore.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[path]; ok {
		return &e, nil
	}
	return nil, nil
}

func (r *fakeRoom) PutBlob(_ context.Context, sha string, body io.Reader) error {
	b, err := io.ReadAll(body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blobs[sha] = b
	return err
}

func (r *fakeRoom) Commit(_ context.Context, req filestore.CommitRequest, _ string) (*filestore.Entry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.entries[req.Path]
	if cur.Rev != req.Base {
		return nil, true, nil
	}
	e := filestore.Entry{Path: req.Path, Rev: cur.Rev + 1, Sha: req.Sha, Size: req.Size}
	r.entries[req.Path] = e
	return &e, false, nil
}

func (r *fakeRoom) OpenBlob(_ context.Context, sha string) (io.ReadCloser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.blobs[sha]
	if !ok {
		return nil, errors.New("no blob")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

const token = "0123456789abcdef0123456789abcdef" // gitleaks:allow (a test token, not a secret)

func setup(t *testing.T) (*Service, *fakeRoom, http.Handler) {
	t.Helper()
	room := &fakeRoom{blobs: map[string][]byte{}, entries: map[string]filestore.Entry{}}
	s := New(storetest.Open(t), func(context.Context, string) (Room, error) { return room, nil },
		func(j Job) string {
			return "/m/" + j.MachineID + j.Prefix + "/design/store/" + j.Project + "/k/key/f/" + j.Board
		},
		token, nil, nil)
	if s == nil {
		t.Fatal("no service")
	}
	mux := http.NewServeMux()
	mux.Handle("POST "+ClaimPath, httpx.Handler(s.ServeClaim))
	mux.Handle("POST "+DonePath, httpx.Handler(s.ServeDone))
	return s, room, mux
}

func call(h http.Handler, path string, body []byte, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func want(t *testing.T, s *Service, rev int64) {
	t.Helper()
	if err := s.Want(context.Background(), Job{SpaceID: "sp", MachineID: "m1", OwnerID: "u1", Prefix: "/api/x",
		Project: "p1", Board: "pages/home.html", Rev: rev, BoardW: 390, BoardH: 844}); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDrawsAndStoresThePicture(t *testing.T) {
	s, room, h := setup(t)
	if rec := call(h, ClaimPath, nil, "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("claim without the token: %d", rec.Code)
	}
	want(t, s, 7)
	want(t, s, 7) // the same board and revision is queued once
	rec := call(h, ClaimPath, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", rec.Code, rec.Body)
	}
	var j Job
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	if j.Path != "/m/m1/api/x/design/store/p1/k/key/f/pages/home.html" || j.BoardW != 390 || j.Rev != 7 {
		t.Fatalf("job: %+v", j)
	}
	if strings.Contains(rec.Body.String(), "u1") || strings.Contains(rec.Body.String(), `"sp"`) {
		t.Fatalf("the job tells the worker more than it needs: %s", rec.Body)
	}
	// The duplicate was not queued: nothing else is given out (the claim waits, so it is asked under a short context).
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if again, err := s.claim(ctx, false); err != nil || again != nil {
		t.Fatalf("a second job: %+v %v", again, err)
	}
	if rec := call(h, DonePath+"?id="+j.ID, []byte("not a jpeg"), token); rec.Code != http.StatusBadRequest {
		t.Fatalf("a body that is not a picture: %d", rec.Code)
	}
	pic := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{1}, 100)...)
	if rec := call(h, DonePath+"?id="+j.ID, pic, token); rec.Code != http.StatusNoContent {
		t.Fatalf("done: %d %s", rec.Code, rec.Body)
	}
	e, _ := room.Current(context.Background(), filestore.ThumbPath("p1", "pages/home.html"))
	if e == nil || !bytes.Equal(room.blobs[e.Sha], pic) {
		t.Fatalf("the picture is not in the thumbs collection: %+v", e)
	}
	if rec := call(h, DonePath+"?id="+j.ID, pic, token); rec.Code != http.StatusConflict {
		t.Fatalf("a job answered twice: %d", rec.Code)
	}
	// A newer revision of the project is drawn again.
	want(t, s, 8)
	if next, err := s.claim(context.Background(), false); err != nil || next == nil || next.Rev != 8 {
		t.Fatalf("the newer revision: %+v %v", next, err)
	}
}

func TestFailedJobIsTriedAgainThenLeft(t *testing.T) {
	s, _, h := setup(t)
	want(t, s, 1)
	for i := range maxAttempts {
		j, err := s.claim(context.Background(), false)
		if err != nil || j == nil {
			t.Fatalf("attempt %d: %+v %v", i+1, j, err)
		}
		if rec := call(h, DonePath+"?id="+j.ID+"&error=boom", nil, token); rec.Code != http.StatusNoContent {
			t.Fatalf("fail: %d", rec.Code)
		}
	}
	if j, err := s.claim(context.Background(), false); err != nil || j != nil {
		t.Fatalf("a job failed %d times is given out again: %+v %v", maxAttempts, j, err)
	}
}

func TestQuietWorkersJobIsGivenOutAgain(t *testing.T) {
	s, _, _ := setup(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	want(t, s, 1)
	if j, _ := s.claim(context.Background(), false); j == nil {
		t.Fatal("no job")
	}
	if j, _ := s.claim(context.Background(), false); j != nil {
		t.Fatal("a claimed job is given out while its lease runs")
	}
	now = now.Add(leaseFor + time.Second)
	if j, _ := s.claim(context.Background(), false); j == nil {
		t.Fatal("the job of a worker that went quiet is not given out again")
	}
}

func TestNoTokenNoQueue(t *testing.T) {
	if s := New(storetest.Open(t), func(context.Context, string) (Room, error) { return nil, nil },
		func(Job) string { return "/x" }, "", nil, nil); s != nil {
		t.Fatal("a queue without a worker's token")
	}
	var s *Service
	if err := s.Want(context.Background(), Job{}); err != nil {
		t.Fatal(err)
	}
}

// a cover job draws the project's first board and puts it into the covers collection, not the thumbs one;
// a queue without the covers collection queues no cover.
func TestCoverJobStoresTheCover(t *testing.T) {
	s, thumbs, h := setup(t)
	cover := Job{SpaceID: "sp", MachineID: "m1", OwnerID: "u1", Prefix: "/api/x", Project: "p1", Board: "pages/home.html",
		Kind: KindCover, Rev: 3}
	if err := s.Want(context.Background(), cover); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if j, err := s.claim(ctx, false); err != nil || j != nil {
		t.Fatalf("a cover queued without the covers collection: %+v %v", j, err)
	}
	covers := &fakeRoom{blobs: map[string][]byte{}, entries: map[string]filestore.Entry{}}
	s.UseCovers(func(context.Context, string) (Room, error) { return covers, nil })
	if err := s.Want(context.Background(), cover); err != nil {
		t.Fatal(err)
	}
	rec := call(h, ClaimPath, nil, token)
	var j Job
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil || j.Kind != KindCover {
		t.Fatalf("claim: %d %s", rec.Code, rec.Body)
	}
	pic := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{2}, 50)...)
	if rec := call(h, DonePath+"?id="+j.ID, pic, token); rec.Code != http.StatusNoContent {
		t.Fatalf("done: %d %s", rec.Code, rec.Body)
	}
	e, _ := covers.Current(context.Background(), filestore.CoverPath("p1"))
	if e == nil || !bytes.Equal(covers.blobs[e.Sha], pic) {
		t.Fatalf("the cover is not in the covers collection: %+v", e)
	}
	if len(thumbs.entries) != 0 {
		t.Fatalf("the cover went into the thumbs collection too: %v", thumbs.entries)
	}
}

// a page of a guest link's snapshot is drawn at its revision and handed to the hub that waits for it; it is
// given out before the gallery's pictures, commits nothing, and fails at the first error.
func TestPageIsDrawnForTheWaitingHub(t *testing.T) {
	old := pagePoll
	pagePoll = 10 * time.Millisecond
	defer func() { pagePoll = old }()
	s, room, h := setup(t)
	s.locate = func(j Job) string { return "/at/" + j.Kind }
	want(t, s, 7) // a gallery picture queued first
	page := Job{SpaceID: "sp", MachineID: "m1", OwnerID: "u1", Prefix: "/api/x", Project: "p1", Board: "pages/home.html",
		Rev: 5, BoardW: 390, BoardH: 844}
	pic := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{3}, 64)...)
	got := make(chan []byte, 1)
	failed := make(chan error, 1)
	go func() {
		b, err := s.Page(context.Background(), page)
		if err != nil {
			failed <- err
			return
		}
		got <- b
	}()
	queued(t, s, 5)
	var j Job
	for deadline := time.Now().Add(5 * time.Second); ; {
		rec := call(h, ClaimPath, nil, token)
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no job")
		}
	}
	if j.Kind != KindPage || j.Rev != 5 || j.Path != "/at/page" {
		t.Fatalf("the waited page is not given out first: %+v", j)
	}
	if rec := call(h, DonePath+"?id="+j.ID, pic, token); rec.Code != http.StatusNoContent {
		t.Fatalf("done: %d %s", rec.Code, rec.Body)
	}
	select {
	case b := <-got:
		if !bytes.Equal(b, pic) {
			t.Fatal("another picture")
		}
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("the page did not reach the waiting hub")
	}
	if len(room.entries) != 0 {
		t.Fatalf("a snapshot's page was committed: %v", room.entries)
	}
	// Asked again, the drawn page is given at once.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if b, err := s.Page(ctx, page); err != nil || !bytes.Equal(b, pic) {
		t.Fatalf("again: %v", err)
	}

	// A page that does not draw cleanly fails at once, and the next ask tries it anew.
	page.Rev = 6
	go func() {
		_, err := s.Page(context.Background(), page)
		failed <- err
	}()
	queued(t, s, 6)
	for deadline := time.Now().Add(5 * time.Second); ; {
		rec := call(h, ClaimPath, nil, token)
		if rec.Code == http.StatusOK {
			_ = json.Unmarshal(rec.Body.Bytes(), &j)
			if j.Kind == KindPage {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no page job")
		}
	}
	if rec := call(h, DonePath+"?id="+j.ID+"&error=dirty", nil, token); rec.Code != http.StatusNoContent {
		t.Fatalf("fail: %d", rec.Code)
	}
	select {
	case err := <-failed:
		if !errors.Is(err, ErrPageFailed) {
			t.Fatalf("want ErrPageFailed, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the failure did not reach the waiting hub")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	_, _ = s.Page(ctx2, page)
	if next, err := s.claim(context.Background(), false); err != nil || next == nil || next.Kind != KindPage || next.Rev != 6 {
		t.Fatalf("a failed page is not tried again on the next ask: %+v %v", next, err)
	}
}

// queued waits until the page at rev is in the queue.
func queued(t *testing.T, s *Service, rev int64) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		var n int
		if err := s.db.QueryRow(context.Background(), `SELECT COUNT(*) FROM design_render_jobs WHERE kind = 'page' AND rev = ?
			AND state = 'queued'`, rev).Scan(&n); err == nil && n > 0 {
			return
		}
	}
	t.Fatalf("the page at %d was not queued", rev)
}
