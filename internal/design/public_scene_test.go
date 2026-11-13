package design

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// HRN-1262 with HRN-1258: the runtime a scene off the machine gets finds its fonts beside itself and the guest board
// script, never the machine's own /__lib/ paths.
func TestPublicRuntimeNamesNoMachinePaths(t *testing.T) {
	a := &API{L: &Library{}}
	w := httptest.NewRecorder()
	a.publicLib(w, httptest.NewRequest(http.MethodGet, "/x/video-runtime.js", nil), []string{"video-runtime.js"}, false)
	if w.Code != 200 || strings.Contains(w.Body.String(), "'/__lib/") {
		t.Fatalf("runtime: %d, still names the machine's paths", w.Code)
	}
}
