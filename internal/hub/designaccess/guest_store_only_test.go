package designaccess

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// what a guest is told when the store cannot serve it. The machine's tunnel fails the test on any use.

type fakeView struct {
	head    int64
	headErr error
}

func (f fakeView) Head(context.Context, string, string) (int64, error) { return f.head, f.headErr }
func (f fakeView) Wait(context.Context, string, string, int64) (int64, error) {
	return f.head, nil
}
func (f fakeView) Serve(w http.ResponseWriter, _ *http.Request, _, _ string, _ int64, _, _ []string, _ bool) error {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"title":"Shop","boards":[]}`))
	return nil
}
func (f fakeView) Write(context.Context, string, string, string, int64, []byte, string) (Written, error) {
	return Written{}, errors.New("not used")
}
func (f fakeView) Edit(context.Context, http.ResponseWriter, *http.Request, string, string, string, string) error {
	return errors.New("not used")
}

type deadTunnel struct{ t *testing.T }

func (d deadTunnel) RoundTrip(r *http.Request) (*http.Response, error) {
	d.t.Errorf("a guest read reached the machine: %s", r.URL)
	return nil, errors.New("machine is not to be asked")
}

func TestGuestIsServedFromTheStoreOnly(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		view    Viewer
		rev     string
		status  int
		message string
	}{
		{"the project is not in the store", fakeView{head: 0}, "", 404, "ещё не опубликован"},
		{"the store did not answer", fakeView{headErr: errors.New("db down")}, "", 503, "Попробуйте позже"},
		{"no store at all", nil, "", 503, "недоступен"},
		{"a link pinned to a version of git", fakeView{head: 3}, strings.Repeat("ab", 20), 410, "устарела"},
		{"the store serves it", fakeView{head: 3}, "", 200, "Shop"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newSpaceRig(t, true)
			r.svc.transport = func(string) http.RoundTripper { return deadTunnel{t} }
			if c.view != nil {
				r.svc.UseView(c.view)
			}
			if done, err := r.svc.registerCreated(ctx, r.owner, "shop", r.ownerU.Email); err != nil || !done {
				t.Fatalf("registerCreated: %v %v", done, err)
			}
			in := shareInput{Rights: roleView, Version: "latest", TTL: ttl{none: true}}
			if c.rev != "" {
				in.Version, in.Rev = "pinned", c.rev
			}
			var token string
			if err := r.svc.write(ctx, r.owner, func(t *txn) error {
				var err error
				_, token, err = t.createShare("shop", r.ownerU.Email, in)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{"/index.json", "/version", "/f/Main.dc.html"} {
				if c.status == 200 && p != "/index.json" {
					continue
				}
				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "/s/"+token+p, nil)
				r.svc.Public().ServeHTTP(w, req)
				if w.Code != c.status || !strings.Contains(w.Body.String(), c.message) {
					t.Fatalf("%s: %d %s", p, w.Code, w.Body.String())
				}
			}
		})
	}
}
