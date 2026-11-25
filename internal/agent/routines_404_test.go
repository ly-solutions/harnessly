package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// The hub's own 404 (a routine not found) is its answer; only a bare 404 means the hub does not serve the route.
func TestSignedCallKeepsTheHubsOwn404(t *testing.T) {
	for _, tc := range []struct {
		body      string
		notServed bool
		inText    string
	}{
		{`{"error":"not_found","message":"Правило не найдено"}`, false, "Правило не найдено"},
		{`404 page not found`, true, ""},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(tc.body))
		}))
		seed := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SeedSize))
		h := &hub{machine: &MachineFile{MachineID: "m1", DialURL: "ws" + strings.TrimPrefix(srv.URL, "http") + "/agent/v1", Ed25519: seed}}
		var out map[string]any
		err := h.signedCall(context.Background(), hubproto.SignRoutines, hubproto.RoutinesPath, map[string]string{}, &out)
		srv.Close()
		if errors.Is(err, errPeersNotServed) != tc.notServed {
			t.Errorf("%q: notServed=%v err=%v", tc.body, errors.Is(err, errPeersNotServed), err)
		}
		if tc.inText != "" && (err == nil || !strings.Contains(err.Error(), tc.inText)) {
			t.Errorf("%q: error %v", tc.body, err)
		}
	}
}
