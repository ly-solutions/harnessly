package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// a link to a chat from another site, opened through the app's service worker, reaches the hub as a
// navigation with no destination; it passes as a document does. A frame, a script or a plain request of another site
// is still refused.
func TestCrossSiteNavigationThroughTheWorkerPasses(t *testing.T) {
	for _, c := range []struct {
		mode, dest string
		refused    bool
	}{
		{"navigate", "document", false},
		{"navigate", "empty", false},
		{"navigate", "iframe", true},
		{"cors", "empty", true},
		{"no-cors", "script", true},
	} {
		r := httptest.NewRequest(http.MethodGet, "/agents/c/tab-x", nil)
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		r.Header.Set("Sec-Fetch-Mode", c.mode)
		r.Header.Set("Sec-Fetch-Dest", c.dest)
		if got := refuseCrossSite(httptest.NewRecorder(), r); got != c.refused {
			t.Errorf("%s/%s: refused %v, want %v", c.mode, c.dest, got, c.refused)
		}
	}
}
