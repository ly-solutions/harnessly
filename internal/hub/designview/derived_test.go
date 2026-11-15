package designview

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// memDerived keeps the answers in memory, as the hub's database and store do for every replica.
type memDerived struct {
	mu    sync.Mutex
	rows  map[string]DerivedAnswer
	blobs map[string][]byte
}

func newMemDerived() *memDerived {
	return &memDerived{rows: map[string]DerivedAnswer{}, blobs: map[string][]byte{}}
}

func (m *memDerived) Lookup(_ context.Context, key string) (DerivedAnswer, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.rows[key]
	return a, ok
}

func (m *memDerived) Keep(_ context.Context, a DerivedAnswer, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[a.Key] = a
	m.blobs[a.Sha] = append([]byte(nil), body...)
	return nil
}

func (m *memDerived) Open(_ context.Context, _, sha string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blobs[sha]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(string(b))), nil
}

func ask(t *testing.T, v *View, at int64, headers []string, segs ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/"+strings.Join(segs, "/"), nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	if err := v.Serve(w, req, "space", proj, at, segs, nil, false); err != nil {
		t.Fatal(err)
	}
	return w
}

// what one replica worked out from a revision another replica gives from the store, without reading the
// project: the list of the boards, a board's page and a file answer the same, the page answers 304 by its hash, and a
// new revision is worked out anew.
func TestAnAnswerAboutARevisionIsWorkedOutOnce(t *testing.T) {
	room := newMemRoom()
	rev := room.commit(deckFiles("<main class=\"board\">two</main>"))
	kept := newMemDerived()
	rooms := map[string]*memRoom{"space": room}
	first := liveView(t, rooms, 12)
	first.UseDerived(kept, "build-1")
	want := map[string]string{}
	paths := [][]string{nil, {"one.dc.html"}, {"ds", "kit", "theme.css"}}
	for _, segs := range paths {
		w := ask(t, first, rev, nil, segs...)
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("%v from the folder: %d", segs, w.Code)
		}
		want[strings.Join(segs, "/")] = w.Body.String()
	}
	if len(kept.rows) != len(paths) {
		t.Fatalf("kept answers: %d, want %d", len(kept.rows), len(paths))
	}

	// Another replica: a folder of its own, nothing of the project in it, and the room is not read at all.
	second := liveView(t, rooms, 12)
	second.UseDerived(kept, "build-1")
	room.mu.Lock()
	before := room.reads
	room.mu.Unlock()
	var etag string
	for _, segs := range paths {
		w := ask(t, second, rev, nil, segs...)
		if w.Code != 200 || w.Body.String() != want[strings.Join(segs, "/")] {
			t.Fatalf("%v from the store: %d %q", segs, w.Code, w.Body.String())
		}
		if etag = w.Header().Get("ETag"); etag == "" {
			t.Fatalf("%v carries no validator: %v", segs, w.Header())
		}
	}
	if w := ask(t, second, rev, []string{"If-None-Match", etag}, paths[len(paths)-1]...); w.Code != 304 {
		t.Fatalf("the same contents again: %d", w.Code)
	}
	room.mu.Lock()
	after := room.reads
	room.mu.Unlock()
	if after != before {
		t.Fatalf("the second replica read %d blobs of the project", after-before)
	}

	// A new revision is another answer; another build's answers are not this build's.
	next := room.commit(deckFiles("<main class=\"board\">two, changed</main>"))
	if w := ask(t, second, next, nil, "two.dc.html"); w.Code != 200 || !strings.Contains(w.Body.String(), "two, changed") {
		t.Fatalf("the new revision: %d %q", w.Code, w.Body.String())
	}
	third := liveView(t, rooms, 12)
	third.UseDerived(kept, "build-2")
	n := len(kept.rows)
	if w := ask(t, third, rev, nil, "one.dc.html"); w.Code != 200 || len(kept.rows) != n+1 {
		t.Fatalf("another build works its own answer out: %d, kept %d → %d", w.Code, n, len(kept.rows))
	}
}

// A kept answer whose blob the store lost is worked out again; a range that is not kept yet goes the old way and is
// not kept as the whole answer.
func TestALostOrPartialAnswerIsNotServedFromTheStore(t *testing.T) {
	room := newMemRoom()
	rev := room.commit(deckFiles("<main class=\"board\">two</main>"))
	kept := newMemDerived()
	v := liveView(t, map[string]*memRoom{"space": room}, 12)
	v.UseDerived(kept, "b")
	if w := ask(t, v, rev, []string{"Range", "bytes=0-3"}, "ds", "kit", "theme.css"); w.Code != 200 && w.Code != 206 {
		t.Fatalf("a range the old way: %d", w.Code)
	}
	if len(kept.rows) != 0 {
		t.Fatalf("a range was kept as the answer: %d", len(kept.rows))
	}
	body := ask(t, v, rev, nil, "one.dc.html").Body.String()
	kept.mu.Lock()
	kept.blobs = map[string][]byte{}
	kept.mu.Unlock()
	if w := ask(t, v, rev, nil, "one.dc.html"); w.Code != 200 || w.Body.String() != body {
		t.Fatalf("after the blob is lost: %d %q", w.Code, w.Body.String())
	}
	if len(kept.blobs) != 1 {
		t.Fatalf("the answer was not kept again: %d", len(kept.blobs))
	}
}
