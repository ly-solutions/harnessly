package designaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A live guest link is only as good as its maker: it stops when they leave the project's space or their account is
// switched off, though the project still names them its owner.
func TestGuestLinkEndsWithItsMakersMembership(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]struct {
		maker  string
		end    func(r *spaceRig) error
		status int
	}{
		"a member's link": {status: http.StatusOK},
		"removed from the space": {status: http.StatusGone, end: func(r *spaceRig) error {
			return r.db.S().RemoveMember(ctx, r.owner.SpaceID, r.mateU.ID)
		}},
		"the account switched off": {status: http.StatusGone, end: func(r *spaceRig) error {
			_, err := r.db.S().DisableUser(ctx, r.mateU.ID, time.Now())
			return err
		}},
		// A link brought over from the gateway by a person who has not signed in to the hub yet keeps working.
		"a maker the hub has no account of": {maker: "elsewhere@example.com", status: http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			r := newSpaceRig(t, true)
			r.svc.UseView(fakeView{head: 3})
			maker := c.maker
			if maker == "" {
				maker = r.mateU.Email
			}
			if done, err := r.svc.registerCreated(ctx, r.mate, "shop", maker); err != nil || !done {
				t.Fatalf("registerCreated: %v %v", done, err)
			}
			var token string
			if err := r.svc.write(ctx, r.mate, func(t *txn) error {
				var err error
				_, token, err = t.createShare("shop", maker, shareInput{Rights: roleView, Version: "latest", TTL: ttl{none: true}})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if c.end != nil {
				if err := c.end(r); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			r.svc.Public().ServeHTTP(w, httptest.NewRequest("GET", "/s/"+token+"/index.json", nil))
			if w.Code != c.status {
				t.Fatalf("%d %s, want %d", w.Code, w.Body.String(), c.status)
			}
		})
	}
}
