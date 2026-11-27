package designaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/storerev"
)

// versionsView is the store's versions for the routes: three versions of a project whose newest revision is 5.
type versionsView struct {
	fakeLibrary
	served   []string
	restored []int64
	base     int64
	stale    bool
}

func (v *versionsView) Serve(w http.ResponseWriter, _ *http.Request, _, _ string, at int64, segs, _ []string, pinned bool) error {
	v.served = append(v.served, strings.Join(segs, "/")+"@"+storerev.ID(at)[36:]+map[bool]string{true: "!", false: ""}[pinned])
	_, _ = w.Write([]byte("page"))
	return nil
}

func (v *versionsView) Versions(context.Context, string, string, int) ([]Version, error) {
	return []Version{
		{Rev: 5, At: 3000, Author: "owner@example.com", Files: []VersionFile{{Path: "project/Main.dc.html", Kind: "changed"}, {Path: "project/a.css", Kind: "changed"}}},
		{Rev: 3, At: 2000, Machine: "vm-owner", Files: []VersionFile{{Path: "project/Main.dc.html", Kind: "removed"}}},
	}, nil
}

func (v *versionsView) Compare(_ context.Context, _, _ string, at int64) ([]Change, int, error) {
	return []Change{{Path: "Main.dc.html", Kind: "changed", Board: "Main"}}, 2, nil
}

func (v *versionsView) Restore(_ context.Context, _, _ string, at, base int64, _ string) (Written, error) {
	v.restored = append(v.restored, at)
	v.base = base
	if v.stale || base != 0 && base != 5 {
		return Written{Rev: 6}, ErrConflict
	}
	return Written{Rev: 6, Head: 6}, nil
}

func TestStoreVersionsRoutes(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	view := &versionsView{fakeLibrary: fakeLibrary{heads: map[string]int64{"plan": 5}}}
	r.svc.UseView(view)
	if _, err := r.svc.registerCreated(ctx, r.owner, "plan", r.ownerU.Email); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.write(ctx, r.owner, func(tx *txn) error {
		if err := tx.setTeam("plan", r.ownerU.Email, levelNone); err != nil {
			return err
		}
		if _, err := tx.upsertMember("plan", r.ownerU.Email, r.mateU.Email, roleEdit, nil, ""); err != nil {
			return err
		}
		_, err := tx.upsertMember("plan", r.ownerU.Email, r.strangeU.Email, roleView, nil, "")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ask := func(u store.User, m store.Machine, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/m/"+m.ID+"/api/platform/design/store/plan/"+path, strings.NewReader(body))
		w := httptest.NewRecorder()
		if !r.svc.ServeMachine(w, req, u, m) {
			t.Fatalf("the hub left %s to the machine", path)
		}
		return w
	}
	pin3 := storerev.ID(3)
	for _, c := range []struct {
		name   string
		who    store.User
		on     store.Machine
		method string
		path   string
		body   string
		status int
		has    string
	}{
		{"the list, by a viewer", r.strangeU, r.stranger, "GET", "versions", "", 200, `"subject":"project/Main.dc.html +1"`},
		{"a revision's page is read pinned", r.strangeU, r.stranger, "GET", "v/" + pin3 + "/f/Main.dc.html", "", 200, "page"},
		{"a revision's index", r.strangeU, r.stranger, "GET", "v/3/index.json", "", 200, "page"},
		{"what changed since", r.strangeU, r.stranger, "GET", "v/" + pin3 + "/diff", "", 200, `"board":"Main"`},
		{"a revision past the head", r.strangeU, r.stranger, "GET", "v/9/index.json", "", 404, ""},
		{"restore by an editor", r.mateU, r.mate, "POST", "restore", `{"version":"` + pin3 + `"}`, 200, `"rev":6`},
		{"restore by a viewer", r.strangeU, r.stranger, "POST", "restore", `{"version":"` + pin3 + `"}`, 403, ""},
		{"restore on a newer head than the editor saw", r.mateU, r.mate, "POST", "restore", `{"version":"3","base":4}`, 409, `"code":"conflict"`},
		{"restore by GET", r.mateU, r.mate, "GET", "restore", "", 400, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := ask(c.who, c.on, c.method, c.path, c.body)
			if w.Code != c.status || !strings.Contains(w.Body.String(), c.has) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	var list struct {
		Points []struct {
			ID   string `json:"id"`
			Auto bool   `json:"auto"`
		} `json:"points"`
	}
	if err := json.Unmarshal(ask(r.mateU, r.mate, "GET", "versions", "").Body.Bytes(), &list); err != nil || len(list.Points) != 2 || list.Points[0].ID != storerev.ID(5) || !list.Points[1].Auto {
		t.Fatalf("list: %+v %v", list, err)
	}
	if len(view.served) != 2 || view.served[0] != "Main.dc.html@0003!" || view.served[1] != "@0003!" {
		t.Fatalf("pinned reads: %v", view.served)
	}
	// A write between the read and the commit: the store says no, and the editor is told to read again.
	view.stale = true
	if w := ask(r.mateU, r.mate, "POST", "restore", `{"version":"`+pin3+`"}`); w.Code != http.StatusConflict {
		t.Fatalf("race: %d %s", w.Code, w.Body.String())
	}
}
