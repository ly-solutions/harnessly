package designaccess

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// an editor takes the lease and renews it, the others see who edits, a viewer cannot take it, and another
// editor does not take it over while it runs.
func TestEditLease(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	r.svc.UseView(&versionsView{fakeLibrary: fakeLibrary{heads: map[string]int64{"plan": 5}}})
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
	ask := func(u store.User, m store.Machine, method string) (int, *editLease) {
		req := httptest.NewRequest(method, "/m/"+m.ID+"/api/platform/design/store/plan/lease", nil)
		w := httptest.NewRecorder()
		if !r.svc.ServeMachine(w, req, u, m) {
			t.Fatal("the hub left the lease to the machine")
		}
		var out struct{ Lease *editLease }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Lease
	}
	if code, l := ask(r.ownerU, r.owner, "GET"); code != 200 || l != nil {
		t.Fatalf("nobody edits yet: %d %+v", code, l)
	}
	if code, l := ask(r.mateU, r.mate, "POST"); code != 200 || l == nil || !l.Mine {
		t.Fatalf("the editor did not take the lease: %d %+v", code, l)
	}
	if code, l := ask(r.ownerU, r.owner, "GET"); code != 200 || l == nil || l.Mine || l.Person != normEmail(r.mateU.Email) {
		t.Fatalf("the owner does not see who edits: %d %+v", code, l)
	}
	if code, _ := ask(r.strangeU, r.stranger, "POST"); code != 403 {
		t.Fatalf("a viewer took the lease: %d", code)
	}
	if code, l := ask(r.ownerU, r.owner, "POST"); code != 200 || l == nil || l.Mine {
		t.Fatalf("a running lease was taken over: %d %+v", code, l)
	}
	if code, l := ask(r.mateU, r.mate, "DELETE"); code != 200 || l != nil {
		t.Fatalf("the lease was not given back: %d %+v", code, l)
	}
	if code, l := ask(r.ownerU, r.owner, "POST"); code != 200 || l == nil || !l.Mine {
		t.Fatalf("a free lease was not taken: %d %+v", code, l)
	}
}
