package designaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/ly-solutions/harnessly/internal/hub/link"
	"github.com/ly-solutions/harnessly/internal/hubproto"
	"github.com/ly-solutions/harnessly/internal/storerev"
)

// matrixMachine is the owner's machine as the hub meets it while making a link, in each of the states it can be in.
type matrixMachine struct{ state string }

const matrixRev = "0123456789abcdef0123456789abcdef01234567"

func (f matrixMachine) RoundTrip(r *http.Request) (*http.Response, error) {
	answer := func(status int, h http.Header, body string) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}
	switch f.state {
	case "off":
		return nil, errors.New("machine offline")
	case "gvisor":
		// The runtime's refuseUnsafeFS (SHR-204): every guest path, its render included.
		return answer(http.StatusServiceUnavailable, http.Header{refusedHeader: {"1"}}, "")
	case "blocked":
		// The runtime's refuseShare: the apps' checks found what keeps the project from being shared.
		return answer(http.StatusConflict, http.Header{"Content-Type": {"application/json"}},
			`{"code":"share_blocked","error":"secrets in a page","blocked":true,"results":[]}`)
	}
	h := http.Header{revHeader: {matrixRev}, pagesHeader: {"limited"}}
	switch {
	case strings.HasSuffix(r.URL.Path, "/__rev"):
		return answer(200, h, `{"rev":"`+matrixRev+`"}`)
	case strings.HasSuffix(r.URL.Path, "/__share"):
		return answer(200, h, `{}`)
	case strings.Contains(r.URL.Path, "/__raster/"):
		var b bytes.Buffer
		_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
		h.Set("Content-Type", "image/jpeg")
		h.Set("X-Design-Static-Page", r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		return answer(200, h, b.String())
	}
	return answer(200, h, `{"boards":[{"Path":"a.html","Title":"A"},{"Path":"b.html","Title":"B"}]}`)
}

// HRN-1787, GitHub #187, #190. Every way of making a link, on a hub with and without the store, against every state
// of the owner's machine, through the route the share dialog calls. A new way or a new state is a new row here, not a
// new patch: every refusal must come with its code, a status the dialog shows, and a Russian text that says what to do.
func TestLinkMakingMatrix(t *testing.T) {
	storeHead := storerev.ID(7)
	type want struct {
		code string // "" is a link made
		rev  string // the version a made link is pinned to; "" for a live link
	}
	ok := func(rev string) want { return want{rev: rev} }
	no := func(code string) want { return want{code: code} }
	cases := []struct {
		way, machine string
		store        bool
		want         want
	}{
		{"live", "on", true, ok("")},
		{"live", "off", true, ok("")},
		{"live", "gvisor", true, ok("")}, // #187: guests read the store, the runtime's files never
		{"live", "blocked", true, no("share_blocked")},
		{"live", "on", false, ok("")},
		{"live", "off", false, ok("")},
		{"live", "gvisor", false, no("share_unsafe_runtime")},
		{"live", "blocked", false, no("share_blocked")},

		{"pinned", "on", true, ok(storeHead)},
		{"pinned", "off", true, ok(storeHead)},
		{"pinned", "gvisor", true, ok(storeHead)},
		{"pinned", "blocked", true, no("share_blocked")},
		{"pinned", "on", false, ok(matrixRev)},
		{"pinned", "off", false, no("unavailable")},
		{"pinned", "gvisor", false, no("share_unsafe_runtime")},
		{"pinned", "blocked", false, no("share_blocked")},

		// A snapshot of the pages is drawn by the owner's browser from git, with or without the store.
		{"snapshot", "on", true, ok(matrixRev)},
		{"snapshot", "off", true, no("unavailable")},
		{"snapshot", "gvisor", true, no("share_unsafe_runtime")}, // #190
		{"snapshot", "blocked", true, no("share_blocked")},
		{"snapshot", "on", false, ok(matrixRev)},
		{"snapshot", "off", false, no("unavailable")},
		{"snapshot", "gvisor", false, no("share_unsafe_runtime")},
		{"snapshot", "blocked", false, no("share_blocked")},
	}
	for _, c := range cases {
		store := "no store"
		if c.store {
			store = "store"
		}
		t.Run(c.way+"/"+store+"/"+c.machine, func(t *testing.T) {
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
			for _, req := range []hubproto.DesignPublicTokenRequest{
				{Project: "site", Pin: true},
				{Project: "site", Capture: true},
				{Project: "site", Rev: matrixRev, Capture: true},
				{Project: "site", Rev: matrixRev, Capture: true, Pages: []string{"a.html", "b.html"}},
			} {
				warm(r.svc, r.owner.ID, req)
			}
			body := `{"forWhom":"Клиент","rights":"view","ttl":"7d"}`
			switch c.way {
			case "pinned":
				body = `{"forWhom":"Клиент","rights":"view","ttl":"7d","version":"pinned"}`
				r.svc.shareURL = "https://share.example"
			case "live":
				r.svc.shareURL = "https://share.example"
			case "snapshot":
				r.svc.staticGuests = true
			}
			req := httptest.NewRequest(http.MethodPost, "/m/"+r.owner.ID+"/api/platform/access/design/site/links", strings.NewReader(body))
			w := httptest.NewRecorder()
			r.svc.ServeMachine(w, req, r.ownerU, r.owner)

			var got struct {
				Code  string
				Error string
				Link  struct{ ID string }
			}
			_ = json.Unmarshal(w.Body.Bytes(), &got)
			if c.want.code == "" {
				if w.Code != http.StatusCreated {
					t.Fatalf("want a link, got %d %s", w.Code, w.Body.String())
				}
				tx, err := r.svc.read(ctx, r.owner)
				if err != nil {
					t.Fatal(err)
				}
				rows, err := tx.shareRows("site")
				if err != nil || len(rows) != 1 || rows[0].ID != got.Link.ID {
					t.Fatalf("links %v %v", rows, err)
				}
				if rows[0].Rev != c.want.rev {
					t.Fatalf("pinned to %q, want %q", rows[0].Rev, c.want.rev)
				}
				return
			}
			if got.Code != c.want.code {
				t.Fatalf("want %s, got %d %s", c.want.code, w.Code, w.Body.String())
			}
			if w.Code != statusOf[c.want.code] || w.Code < 400 {
				t.Fatalf("%s answered %d, want %d", c.want.code, w.Code, statusOf[c.want.code])
			}
			if !russian(got.Error) {
				t.Fatalf("the owner reads %q", got.Error)
			}
			if c.way == "snapshot" && c.machine != "blocked" && !strings.Contains(got.Error, "Снимок") {
				t.Fatalf("a snapshot refused without saying the snapshot is what cannot be made: %q", got.Error)
			}
		})
	}
}

// russian is a text the owner can act on: mostly Cyrillic, at least a sentence long.
func russian(s string) bool {
	cyr, letters := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.Is(unicode.Cyrillic, r) {
				cyr++
			}
		}
	}
	return letters >= 20 && cyr*2 > letters
}
