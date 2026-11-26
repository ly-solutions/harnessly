package design

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sitePagesPost(a *API, body string) (int, string) {
	r := httptest.NewRequest(http.MethodPost, "/design/site/pages", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ans := a.SitePages(r)
	raw, _ := json.Marshal(ans.Body)
	return ans.Status, string(raw)
}

// «Макет» → «Страницы»: a page made, duplicated as a variant, renamed and removed by a person who may
// edit; one who may only look is refused.
func TestSitePages(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root, "p1", "Plan", otherV2)
	l := accessLibrary(t, root)
	l.Migrate(false)
	a := pagesAPI(l)
	set := func(role string) {
		t.Helper()
		s := snapshot(map[string]Grant{"p1": {Role: role, Owner: "k@x", OwnerName: "Олег"}}, map[string]string{"p1": "none"})
		s.At = s.At.Add(time.Duration(len(role)) * time.Second)
		if err := l.SetAccess(s); err != nil {
			t.Fatal(err)
		}
	}
	set("view")
	if st, _ := sitePagesPost(a, `{"artifact":"p1","action":"create","title":"Новая"}`); st != 403 {
		t.Errorf("a viewer made a page: %d", st)
	}
	set("edit")
	st, raw := sitePagesPost(a, `{"artifact":"p1","action":"create","title":"Новая"}`)
	if st != 200 || !strings.Contains(raw, `"title":"Новая"`) {
		t.Fatalf("create: %d %s", st, raw)
	}
	var made struct{ Page Page }
	_ = json.Unmarshal([]byte(raw), &made)
	st, raw = sitePagesPost(a, `{"artifact":"p1","action":"duplicate","page":"`+made.Page.File+`"}`)
	if st != 200 || !strings.Contains(raw, `"variantOf":"`+made.Page.ID+`"`) || !strings.Contains(raw, "Новая (") {
		t.Fatalf("duplicate: %d %s", st, raw)
	}
	if st, raw = sitePagesPost(a, `{"artifact":"p1","action":"rename","page":"`+made.Page.File+`","title":"Другая"}`); st != 200 || !strings.Contains(raw, `"title":"Другая"`) {
		t.Fatalf("rename: %d %s", st, raw)
	}
	if st, _ = sitePagesPost(a, `{"artifact":"p1","action":"rename","page":"`+made.Page.File+`","title":" "}`); st != 400 {
		t.Errorf("an empty title taken: %d", st)
	}
	if st, raw = sitePagesPost(a, `{"artifact":"p1","action":"remove","page":"`+made.Page.File+`"}`); st != 200 {
		t.Fatalf("remove: %d %s", st, raw)
	}
	list, _ := l.ListPages("p1")
	for _, p := range list.Pages {
		if p.ID == made.Page.ID {
			t.Error("the removed page is still listed")
		}
	}
}
