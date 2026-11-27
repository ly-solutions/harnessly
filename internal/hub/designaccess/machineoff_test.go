package designaccess

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/sitesclient"
)

// offView is the store of a machine that is off: one project at revision 4, which it can make a site of.
type offView struct {
	fakeLibrary
	made int
}

func (v *offView) PublishFiles(_ context.Context, _, _ string, at int64, _ []string) (string, []sitesclient.DeployFile, error) {
	v.made++
	return "Сайт", []sitesclient.DeployFile{{Path: "index.html", Data: []byte("<h1>hi</h1>")}}, nil
}

type fakeSites struct {
	got   []SitePublish
	taken bool
}

func (f *fakeSites) PublishSite(_ context.Context, _ store.Machine, _ store.User, p SitePublish) (PublishedSite, error) {
	if f.taken {
		return PublishedSite{}, ErrSiteNameTaken
	}
	f.got = append(f.got, p)
	return PublishedSite{URL: "https://" + p.Name + ".sites.example", Name: p.Name, Access: p.Access, Version: len(f.got)}, nil
}

func (f *fakeSites) FindSite(context.Context, store.Machine, string) (*PublishedSite, error) {
	if len(f.got) == 0 {
		return nil, nil
	}
	return &PublishedSite{Name: f.got[0].Name}, nil
}

// offRig is the owner's project «plan» with the owner's machine off: the mate may edit it, the stranger may only view it.
func offRig(t *testing.T) (*spaceRig, *offView, map[string]filestore.Record) {
	t.Helper()
	ctx := context.Background()
	r := newSpaceRig(t, true)
	view := &offView{fakeLibrary: fakeLibrary{
		heads: map[string]int64{"plan": 4},
		items: []LibraryItem{{ID: "plan", Title: "План", UpdatedAt: 1000, Kind: "site", Rev: 4}},
	}}
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
	records := map[string]filestore.Record{}
	r.svc.UseVerdicts(func(_ context.Context, _, id string) (filestore.Record, bool, error) {
		rec, ok := records[id]
		return rec, ok, nil
	})
	return r, view, records
}

func TestMachineOffPublishesASiteOnlyWithTheVerdict(t *testing.T) {
	r, view, records := offRig(t)
	sites := &fakeSites{}
	r.svc.UseSites(sites)
	ask := func(u store.User, m store.Machine, method, path, body string) (*httptest.ResponseRecorder, bool) {
		req := httptest.NewRequest(method, "/m/"+m.ID+"/api/platform/design/"+path, strings.NewReader(body))
		w := httptest.NewRecorder()
		return w, r.svc.ServeMachine(w, req, u, m)
	}
	body := `{"project":"plan","name":"plan-site","access":"link"}`

	// No verdict for the newest revision: the project is not published (share_blocked is not passed by).
	w, served := ask(r.ownerU, r.owner, "POST", "publish", body)
	if !served || w.Code == http.StatusOK || !strings.Contains(w.Body.String(), shareUnchecked) || view.made != 0 {
		t.Fatalf("unchecked: %v %d %s", served, w.Code, w.Body.String())
	}
	records["plan"] = filestore.Record{ID: "plan", Share: &filestore.ShareVerdict{Rev: 4, Blocked: true}}
	if w, _ = ask(r.ownerU, r.owner, "POST", "publish", body); w.Code == http.StatusOK || len(sites.got) != 0 {
		t.Fatalf("blocked: %d %s", w.Code, w.Body.String())
	}
	records["plan"] = filestore.Record{ID: "plan", Share: &filestore.ShareVerdict{Rev: 4}}

	// A viewer may not publish.
	if w, _ = ask(r.strangeU, r.owner, "POST", "publish", body); w.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d %s", w.Code, w.Body.String())
	}
	// An editor may not either: publishing is the managers'.
	if w, _ = ask(r.mateU, r.owner, "POST", "publish", body); w.Code != http.StatusForbidden {
		t.Fatalf("editor: %d %s", w.Code, w.Body.String())
	}
	// The owner publishes from the store.
	w, _ = ask(r.ownerU, r.owner, "POST", "publish", body)
	if w.Code != http.StatusOK || w.Header().Get("X-Harnessly-From") != "store" || len(sites.got) != 1 {
		t.Fatalf("owner: %d %s", w.Code, w.Body.String())
	}
	if p := sites.got[0]; p.Name != "plan-site" || p.Access != "link" || p.Origin != "sinopia:plan" || len(p.Files) != 1 {
		t.Fatalf("published: %+v", p)
	}
	// A revision past the newest is not there.
	if w, _ = ask(r.ownerU, r.owner, "POST", "publish", `{"project":"plan","name":"plan-site","revision":"x"}`); w.Code != http.StatusNotFound {
		t.Fatalf("revision: %d %s", w.Code, w.Body.String())
	}
	// The site the project has.
	if w, _ = ask(r.ownerU, r.owner, "GET", "published?project=plan", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "plan-site") {
		t.Fatalf("published: %d %s", w.Code, w.Body.String())
	}
	sites.taken = true
	if w, _ = ask(r.ownerU, r.owner, "POST", "publish", body); w.Code != http.StatusConflict {
		t.Fatalf("taken: %d %s", w.Code, w.Body.String())
	}

}

func TestMachineOffKeepsACommentForTheAgent(t *testing.T) {
	ctx := context.Background()
	r, _, _ := offRig(t)
	type call struct {
		who, path, body string
	}
	var calls []call
	answer := http.StatusOK
	r.svc.UseForward(func(w http.ResponseWriter, req *http.Request, u store.User, _ store.Machine) {
		b, _ := io.ReadAll(req.Body)
		calls = append(calls, call{u.Email, req.URL.Path, string(b)})
		w.WriteHeader(answer)
	})
	post := func(u store.User, m store.Machine, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/m/"+m.ID+"/api/platform/design/site/comments/agent", strings.NewReader(body))
		w := httptest.NewRecorder()
		if !r.svc.ServeMachine(w, req, u, m) {
			t.Fatal("the hub left the comment to the machine")
		}
		return w
	}
	if w := post(r.strangeU, r.owner, `{"artifact":"plan","id":"c1","chat":"tab-1"}`); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer called the agent: %d %s", w.Code, w.Body.String())
	}
	for range 2 { // the same comment twice waits once
		if w := post(r.mateU, r.owner, `{"artifact":"plan","id":"c1","chat":"tab-1"}`); w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"queued":true`) {
			t.Fatalf("queued: %d %s", w.Code, w.Body.String())
		}
	}
	waiting, err := r.svc.waitingComments(ctx, r.owner.ID, "plan")
	if err != nil || len(waiting) != 1 {
		t.Fatalf("waiting: %v %v", waiting, err)
	}

	// The machine is back but the comment's revision has not reached it: the comment waits on.
	answer = http.StatusNotFound
	if left, err := r.svc.handOnAgent(ctx, r.owner.ID); err != nil || left != 1 || len(calls) != 1 {
		t.Fatalf("not yet: %d %v %v", left, err, calls)
	}
	// Now it goes, as the person who wrote it, by the runtime's own route.
	answer = http.StatusOK
	later := r.svc.now().Add(2 * agentQueueClaim)
	r.svc.now = func() time.Time { return later }
	if left, err := r.svc.handOnAgent(ctx, r.owner.ID); err != nil || left != 0 || len(calls) != 2 {
		t.Fatalf("handed on: %d %v %v", left, err, calls)
	}
	c := calls[1]
	if c.who != r.mateU.Email || c.path != "/api/platform/design/site/comments/agent" || !strings.Contains(c.body, `"chat":"tab-1"`) {
		t.Fatalf("call: %+v", c)
	}
	if waiting, _ := r.svc.waitingComments(ctx, r.owner.ID, "plan"); len(waiting) != 0 {
		t.Fatalf("still waiting: %v", waiting)
	}
}
