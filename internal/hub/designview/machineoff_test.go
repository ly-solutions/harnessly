package designview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// a site is made of the project at a revision of the store by the runtime's own publishing code.
func TestPublishFilesFromTheStore(t *testing.T) {
	room := newMemRoom()
	at := room.commit(map[string]string{
		"artifact.json":        `{"title":"Лендинг"}`,
		"project/home.dc.html": "<html><body>home</body></html>",
	})
	v := New(Options{Rooms: func(context.Context, string) (Room, error) { return room, nil }, Dir: t.TempDir(), Poll: time.Millisecond})
	title, files, err := v.PublishFiles(context.Background(), "s1", "land", at, nil)
	if err != nil {
		t.Fatal(err)
	}
	if title == "" || len(files) == 0 {
		t.Fatalf("site: %q %d files", title, len(files))
	}
	html := false
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".html") && strings.Contains(string(f.Data), "home") {
			html = true
		}
	}
	if !html {
		t.Fatalf("no page of the board among %d files", len(files))
	}
}

// the reads of kits are answered from the space's kits collection, by the runtime's own handlers.
func TestServeKitsFromTheStore(t *testing.T) {
	kits := newMemRoom()
	kits.commit(map[string]string{"brand/kit.json": `{"name":"Бренд"}`})
	v := New(Options{
		Rooms: func(context.Context, string) (Room, error) { return newMemRoom(), nil },
		Kits:  func(context.Context, string) (Room, error) { return kits, nil },
		Dir:   t.TempDir(), Poll: time.Millisecond,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/platform/design/kits/state?kit=brand", nil)
	if err := v.ServeKits(w, r, "s1", []string{"brand", "../bad"}, "kits/state"); err != nil {
		t.Fatal(err)
	}
	if w.Code >= http.StatusInternalServerError || w.Body.Len() == 0 {
		t.Fatalf("kits/state: %d %s", w.Code, w.Body.String())
	}
	if err := v.ServeKits(httptest.NewRecorder(), r, "s1", nil, "kit/unknown"); err != ErrMissing {
		t.Fatalf("an unknown route: %v", err)
	}
}

// a project on a kit is shown by the hub with its kit: the revision's library reads the kit laid out from the
// kits collection, so a guest link and the render worker draw the board with the kit's styles while the machine is off.
func TestProjectOnAKitReadsItFromTheStore(t *testing.T) {
	room := newMemRoom()
	at := room.commit(map[string]string{
		"artifact.json":        `{"title":"Лендинг"}`,
		"meta/project.json":    `{"kit":{"id":"ui","version":"1.0.0"}}`,
		"project/home.dc.html": "<html><body>home</body></html>",
	})
	kits := newMemRoom()
	kits.commit(map[string]string{"1.0.0/kit.json": `{"name":"UI"}`, "1.0.0/tokens.css": ":root{--brand:#123}"})
	v := New(Options{
		Rooms: func(context.Context, string) (Room, error) { return room, nil },
		Kits:  func(context.Context, string) (Room, error) { return kits, nil },
		Dir:   t.TempDir(), Poll: time.Millisecond,
	})
	s, err := v.snapshot(context.Background(), "s1", "land", at)
	if err != nil {
		t.Fatal(err)
	}
	dir := s.api.L.KitsDir()
	if b, err := os.ReadFile(filepath.Join(dir, "ui", "1.0.0", "tokens.css")); err != nil || !strings.Contains(string(b), "--brand") {
		t.Fatalf("the project's kit is not in its library's kits (%q): %v", dir, err)
	}
	// A project on no kit reads none.
	plain := newMemRoom()
	at = plain.commit(map[string]string{"artifact.json": `{"title":"Без кита"}`, "project/home.dc.html": "<html></html>"})
	v2 := New(Options{
		Rooms: func(context.Context, string) (Room, error) { return plain, nil },
		Kits:  func(context.Context, string) (Room, error) { return kits, nil },
		Dir:   t.TempDir(), Poll: time.Millisecond,
	})
	s, err = v2.snapshot(context.Background(), "s1", "plain", at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.api.L.KitsDir(), "ui")); !os.IsNotExist(err) {
		t.Fatalf("a project on no kit reads a kit: %v", err)
	}
}
