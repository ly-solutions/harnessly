package proxy_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ly-solutions/harnessly/internal/files"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// A video the pane plays comes from the machine through the hub in ranges: Safari plays nothing without a
// 206. The hub passes the Range on and the machine's 206, type and Content-Range back, for a file of more than the
// 15 MB a picture may have that lies in a folder with a dot.
func TestAVideoOfTheMachineComesThroughTheHubInRanges(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	clip := filepath.Join(dir, ".media", "render.mp4")
	if err := os.MkdirAll(filepath.Dir(clip), 0o755); err != nil {
		t.Fatal(err)
	}
	const size = files.MaxPreviewBytes + 1<<20
	body := make([]byte, size)
	copy(body, "head")
	copy(body[size-4:], "tail")
	if err := os.WriteFile(clip, body, 0o644); err != nil {
		t.Fatal(err)
	}
	owner := hubtest.New(t).SignIn("owner@example.com")
	space, _ := owner.NewSpace("Команда")
	id := owner.Machine(space, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		files.ServeShown(w, r, clip)
	})).MachineID
	path := "/m/" + id + "/api/v2/conversations/c/download?show=1&path=.media%2Frender.mp4"
	total := strconv.Itoa(size)

	r := owner.Do("GET", path, nil, "Range", "bytes=0-3")
	if r.Code != http.StatusPartialContent || string(r.Body) != "head" || r.Header.Get("Content-Type") != "video/mp4" ||
		r.Header.Get("Accept-Ranges") != "bytes" || r.Header.Get("Content-Range") != "bytes 0-3/"+total || r.Header.Get("Content-Length") != "4" {
		t.Fatalf("first bytes: %d %q %v", r.Code, r.Body, r.Header)
	}
	r = owner.Do("GET", path, nil, "Range", "bytes="+strconv.Itoa(size-4)+"-")
	if r.Code != http.StatusPartialContent || string(r.Body) != "tail" || r.Header.Get("Content-Range") != "bytes "+strconv.Itoa(size-4)+"-"+strconv.Itoa(size-1)+"/"+total {
		t.Fatalf("last bytes: %d %q %v", r.Code, r.Body, r.Header)
	}
	r = owner.Do("GET", path, nil)
	if r.Code != http.StatusOK || len(r.Body) != size || r.Header.Get("Content-Type") != "video/mp4" || r.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("whole: %d, %d bytes, %v", r.Code, len(r.Body), r.Header)
	}
	r = owner.Do("GET", path, nil, "Range", "bytes="+total+"-")
	if r.Code != http.StatusRequestedRangeNotSatisfiable || r.Header.Get("Content-Range") != "bytes */"+total {
		t.Fatalf("a range past the end: %d %v", r.Code, r.Header)
	}
}
