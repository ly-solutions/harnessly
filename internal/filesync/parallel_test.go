package filesync

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
)

type wire struct {
	srv                          *httptest.Server
	delay                        time.Duration
	old                          bool // a store that knows no compression
	now, peak                    atomic.Int32
	gzipPuts, gzipGets, blobGets atomic.Int32
}

func newWire(t testing.TB) *wire {
	t.Helper()
	st, err := filestore.OpenFolder(t.TempDir(), func(*http.Request, string) (filestore.Access, error) {
		return filestore.Access{Room: "design-s", Member: "m"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	w := &wire{}
	h := st.Handler()
	w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/") {
			n := w.now.Add(1)
			defer w.now.Add(-1)
			for p := w.peak.Load(); n > p && !w.peak.CompareAndSwap(p, n); p = w.peak.Load() {
			}
			if r.Header.Get("Content-Encoding") == "gzip" {
				w.gzipPuts.Add(1)
			}
			if r.Method == http.MethodGet {
				w.blobGets.Add(1)
			}
		}
		time.Sleep(w.delay)
		if w.old {
			r.Header.Del("Accept-Encoding")
		}
		h.ServeHTTP(rw, r)
		if r.Method == http.MethodGet && rw.Header().Get("Content-Encoding") == "gzip" {
			w.gzipGets.Add(1)
		}
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *wire) agent(t testing.TB, dir string) *Agent {
	t.Helper()
	a, err := New(Config{Root: dir, StateDir: t.TempDir(), Machine: "m", Store: NewClient(w.srv.URL, "design", nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

func library(t testing.TB, n int) (string, map[string][]byte) {
	t.Helper()
	dir, files := t.TempDir(), map[string][]byte{}
	for i := range n {
		name := fmt.Sprintf("p%d/project/b%d.dc.html", i%5, i)
		data := []byte(strings.Repeat(fmt.Sprintf("<div class=\"board\">Доска %d</div>\n", i), 200))
		if i%7 == 0 { // the same contents under two names
			data = []byte(strings.Repeat("<p>общий файл</p>\n", 300))
		}
		files[name] = data
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, files
}

func pullAll(t testing.TB, a *Agent) {
	t.Helper()
	for {
		more, err := a.Pull(context.Background(), 0)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
}

func same(t testing.TB, dir string, files map[string][]byte) {
	t.Helper()
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %d bytes, %v", name, len(got), err)
		}
	}
}

// contents travel several at once and compressed, with the new and with an older store alike, and nothing of
// the fetching ahead stays in the folder.
func TestSyncSendsAndFetchesTogether(t *testing.T) {
	for _, old := range []bool{false, true} {
		t.Run(fmt.Sprintf("oldStore=%v", old), func(t *testing.T) {
			w := newWire(t)
			w.old, w.delay = old, 5*time.Millisecond
			src, files := library(t, 150)
			if err := w.agent(t, src).ScanAll(context.Background()); err != nil {
				t.Fatal(err)
			}
			up := w.peak.Swap(0)
			dst := t.TempDir()
			b := w.agent(t, dst)
			pullAll(t, b)
			same(t, dst, files)
			if up < 2 || w.peak.Load() < 2 || int(w.peak.Load()) > transferWorkers {
				t.Fatalf("at once: up %d, down %d", up, w.peak.Load())
			}
			if gz := w.gzipPuts.Load() > 0 && w.gzipGets.Load() > 0; gz == old {
				t.Fatalf("compressed: %d up, %d down", w.gzipPuts.Load(), w.gzipGets.Load())
			}
			if g := int(w.blobGets.Load()); g >= len(files) {
				t.Fatalf("the same contents fetched twice: %d for %d files", g, len(files))
			}
			if left, _ := os.ReadDir(filepath.Join(dst, filestore.TmpDir)); len(left) != 0 {
				t.Fatalf("left in tmp: %d", len(left))
			}
		})
	}
}

// An unpacked upload is held to the same limit and must hash to its name.
func TestGzipUploadIsChecked(t *testing.T) {
	w := newWire(t)
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	_, _ = z.Write([]byte("other bytes"))
	_ = z.Close()
	for _, body := range [][]byte{buf.Bytes(), []byte("not gzip")} {
		req, _ := http.NewRequest(http.MethodPut, w.srv.URL+"/v1/collections/design/blobs/"+sha([]byte("x")), bytes.NewReader(body))
		req.Header.Set("Content-Encoding", "gzip")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("taken: %d", resp.StatusCode)
		}
	}
}

// HARNESSLY_SYNC_MEASURE=1 prints how long a first sync takes with one transfer at a time and with transferWorkers.
func TestMeasureSync(t *testing.T) {
	if os.Getenv("HARNESSLY_SYNC_MEASURE") == "" {
		t.Skip("a measurement")
	}
	was := transferWorkers
	defer func() { transferWorkers = was }()
	for _, n := range []int{1, was} {
		transferWorkers = n
		w := newWire(t)
		w.delay = 30 * time.Millisecond
		src, _ := library(t, 600)
		start := time.Now()
		if err := w.agent(t, src).ScanAll(context.Background()); err != nil {
			t.Fatal(err)
		}
		up := time.Since(start)
		start = time.Now()
		pullAll(t, w.agent(t, t.TempDir()))
		t.Logf("at once %d: 600 files, 30ms a request: up %v, down %v", n, up.Round(100*time.Millisecond), time.Since(start).Round(100*time.Millisecond))
	}
}
