package proxy_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// The gateway's assertion signs its bearer in to the hub, so it ends there: neither a machine nor a dev server in a
// preview is given it.
func TestTheGatewaysAssertionEndsAtTheHub(t *testing.T) {
	for _, header := range []string{"Teleport-Jwt-Assertion", "X-Forwarded-Access-Token"} {
		t.Run(header, func(t *testing.T) {
			s := hubtest.New(t, hubtest.Env("HUB_PROXY_JWT_JWKS_URL", "https://gateway.example.com/jwks"),
				hubtest.Env("HUB_PROXY_JWT_AUDIENCE", "hub"), hubtest.Env("HUB_PROXY_JWT_HOST", "hub.gateway.example.com"),
				hubtest.Env("HUB_PROXY_JWT_HEADER", header))
			owner := s.SignIn("owner@example.com")
			space, _ := owner.NewSpace("Team")
			id := owner.Machine(space, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "got["+r.Header.Get(header)+"] other["+r.Header.Get("X-Kept")+"]")
			})).MachineID
			for _, path := range []string{"/m/" + id + "/headers", "/m/" + id + "/__pv/1-" + strings.Repeat("0a", 16) + "/x"} {
				r := owner.Do("GET", path, nil, header, "secret-assertion", "X-Kept", "yes")
				if r.Code != 200 || r.String() == "" || strings.Contains(r.String(), "secret-assertion") || !strings.Contains(r.String(), "other[yes]") {
					t.Errorf("%s: %s", path, r)
				}
			}
		})
	}
}
