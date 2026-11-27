package designaccess

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

// A guest's address is what the connection says, and X-Forwarded-For only from a trusted proxy: otherwise a new header
// with every request is a new budget.
func TestGuestBudgetIsNotBeatenByAForwardedHeader(t *testing.T) {
	r := newSpaceRig(t, true)
	r.svc.trusted = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	path := "/s/" + strings.Repeat("a", 22) + "/index.json"
	flood := func(peer string) (limited bool) {
		for i := range ipPerMinute + 1 {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = peer + ":1000"
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.%d.%d", i/250, i%250))
			w := httptest.NewRecorder()
			r.svc.Public().ServeHTTP(w, req)
			limited = limited || w.Code == http.StatusTooManyRequests
		}
		return limited
	}
	if !flood("192.0.2.1") {
		t.Fatal("a direct caller got a new budget with every header")
	}
	if flood("10.0.0.1") {
		t.Fatal("guests behind a trusted proxy share one budget")
	}
}
