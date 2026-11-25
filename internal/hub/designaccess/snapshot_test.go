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

// GitHub #180: with the Store off, A publishes a private snapshot, goes offline, and B (granted) reads only that.
func TestPrivateSnapshotServesAGrantedReaderWhileTheOwnerIsOffline(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, false)
	for _, u := range []store.User{r.ownerU, r.mateU, r.strangeU} {
		if err := r.db.S().CreateIdentity(ctx, &store.Identity{UserID: u.ID, Provider: "test", Subject: u.ID, Email: u.Email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.svc.registerCreated(ctx, r.owner, "site", r.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	grant := func() {
		t.Helper()
		if err := r.svc.write(ctx, r.owner, func(tx *txn) error {
			if err := tx.setTeam("site", r.ownerU.Email, levelNone); err != nil {
				return err
			}
			return tx.grant("site", r.ownerU.Email, r.mateU.Email, roleView, ttl{none: true}, false)
		}); err != nil {
			t.Fatal(err)
		}
	}
	grant()
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
	// Only the owner publishes; a reader (even with a grant) cannot.
	if w := call(http.MethodPut, "access/design/site/snapshot", r.mateU, r.mate, `{"pages":["a.html","b.html"]}`); w.Code == http.StatusOK {
		t.Fatalf("a reader published: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "design/store/site/snapshot", r.mateU, r.mate, ""); w.Code == http.StatusOK {
		t.Fatalf("a snapshot before anyone published: %s", w.Body.String())
	}
	if w := call(http.MethodPut, "access/design/site/snapshot", r.ownerU, r.owner, `{"pages":["a.html","b.html"]}`); w.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	// Publishing made no grant: the stranger still has nothing.
	if w := call(http.MethodGet, "design/store/site/snapshot", r.strangeU, r.stranger, ""); w.Code != http.StatusNotFound {
		t.Fatalf("an ungranted account: %d %s", w.Code, w.Body.String())
	}
	// While the owner machine answers, a reader is sent to the live project, never to the snapshot.
	if w := call(http.MethodGet, "design/store/site/snapshot", r.mateU, r.mate, ""); w.Code == http.StatusOK || !strings.Contains(w.Body.String(), `"live"`) {
		t.Fatalf("snapshot while live: %d %s", w.Code, w.Body.String())
	}
	owner.online = false
	w := call(http.MethodGet, "design/store/site/snapshot", r.mateU, r.mate, "")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("offline read: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	var meta struct {
		CapturedAt string
		Boards     []struct {
			N     int
			Title string
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil || len(meta.Boards) != 2 || meta.CapturedAt == "" {
		t.Fatalf("meta: %v %s", err, w.Body.String())
	}
	img := call(http.MethodGet, "design/store/site/snapshot/img/1", r.mateU, r.mate, "")
	if img.Code != http.StatusOK || img.Header().Get("Content-Type") != "image/jpeg" || img.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("image: %d %v", img.Code, img.Header())
	}
	if _, err := jpeg.Decode(img.Body); err != nil {
		t.Fatal(err)
	}
	// Signed-out readers and ungranted accounts get nothing, the old image address included.
	if w := call(http.MethodGet, "design/store/site/snapshot/img/1", r.strangeU, r.stranger, ""); w.Code != http.StatusNotFound {
		t.Fatalf("ungranted image: %d", w.Code)
	}
	if w := call(http.MethodGet, "design/store/site/snapshot/img/9", r.mateU, r.mate, ""); w.Code != http.StatusNotFound {
		t.Fatalf("a board that is not in the snapshot: %d", w.Code)
	}
	// Revoking the reader stops the old image address at once, and removing the snapshot does not revoke anyone.
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error { return tx.revoke("site", r.ownerU.Email, r.mateU.Email, false) }); err != nil {
		t.Fatal(err)
	}
	if w := call(http.MethodGet, "design/store/site/snapshot/img/1", r.mateU, r.mate, ""); w.Code == http.StatusOK {
		t.Fatalf("an image after the grant was revoked: %d", w.Code)
	}
	grant()
	if w := call(http.MethodGet, "design/store/site/snapshot/img/1", r.mateU, r.mate, ""); w.Code != http.StatusOK {
		t.Fatalf("a regranted reader: %d", w.Code)
	}
	if w := call(http.MethodDelete, "access/design/site/snapshot", r.ownerU, r.owner, ""); w.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "design/store/site/snapshot/img/1", r.mateU, r.mate, ""); w.Code != http.StatusNotFound {
		t.Fatalf("an image after the snapshot was removed: %d", w.Code)
	}
	if w := call(http.MethodGet, "design/store/site/snapshot", r.mateU, r.mate, ""); !strings.Contains(w.Body.String(), "no_snapshot") {
		t.Fatalf("no snapshot, owner offline: %d %s", w.Code, w.Body.String())
	}
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error {
		_, err := tx.upsertMember("site", r.ownerU.Email, r.mateU.Email, roleView, nil, "")
		return err
	}); err != nil {
		t.Logf("grant still there: %v", err)
	}
	// A snapshot lives only its term.
	owner.online = true
	if w := call(http.MethodPut, "access/design/site/snapshot", r.ownerU, r.owner, `{"pages":["a.html","b.html"],"ttl":"1h"}`); w.Code != http.StatusOK {
		t.Fatalf("publish again: %d %s", w.Code, w.Body.String())
	}
	owner.online = false
	r.svc.now = func() time.Time { return time.Date(2026, 11, 9, 11, 0, 0, 0, time.UTC) }
	if w := call(http.MethodGet, "design/store/site/snapshot/img/1", r.mateU, r.mate, ""); w.Code != http.StatusNotFound {
		t.Fatalf("an image after the snapshot expired: %d", w.Code)
	}
}
