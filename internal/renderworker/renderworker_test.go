package renderworker

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type fakeShooter struct{ url, how string }

func (f *fakeShooter) ShootThumb(_ context.Context, url, _ string, _, _ int) ([]byte, error) {
	f.url, f.how = url, "thumb"
	return []byte{0xff, 0xd8, 0xff, 1}, nil
}

func (f *fakeShooter) ShootPage(_ context.Context, url, _ string, _, _ int) ([]byte, error) {
	f.url, f.how = url, "page"
	return []byte{0xff, 0xd8, 0xff, 2}, nil
}

// the worker opens the board at the hub's address the job gives and sends the picture back for the job.
func TestWorkerDrawsTheClaimedBoard(t *testing.T) {
	var mu sync.Mutex
	var got []byte
	var doneQuery string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case claimPath:
			_, _ = io.WriteString(w, `{"id":"j1","project":"p","board":"a.html","rev":3,"path":"/m/m1/design/store/p/k/key/f/a.html"}`)
		case donePath:
			mu.Lock()
			got, _ = io.ReadAll(r.Body)
			doneQuery = r.URL.RawQuery
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer hub.Close()
	sh := &fakeShooter{}
	w := &Worker{Hub: hub.URL, Token: "tok", Shoot: sh, Client: hub.Client(), Log: slog.New(slog.DiscardHandler)}
	if ok, err := w.Once(context.Background()); !ok || err != nil {
		t.Fatalf("once: %v %v", ok, err)
	}
	if sh.url != hub.URL+"/m/m1/design/store/p/k/key/f/a.html" || sh.how != "thumb" {
		t.Fatalf("drew %s", sh.url)
	}
	mu.Lock()
	defer mu.Unlock()
	if doneQuery != "id=j1" || len(got) != 4 {
		t.Fatalf("answer: %q %d bytes", doneQuery, len(got))
	}
}

// a page of a guest link's snapshot is drawn only cleanly (design.Shooter.ShootPage).
func TestWorkerDrawsAPageCleanly(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case claimPath:
			_, _ = io.WriteString(w, `{"id":"j2","project":"p","board":"a.html","kind":"page","rev":3,"path":"/m/m1/design/store/p/k/key/v/3/f/a.html"}`)
		case donePath:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer hub.Close()
	sh := &fakeShooter{}
	w := &Worker{Hub: hub.URL, Token: "tok", Shoot: sh, Client: hub.Client(), Log: slog.New(slog.DiscardHandler)}
	if ok, err := w.Once(context.Background()); !ok || err != nil {
		t.Fatalf("once: %v %v", ok, err)
	}
	if sh.how != "page" || sh.url != hub.URL+"/m/m1/design/store/p/k/key/v/3/f/a.html" {
		t.Fatalf("drew %s as %s", sh.url, sh.how)
	}
}
