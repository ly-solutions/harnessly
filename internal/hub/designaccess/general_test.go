package designaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/link"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// «Общий доступ → Все, у кого есть ссылка» through the route the share dialog calls, against every state of
// the owner's machine, with and without the store: on, a guest opens the project at its address, from the store with
// the machine off; off, the address stops opening; on again, the same address opens again. A new state is a new row.
func TestGeneralAccessMatrix(t *testing.T) {
	cases := []struct {
		machine string
		store   bool
		refused string // "" switches on
	}{
		{"on", true, ""},
		{"off", true, ""},
		{"gvisor", true, ""}, // guests read the store, the runtime's files never
		{"blocked", true, "share_blocked"},
		{"on", false, ""},
		{"off", false, ""},
		{"gvisor", false, "share_unsafe_runtime"},
		{"blocked", false, "share_blocked"},
	}
	for _, c := range cases {
		store := "no store"
		if c.store {
			store = "store"
		}
		t.Run(store+"/"+c.machine, func(t *testing.T) {
			ctx := context.Background()
			r := newSpaceRig(t, false)
			if _, err := r.svc.registerCreated(ctx, r.owner, "site", r.ownerU.Email); err != nil {
				t.Fatal(err)
			}
			r.svc.reg = link.NewRegistry()
			if c.machine != "off" {
				r.svc.reg.Add(&link.Link{MachineID: r.owner.ID})
			}
			r.svc.transport = func(string) http.RoundTripper { return matrixMachine{c.machine} }
			if c.store {
				r.svc.UseView(fakeView{head: 7})
			}
			for _, req := range []hubproto.DesignPublicTokenRequest{{Project: "site", Pin: true}, {Project: "site", Capture: true}} {
				warm(r.svc, r.owner.ID, req)
			}
			r.svc.shareURL = "https://share.example"
			r.svc.seal = newSealer([]byte("0123456789abcdef0123456789abcdef"))

			set := func(level string) (int, map[string]any) {
				req := httptest.NewRequest(http.MethodPut, "/m/"+r.owner.ID+"/api/platform/access/design/site/general",
					strings.NewReader(`{"level":"`+level+`"}`))
				w := httptest.NewRecorder()
				r.svc.ServeMachine(w, req, r.ownerU, r.owner)
				var out map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &out)
				return w.Code, out
			}
			general := func(v map[string]any) (string, string) {
				g, _ := v["general"].(map[string]any)
				level, _ := g["level"].(string)
				url, _ := g["url"].(string)
				return level, url
			}
			guest := func(url string) int {
				w := httptest.NewRecorder()
				r.svc.Public().ServeHTTP(w, httptest.NewRequest("GET", strings.TrimPrefix(url, "https://share.example")+"/index.json", nil))
				return w.Code
			}

			code, v := set(roleView)
			if c.refused != "" {
				if v["code"] != c.refused || code != statusOf[c.refused] {
					t.Fatalf("want %s, got %d %v", c.refused, code, v)
				}
				if msg, _ := v["error"].(string); !russian(msg) {
					t.Fatalf("the owner reads %q", msg)
				}
				return
			}
			level, url := general(v)
			if code != 200 || level != roleView || !strings.HasPrefix(url, "https://share.example/s/") {
				t.Fatalf("on: %d %v", code, v)
			}
			if links, _ := v["links"].([]any); len(links) != 0 {
				t.Fatalf("the general link is listed among the links: %v", links)
			}
			if c.store {
				// The machine is not asked: the store serves the guest.
				r.svc.transport = func(string) http.RoundTripper { return deadTunnel{t} }
				if got := guest(url); got != 200 {
					t.Fatalf("a guest opens the general link: %d", got)
				}
			}

			code, v = set(levelNone)
			if level, _ := general(v); code != 200 || level != levelNone {
				t.Fatalf("off: %d %v", code, v)
			}
			if c.store {
				if got := guest(url); got == 200 {
					t.Fatal("the general link opens after it was switched off")
				}
				r.svc.transport = func(string) http.RoundTripper { return matrixMachine{c.machine} }
			}

			code, v = set(roleComment)
			level, again := general(v)
			if code != 200 || level != roleComment || again != url {
				t.Fatalf("on again: %d %q, want the same address %q", code, again, url)
			}
			if c.store {
				r.svc.transport = func(string) http.RoundTripper { return deadTunnel{t} }
				if got := guest(url); got != 200 {
					t.Fatalf("a guest opens the general link switched on again: %d", got)
				}
			}
		})
	}
}

// The general link opens the project to anyone: an editor switches it only while the owner lets editors share.
func TestGeneralAccessNeedsTheRightToShare(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, false)
	if _, err := r.svc.registerCreated(ctx, r.owner, "site", r.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.write(ctx, r.owner, func(t *txn) error { return t.setEditorsCanShare("site", r.ownerU.Email, false) }); err != nil {
		t.Fatal(err)
	}
	r.svc.reg = link.NewRegistry()
	r.svc.transport = func(string) http.RoundTripper { return matrixMachine{"off"} }
	r.svc.shareURL = "https://share.example"
	req := httptest.NewRequest(http.MethodPut, "/m/"+r.owner.ID+"/api/platform/access/design/site/general", strings.NewReader(`{"level":"view"}`))
	w := httptest.NewRecorder()
	r.svc.ServeMachine(w, req, r.mateU, r.owner)
	if w.Code < 400 {
		t.Fatalf("a person without the right switched general access on: %d %s", w.Code, w.Body.String())
	}
}
