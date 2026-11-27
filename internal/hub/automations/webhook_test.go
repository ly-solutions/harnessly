package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/config"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/link"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// hookLinks is a machine link that records what was passed and answers a status.
type hookLinks struct {
	op     string
	params map[string]any
	status int
	away   bool
}

func (f *hookLinks) Get(string) *link.Link { return nil }

func (f *hookLinks) Do(_ context.Context, _ string, op string, params any, _ time.Duration) (json.RawMessage, error) {
	if f.away {
		return nil, httpx.ErrMachineOffline
	}
	f.op = op
	raw, _ := json.Marshal(params)
	_ = json.Unmarshal(raw, &f.params)
	return json.Marshal(map[string]int{"status": f.status})
}

// hookMachine is a machine of the hub with this id.
func hookMachine(t *testing.T, s *Service, id string) {
	t.Helper()
	ctx := context.Background()
	u := &store.User{Email: id + "@example.com"}
	if err := s.db.S().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	sp := &store.Space{Slug: id, Name: id, CreatedBy: u.ID}
	if err := s.db.S().CreateSpace(ctx, sp); err != nil {
		t.Fatal(err)
	}
	if err := s.db.S().CreateMachine(ctx, &store.Machine{ID: id, SpaceID: sp.ID, Name: id, KeyPub: id, CreatedBy: u.ID}); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookPassesToTheMachineOnly(t *testing.T) {
	s := newSvc(t, noon())
	hookMachine(t, s, "mach1")
	hookMachine(t, s, "mach2")
	links := &hookLinks{status: http.StatusAccepted}
	s.links = links
	webhookLimits = &limiter{m: map[string]*bucket{}}
	mux := http.NewServeMux()
	s.Mount(mux)
	post := func(path, body string, h map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/platform/automations/hook/m/"+path, strings.NewReader(body))
		for k, v := range h {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	rec := post("mach1/cx0123456789ab", `{"a":1}`, map[string]string{"X-Hub-Signature-256": "sha256=ab", "Cookie": "session=1"})
	if rec.Code != http.StatusAccepted || links.op != "connectors.webhook" || links.params["connection"] != "cx0123456789ab" {
		t.Fatalf("passed: %d %+v", rec.Code, links)
	}
	header, _ := links.params["header"].(map[string]any)
	if header["X-Hub-Signature-256"] != "sha256=ab" || header["Cookie"] != nil {
		t.Fatalf("headers: %+v", header)
	}
	// The machine's 401 is the sender's 401, without words.
	links.status = http.StatusUnauthorized
	rec = post("mach1/cx0123456789ab", `{}`, nil)
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "signature") {
		t.Fatalf("unauthorized: %d %s", rec.Code, rec.Body)
	}
	// A malformed address or secret never reaches a machine.
	links.op = ""
	if rec = post("mach1/../x", `{}`, nil); rec.Code == http.StatusAccepted || links.op != "" {
		t.Fatalf("bad address: %d", rec.Code)
	}
	if rec = post("mach1/cx0123456789ab/short", `{}`, nil); rec.Code != http.StatusUnauthorized || links.op != "" {
		t.Fatalf("bad secret: %d", rec.Code)
	}
	// Too big.
	if rec = post("mach1/cx0123456789ab", strings.Repeat("x", webhookMaxBody+1), nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too big: %d", rec.Code)
	}
	// The machine away: 503, so the sender tries again.
	links.away = true
	if rec = post("mach1/cx0123456789ab", `{}`, nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("away: %d", rec.Code)
	}
	// A flood of one address is cut.
	links.away = false
	limited := false
	for range webhookBurst + 5 {
		if post("mach2/cx0123456789ab", `{}`, nil).Code == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatal("no rate limit")
	}
}

// The gateway's assertion on its own address is not a webhook's proof and does not go to the machine.
func TestWebhookKeepsTheGatewaysAssertion(t *testing.T) {
	s := newSvc(t, noon())
	s.cfg.ProxyJWT = config.ProxyJWT{JWKSURL: "https://gw.example.com/jwks", Host: "gw.example.com", Header: "Authorization"}
	links := &hookLinks{status: http.StatusAccepted}
	s.links = links
	webhookLimits = &limiter{m: map[string]*bucket{}}
	hookMachine(t, s, "mach1")
	mux := http.NewServeMux()
	s.Mount(mux)
	for host, want := range map[string]any{"gw.example.com": nil, "hub.example.com": "Bearer x"} {
		req := httptest.NewRequest(http.MethodPost, "/api/platform/automations/hook/m/mach1/cx0123456789ab", strings.NewReader(`{}`))
		req.Host = host
		req.Header.Set("Authorization", "Bearer x")
		mux.ServeHTTP(httptest.NewRecorder(), req)
		if header, _ := links.params["header"].(map[string]any); header["Authorization"] != want {
			t.Errorf("%s: passed %v, want %v", host, header["Authorization"], want)
		}
	}
}

// The route needs no account, so what a caller names costs nothing until it is a machine, one address is limited
// whatever it names, and the memory of the limiter has a ceiling.
func TestWebhookIsBounded(t *testing.T) {
	s := newSvc(t, noon())
	links := &hookLinks{status: http.StatusAccepted}
	s.links = links
	webhookLimits, webhookSenders = &limiter{m: map[string]*bucket{}}, &limiter{m: map[string]*bucket{}}
	hookMachine(t, s, "mach1")
	mux := http.NewServeMux()
	s.Mount(mux)
	post := func(ip, path string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/platform/automations/hook/m/"+path, strings.NewReader(`{}`))
		req.RemoteAddr = ip + ":1000"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post("192.0.2.1", "ghost/cx0123456789ab"); code != http.StatusServiceUnavailable || links.op != "" || len(webhookLimits.m) != 0 {
		t.Fatalf("an unknown machine: %d, passed %q, %d addresses kept", code, links.op, len(webhookLimits.m))
	}
	limited := false
	for i := range webhookSenderBurst + 5 {
		if post("192.0.2.2", fmt.Sprintf("mach1/cx%012x", i)) == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatal("one sender is not limited across addresses")
	}
	if code := post("192.0.2.3", "mach1/cx0123456789ab"); code != http.StatusAccepted {
		t.Fatalf("another sender: %d", code)
	}
	l := &limiter{m: map[string]*bucket{}}
	for i := range limiterKeys + 100 {
		l.allow(fmt.Sprint(i), noon(), webhookRate, webhookBurst)
	}
	if len(l.m) > limiterKeys {
		t.Fatalf("the limiter holds %d keys, more than %d", len(l.m), limiterKeys)
	}
}
