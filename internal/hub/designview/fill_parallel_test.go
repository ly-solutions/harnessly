package designview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// slowRoom answers a blob after a moment and remembers how many were being read at once.
type slowRoom struct {
	*memRoom
	mu      sync.Mutex
	reading int
	peak    int
	broken  string
}

func (r *slowRoom) OpenBlob(ctx context.Context, sha string) (io.ReadCloser, error) {
	r.mu.Lock()
	r.reading++
	r.peak = max(r.peak, r.reading)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.reading--
		r.mu.Unlock()
	}()
	select {
	case <-time.After(15 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if sha == r.broken {
		return nil, errors.New("the bucket did not answer")
	}
	return r.memRoom.OpenBlob(ctx, sha)
}

// the first look at a project on a hub that has none of its contents reads them several at a time, never
// more than fillBlobs, and the page it answers is whole.
func TestAColdRevisionIsReadSeveralBlobsAtATime(t *testing.T) {
	files := deckFiles("<main class=\"board\">two</main>")
	for i := range 40 {
		files[fmt.Sprintf("project/assets/a%02d.css", i)] = fmt.Sprintf("body{margin:%dpx}", i)
	}
	room := &slowRoom{memRoom: newMemRoom()}
	at := room.commit(files)
	v := New(Options{Rooms: func(context.Context, string) (Room, error) { return room, nil }, Dir: t.TempDir(), Poll: time.Millisecond})
	w := httptest.NewRecorder()
	if err := v.Serve(w, httptest.NewRequest("GET", "/two.dc.html", nil), "s", "deck", at, []string{"two.dc.html"}, nil, false); err != nil || w.Code != 200 {
		t.Fatalf("the first look: %v %d", err, w.Code)
	}
	if room.peak < 2 || room.peak > fillBlobs {
		t.Fatalf("%d blobs were read at once, want 2..%d", room.peak, fillBlobs)
	}
	if room.reads != len(files) {
		t.Fatalf("%d blobs read for %d files", room.reads, len(files))
	}
}

// one blob the store does not give fails the revision, as it did when they were read one by one.
func TestAColdRevisionFailsWithABlobItCannotRead(t *testing.T) {
	files := deckFiles("<main class=\"board\">two</main>")
	room := &slowRoom{memRoom: newMemRoom(), broken: shaOf("body{margin:0}")}
	at := room.commit(files)
	v := New(Options{Rooms: func(context.Context, string) (Room, error) { return room, nil }, Dir: t.TempDir(), Poll: time.Millisecond, Live: -1})
	w := httptest.NewRecorder()
	if err := v.Serve(w, httptest.NewRequest("GET", "/two.dc.html", nil), "s", "deck", at, []string{"two.dc.html"}, nil, false); err == nil {
		t.Fatalf("a revision with an unreadable blob was answered: %d", w.Code)
	}
}
