package designaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/link"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// fakeOwner stands for the owner machine's public design routes: an index of two boards and a raster of each.
type fakeOwner struct {
	online bool
	live   int
}

func (f *fakeOwner) RoundTrip(r *http.Request) (*http.Response, error) {
	if !f.online {
		return nil, errors.New("machine offline")
	}
	f.live++
	h := http.Header{}
	body := ""
	switch {
	case strings.HasSuffix(r.URL.Path, "/__rev"):
		body = `{"rev":"0123456789abcdef0123456789abcdef01234567"}`
		h.Set(revHeader, "0123456789abcdef0123456789abcdef01234567")
	case strings.Contains(r.URL.Path, "/__raster/"):
		var b bytes.Buffer
		_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
		body = b.String()
		h.Set("Content-Type", "image/jpeg")
		h.Set("X-Design-Static-Page", r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		h.Set(revHeader, "0123456789abcdef0123456789abcdef01234567")
		h.Set(pagesHeader, "limited")
	default:
		body = `{"boards":[{"Path":"a.html","Title":"A"},{"Path":"b.html","Title":"B"}]}`
		h.Set(revHeader, "0123456789abcdef0123456789abcdef01234567")
		h.Set(pagesHeader, "limited")
	}
	return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func warm(s *Service, machine string, req hubproto.DesignPublicTokenRequest) {
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%v\x00%t\x00%q", machine, req.Project, req.Rev, req.Pin, req.Capture, req.Pages)
	s.tokens[key] = cachedToken{tok: hubproto.DesignPublicToken{Token: "t", Header: "X-T"}, until: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
}

// GitHub #180: the owner publishes a private snapshot of a project and removes it; only the owner may. HRN-1795: the
// hub always has the store, which a granted reader reads while the owner's machine is off, so the snapshot is no longer
// served through the store's routes.
func TestPrivateSnapshotIsPublishedByTheOwnerOnly(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	r.svc.UseView(fakeView{head: 7})
	if _, err := r.svc.registerCreated(ctx, r.owner, "site", r.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error {
		return tx.grant("site", r.ownerU.Email, r.mateU.Email, roleView, ttl{none: true}, false)
	}); err != nil {
		t.Fatal(err)
	}
	owner := &fakeOwner{online: true}
	r.svc.reg = link.NewRegistry()
	r.svc.transport = func(string) http.RoundTripper { return owner }
	warm(r.svc, r.owner.ID, hubproto.DesignPublicTokenRequest{Project: "site", Pin: true})
	warm(r.svc, r.owner.ID, hubproto.DesignPublicTokenRequest{Project: "site", Rev: "0123456789abcdef0123456789abcdef01234567", Capture: true, Pages: []string{"a.html", "b.html"}})
	warm(r.svc, r.owner.ID, hubproto.DesignPublicTokenRequest{Project: "site", Capture: true})
	warm(r.svc, r.owner.ID, hubproto.DesignPublicTokenRequest{Project: "site"})

	call := func(method, path string, u store.User, m store.Machine, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/m/"+m.ID+"/api/platform/"+path, strings.NewReader(body))
		w := httptest.NewRecorder()
		r.svc.ServeMachine(w, req, u, m)
		return w
	}
	var got struct {
		Snapshot *struct {
			CapturedAt string
			Boards     []struct{ N int }
		}
	}
	read := func() {
		t.Helper()
		got.Snapshot = nil
		w := call(http.MethodGet, "access/design/site/snapshot", r.ownerU, r.owner, "")
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil {
			t.Fatalf("read: %d %s", w.Code, w.Body.String())
		}
	}
	if w := call(http.MethodPut, "access/design/site/snapshot", r.mateU, r.mate, `{"pages":["a.html","b.html"]}`); w.Code == http.StatusOK {
		t.Fatalf("a reader published: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPut, "access/design/site/snapshot", r.ownerU, r.owner, `{"pages":["a.html","b.html"]}`); w.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	if read(); got.Snapshot == nil || len(got.Snapshot.Boards) != 2 || got.Snapshot.CapturedAt == "" {
		t.Fatalf("published: %+v", got.Snapshot)
	}
	// The store's routes do not serve it: a reader reads the project itself.
	if w := call(http.MethodGet, "design/store/site/snapshot", r.mateU, r.mate, ""); w.Code == http.StatusOK {
		t.Fatalf("the store served a snapshot: %s", w.Body.String())
	}
	if w := call(http.MethodDelete, "access/design/site/snapshot", r.ownerU, r.owner, ""); w.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", w.Code, w.Body.String())
	}
	if read(); got.Snapshot != nil {
		t.Fatalf("removed: %+v", got.Snapshot)
	}
	// A snapshot lives only its term.
	if w := call(http.MethodPut, "access/design/site/snapshot", r.ownerU, r.owner, `{"pages":["a.html","b.html"],"ttl":"1h"}`); w.Code != http.StatusOK {
		t.Fatalf("publish again: %d %s", w.Code, w.Body.String())
	}
	r.svc.now = func() time.Time { return time.Date(2026, 11, 9, 11, 0, 0, 0, time.UTC) }
	if read(); got.Snapshot != nil {
		t.Fatalf("expired: %+v", got.Snapshot)
	}
}
