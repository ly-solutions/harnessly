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
	"sync"
	"testing"
	"unicode"

	"github.com/ly-solutions/harnessly/internal/filestore"
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
	case "old":
		// A runtime on the line that does not know the probe yet: every question of the hub is a 404.
		return answer(http.StatusNotFound, nil, "")
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

// matrixView is the store holding the project's two boards at revision 7.
type matrixView struct{ fakeView }

func (v matrixView) Serve(w http.ResponseWriter, _ *http.Request, _, _ string, at int64, segs, _ []string, _ bool) error {
	if at != 7 || segs != nil {
		return errors.New("the snapshot read the store at another revision or a file")
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"title":"Site","boards":[{"Path":"a.html","Title":"A","W":390,"H":844},{"Path":"b.html","Title":"B"}]}`)
	return nil
}

// matrixDrawer is the hub's render worker: it draws every page it is asked for.
type matrixDrawer struct {
	mu   sync.Mutex
	jobs []RenderJob
}

func (d *matrixDrawer) WantThumb(context.Context, RenderJob) error { return nil }

func (d *matrixDrawer) DrawPage(_ context.Context, j RenderJob) ([]byte, error) {
	d.mu.Lock()
	d.jobs = append(d.jobs, j)
	d.mu.Unlock()
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	return b.Bytes(), nil
}

func (d *matrixDrawer) check(t *testing.T, machine string) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.jobs) != 2 {
		t.Fatalf("the worker drew %d pages, want both boards", len(d.jobs))
	}
	a := d.jobs[0]
	if a.Board != "a.html" || a.Rev != 7 || a.BoardW != 390 || a.BoardH != 844 || a.MachineID != machine || a.OwnerID == "" || a.Prefix == "" {
		t.Fatalf("page job %+v", a)
	}
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
		// verdict is what the project's record holds of the apps' checks,
		// "ok" and "blocked" at the store's newest revision, "stale" at an older one, "none" no verdict.
		verdict string
	}{
		{"live", "on", true, ok(""), ""},
		{"live", "off", true, ok(""), ""},
		{"live", "gvisor", true, ok(""), ""}, // #187: guests read the store, the runtime's files never
		{"live", "blocked", true, no("share_blocked"), ""},
		{"live", "on", false, ok(""), ""},
		{"live", "off", false, ok(""), ""},
		{"live", "gvisor", false, no("share_unsafe_runtime"), ""},
		{"live", "blocked", false, no("share_blocked"), ""},

		{"pinned", "on", true, ok(storeHead), ""},
		{"pinned", "off", true, ok(storeHead), ""},
		{"pinned", "gvisor", true, ok(storeHead), ""},
		{"pinned", "blocked", true, no("share_blocked"), ""},
		{"pinned", "on", false, ok(matrixRev), ""},
		{"pinned", "off", false, no("unavailable"), ""},
		{"pinned", "gvisor", false, no("share_unsafe_runtime"), ""},
		{"pinned", "blocked", false, no("share_blocked"), ""},

		// with the stored verdicts a machine that is off no longer lets the link through unchecked.
		{"live", "off", true, ok(""), "ok"},
		{"live", "off", true, no("share_blocked"), "blocked"},
		{"live", "off", true, no("share_unchecked"), "stale"},
		{"live", "off", true, no("share_unchecked"), "none"},
		{"live", "on", true, ok(""), "blocked"}, // a machine that answers is asked, not its old verdict
		{"live", "blocked", true, no("share_blocked"), "ok"},
		{"live", "off", false, ok(""), "none"}, // without the store there is no revision a verdict could speak for
		{"live", "old", true, ok(""), "none"},  // a machine on the line is not answered for by a verdict it never wrote
		{"pinned", "off", true, ok(storeHead), "ok"},
		{"pinned", "off", true, no("share_blocked"), "blocked"},
		{"pinned", "off", true, no("share_unchecked"), "stale"},
		{"pinned", "off", true, no("share_unchecked"), "none"},
		{"pinned", "on", true, ok(storeHead), "stale"},

		// A snapshot of the pages is drawn by the owner's browser from git, with or without the store.
		{"snapshot", "on", true, ok(matrixRev), ""},
		{"snapshot", "off", true, no("unavailable"), ""},
		{"snapshot", "gvisor", true, no("share_unsafe_runtime"), ""}, // #190
		{"snapshot", "blocked", true, no("share_blocked"), ""},
		{"snapshot", "on", false, ok(matrixRev), ""},
		{"snapshot", "off", false, no("unavailable"), ""},
		{"snapshot", "gvisor", false, no("share_unsafe_runtime"), ""},
		{"snapshot", "blocked", false, no("share_blocked"), ""},

		// with the hub's render worker the snapshot is drawn from the store at its newest revision, and the
		// owner's machine takes no part: it may be off or refuse guests' reads; its checks still stop the link.
		{"snapshot-worker", "on", true, ok(storeHead), ""},
		{"snapshot-worker", "off", true, ok(storeHead), "ok"},
		{"snapshot-worker", "off", true, no("share_blocked"), "blocked"},
		{"snapshot-worker", "off", true, no("share_unchecked"), "none"},
		{"snapshot-worker", "gvisor", true, ok(storeHead), ""},
		{"snapshot-worker", "blocked", true, no("share_blocked"), ""},
		// A worker without the store has nothing to draw from: the machine draws, as before.
		{"snapshot-worker", "on", false, ok(matrixRev), ""},
		{"snapshot-worker", "off", false, no("unavailable"), ""},
	}
	for _, c := range cases {
		store := "no store"
		if c.store {
			store = "store"
		}
		name := c.way + "/" + store + "/" + c.machine
		if c.verdict != "" {
			name += "/verdict-" + c.verdict
		}
		t.Run(name, func(t *testing.T) {
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
			var drawer *matrixDrawer
			if c.way == "snapshot-worker" {
				drawer = &matrixDrawer{}
				r.svc.UseRenderQueue(drawer)
			}
			if c.store {
				r.svc.UseView(matrixView{fakeView{head: 7}})
			}
			if c.verdict != "" {
				r.svc.UseVerdicts(func(_ context.Context, spaceID, id string) (filestore.Record, bool, error) {
					if spaceID != r.owner.SpaceID || id != "site" {
						t.Errorf("record %s/%s", spaceID, id)
					}
					rec := filestore.Record{ID: id, Title: "Site", Kind: "project"}
					switch c.verdict {
					case "ok":
						rec.Share = &filestore.ShareVerdict{Rev: 7}
					case "blocked":
						rec.Share = &filestore.ShareVerdict{Rev: 7, Blocked: true}
					case "stale":
						rec.Share = &filestore.ShareVerdict{Rev: 6}
					}
					return rec, true, nil
				})
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
			case "snapshot", "snapshot-worker":
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
				if drawer != nil && c.store {
					drawer.check(t, r.owner.ID)
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
			if strings.HasPrefix(c.way, "snapshot") && c.machine != "blocked" && c.verdict == "" && !strings.Contains(got.Error, "Снимок") {
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
