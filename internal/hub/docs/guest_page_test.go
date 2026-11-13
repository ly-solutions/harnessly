package docs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/resaccess"
)

func TestGuestLinkOpensTheDocumentPage(t *testing.T) {
	s, mux, _ := accessRig(t)
	var asked []string
	art := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.RequestURI())
		if r.URL.Path != "/a/plan/raw" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<html><body><h1>Plan</h1></body></html>"))
	}))
	t.Cleanup(art.Close)
	s.art = newArtifacts(art.URL, "t", art.Client())

	owner := resaccess.Actor{Email: "author@example.com", Name: "Author"}
	ref := resaccess.Ref{Kind: resaccess.KindDoc, ID: "plan"}
	call := func(rest []string, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.access.Serve(w, httptest.NewRequest("POST", "/x", strings.NewReader(body)), owner, ref, rest, resaccess.Info{Owner: owner.Email}, nil)
		return w
	}
	w := call([]string{"shares"}, `{"rights":"view","ttl":"1d"}`)
	var out struct{ URL string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 201 || !strings.HasPrefix(out.URL, "https://hub.example.com/d/") {
		t.Fatalf("link: %d %s", w.Code, w.Body.String())
	}
	open := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	path := strings.TrimPrefix(out.URL, "https://hub.example.com")

	w = open(path)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<h1>Plan</h1>") {
		t.Fatalf("the guest sees the document: %d %s", w.Code, w.Body.String())
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox") {
		t.Fatalf("the page runs sandboxed: %q", csp)
	}
	if w.Header().Get("X-Robots-Tag") == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("not indexed, not cached: %v", w.Header())
	}
	if strings.Contains(w.Body.String(), "parentWin") {
		t.Fatal("a page of its own carries no frame script")
	}
	var opens int
	if err := s.db.S().Q().QueryRow(context.Background(), `SELECT opens FROM resource_guest_links`).Scan(&opens); err != nil || opens != 1 {
		t.Fatalf("the open is counted: %d %v", opens, err)
	}

	if w := open("/d/nope"); w.Code != 404 || !strings.Contains(w.Body.String(), "Ссылка не найдена") {
		t.Fatalf("unknown token: %d %s", w.Code, w.Body.String())
	}
	if w := call([]string{"revoke-all"}, `{}`); w.Code >= 300 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if w := open(path); w.Code != 410 {
		t.Fatalf("a revoked link is gone: %d %s", w.Code, w.Body.String())
	}
	if len(asked) != 1 {
		t.Fatalf("the documents service is asked only for a live link: %v", asked)
	}
}

// the document's page names its fonts by a path of its own origin; the hub reads them from the documents
// service for anybody, and nothing else of that service is reachable this way.
func TestGuestPageFontsComeThroughTheHub(t *testing.T) {
	s, mux, _ := accessRig(t)
	var asked []string
	art := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.URL.Path != "/static/fonts/geist-400.woff2" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("wOF2-bytes"))
	}))
	t.Cleanup(art.Close)
	s.art = newArtifacts(art.URL, "t", art.Client())
	open := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := open("/static/fonts/geist-400.woff2")
	if w.Code != 200 || w.Body.String() != "wOF2-bytes" || w.Header().Get("Content-Type") != "font/woff2" || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("the font: %d %v", w.Code, w.Header())
	}
	if w := open("/static/fonts/none.woff2"); w.Code != 404 {
		t.Fatalf("a font the service has not got: %d", w.Code)
	}
	for _, p := range []string{"/static/fonts/x.css", "/static/fonts/..%2Fa%2Fplan%2Fraw", "/static/fonts/A.woff2"} {
		if w := open(p); w.Code != 404 {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	if len(asked) != 2 {
		t.Fatalf("only a font's name reaches the documents service: %v", asked)
	}
}
