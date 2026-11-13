package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/files"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// A chat whose folder lies outside the viewer's places (a project of the shared design library) shows its
// files through the chat: a video of more than the 15 MB a picture may have, in a folder with a dot, goes in ranges
// with its type. What lies outside the folder and the places, a secret and a link out of the folder stay closed.
func TestV2FilesShowAVideoOfTheChatFolderOutsideThePlaces(t *testing.T) {
	place, _ := filepath.EvalSymlinks(t.TempDir())
	library, _ := filepath.EvalSymlinks(t.TempDir())
	folder := filepath.Join(library, "project")
	if err := os.MkdirAll(filepath.Join(folder, ".media", "build-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(folder, ".media", "build-1", "render.mp4")
	const size = files.MaxPreviewBytes + 1<<20
	body := make([]byte, size)
	copy(body[size-4:], "tail")
	copy(body, "head")
	other := filepath.Join(library, "other", "clip.mp4")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, b := range map[string][]byte{clip: body, other: []byte("x"), filepath.Join(folder, ".env"): []byte("x"), filepath.Join(folder, "notes.bin"): []byte("x")} {
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(other, filepath.Join(folder, "linked.mp4")); err != nil {
		t.Fatal(err)
	}
	f := &v2Files{
		record: func(id string) (wire.Conversation, bool) { return wire.Conversation{ID: id, Cwd: folder}, true },
		access: files.NewAccess([]string{place}, nil),
	}
	get := func(query string, headers ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v2/conversations/c/download?"+query, nil)
		for i := 0; i+1 < len(headers); i += 2 {
			r.Header.Set(headers[i], headers[i+1])
		}
		w := httptest.NewRecorder()
		f.ServeDownload(w, r, "c")
		return w
	}
	shown := func(p string) string { return "show=1&path=" + url.QueryEscape(p) }

	// The viewer's own route does not open it: the folder is none of its places.
	if got := f.access.Preview(clip); got.OK || got.Status != http.StatusForbidden {
		t.Fatalf("the viewer's route: %+v", got)
	}
	for _, path := range []string{clip, ".media/build-1/render.mp4"} {
		w := get(shown(path), "Range", "bytes=0-3")
		h := w.Header()
		if w.Code != http.StatusPartialContent || h.Get("Content-Type") != "video/mp4" || h.Get("Accept-Ranges") != "bytes" ||
			h.Get("Content-Range") != "bytes 0-3/"+itoa(size) || w.Body.String() != "head" || h.Get("Content-Disposition") != "" {
			t.Fatalf("%s, first bytes: %d %v %q", path, w.Code, h, w.Body.String())
		}
	}
	if w := get(shown(clip), "Range", "bytes=-4"); w.Code != http.StatusPartialContent || w.Body.String() != "tail" {
		t.Fatalf("last bytes: %d %q", w.Code, w.Body.String())
	}
	if w := get(shown(clip)); w.Code != http.StatusOK || w.Body.Len() != size || w.Header().Get("Content-Length") != itoa(size) {
		t.Fatalf("whole: %d, %d bytes, %v", w.Code, w.Body.Len(), w.Header())
	}
	if w := get(shown(clip), "Range", "bytes="+itoa(size)+"-"); w.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("a range past the end: %d", w.Code)
	}
	for name, c := range map[string]struct {
		query string
		code  int
	}{
		"a file of another project": {shown(other), http.StatusForbidden},
		"a link out of the folder":  {shown("linked.mp4"), http.StatusForbidden},
		"a secret":                  {shown(".env"), http.StatusForbidden},
		"a type the viewer has not": {shown("notes.bin"), http.StatusUnsupportedMediaType},
		"a folder":                  {shown(".media"), http.StatusBadRequest},
		"two files":                 {shown(clip) + "&path=notes.bin", http.StatusBadRequest},
		"a file that is not there":  {shown("gone.mp4"), http.StatusNotFound},
		"a path out of the folder":  {shown("../other/clip.mp4"), http.StatusForbidden},
		"the same file to be saved": {"path=" + url.QueryEscape(clip), http.StatusOK},
		"another project's to save": {"path=" + url.QueryEscape(other), http.StatusForbidden},
	} {
		w := get(c.query)
		if w.Code != c.code {
			t.Errorf("%s: %d, want %d (%s)", name, w.Code, c.code, strings.TrimSpace(w.Body.String()[:min(w.Body.Len(), 120)]))
		}
	}
}
