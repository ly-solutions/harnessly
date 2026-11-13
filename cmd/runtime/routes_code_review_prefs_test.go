package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestReviewPrefsKeepInstructionsBotsAndRefuseTheOversized(t *testing.T) {
	s := &reviewPrefs{path: t.TempDir() + "/v2/code-review.json", now: time.Now}
	text, own := " общие ", "свои"
	hide := true
	bots := []string{" release robot ", "", "release robot", "ci"}
	patch := reviewPrefsPatch{Instructions: &text, HideBots: &hide, Bots: &bots}
	patch.Repo = &struct {
		Key  string `json:"key"`
		Text string `json:"text"`
	}{Key: " GitLab.example/Team/App ", Text: own}
	f, ok, err := s.change(patch)
	if err != nil || !ok {
		t.Fatalf("change: %v %v", ok, err)
	}
	if f.Instructions != "общие" || f.Repos["gitlab.example/team/app"] != "свои" || !f.HideBots ||
		strings.Join(f.Bots, ",") != "release robot,ci" {
		t.Errorf("kept %+v", f)
	}
	// It is on the disk, not in the store.
	if again := (&reviewPrefs{path: s.path, now: time.Now}).load(); again.Instructions != "общие" || len(again.Repos) != 1 {
		t.Errorf("read again %+v", again)
	}
	// An empty text takes a repository's instructions off; a part that is not one to keep changes nothing.
	patch = reviewPrefsPatch{Repo: patch.Repo}
	patch.Repo.Text = ""
	if f, _, _ = s.change(patch); len(f.Repos) != 0 {
		t.Errorf("repos %+v", f.Repos)
	}
	long := strings.Repeat("я", reviewTextMax+1)
	off := false
	if _, ok, _ := s.change(reviewPrefsPatch{Instructions: &long, HideBots: &off}); ok {
		t.Error("an oversized text was kept")
	}
	if f := s.load(); f.Instructions != "общие" || !f.HideBots {
		t.Errorf("a refused patch changed %+v", f)
	}
}

func TestReviewPrefsViewedMarksHoldVersionsAndDropTheOldestScopes(t *testing.T) {
	at := time.UnixMilli(1000)
	s := &reviewPrefs{path: t.TempDir() + "/code-review.json", now: func() time.Time { at = at.Add(time.Second); return at }}
	if _, err := s.mark("team/app", "a.go", "v1"); err != nil {
		t.Fatal(err)
	}
	marks, _ := s.mark("team/app", "b.go", "v2")
	if len(marks) != 2 || marks["a.go"] != "v1" {
		t.Errorf("marks %+v", marks)
	}
	if marks, _ = s.mark("team/app", "a.go", ""); len(marks) != 1 || marks["b.go"] != "v2" {
		t.Errorf("after unmark %+v", marks)
	}
	if got := s.marks("other"); len(got) != 0 {
		t.Errorf("another scope %+v", got)
	}
	for i := 0; i < reviewScopesMax; i++ {
		if _, err := s.mark("scope-"+string(rune('a'+i%26))+strings.Repeat("x", i), "f", "v"); err != nil {
			t.Fatal(err)
		}
	}
	if f := s.load(); len(f.Viewed) != reviewScopesMax || f.Viewed["team/app"] != nil {
		t.Errorf("scopes %d, the oldest kept: %v", len(f.Viewed), f.Viewed["team/app"] != nil)
	}
}

func TestReviewPrefsRoutes(t *testing.T) {
	d := deps{
		DataDir:    t.TempDir(),
		SessionOK:  func(*http.Request) bool { return true },
		APIRefusal: func(*http.Request) (int, any, http.Header, bool) { return 0, nil, nil, false },
	}
	call := func(method, target, body, site string) (int, map[string]any) {
		t.Helper()
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Sec-Fetch-Site", site)
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/json")
		}
		for _, rt := range codeReviewRoutes(d) {
			if rt.Match(r) {
				status, v, _ := rt.Compute(r)
				raw, _ := json.Marshal(v)
				out := map[string]any{}
				_ = json.Unmarshal(raw, &out)
				return status, out
			}
		}
		t.Fatalf("no route for %s %s", method, target)
		return 0, nil
	}
	prefs, viewed := platformAPI+"/code-review/prefs", platformAPI+"/code-review/viewed"
	if status, out := call(http.MethodGet, prefs, "", "same-origin"); status != http.StatusOK || out["hideBots"] != false || out["repos"] == nil {
		t.Errorf("defaults: %d %+v", status, out)
	}
	if status, out := call(http.MethodPost, prefs, `{"hideBots":true,"repo":{"key":"h/p","text":"x"}}`, "same-origin"); status != http.StatusOK || out["hideBots"] != true {
		t.Errorf("change: %d %+v", status, out)
	}
	if status, _ := call(http.MethodPost, prefs, `{"repo":{"key":"","text":"x"}}`, "same-origin"); status != http.StatusBadRequest {
		t.Errorf("a repository without a key: %d", status)
	}
	if status, _ := call(http.MethodPost, prefs, `{"hideBots":false}`, "cross-site"); status != http.StatusForbidden {
		t.Errorf("a cross-site change: %d", status)
	}
	if status, out := call(http.MethodPost, viewed, `{"scope":"team/app","path":"a.go","version":"v1"}`, "same-origin"); status != http.StatusOK || out["marks"].(map[string]any)["a.go"] != "v1" {
		t.Errorf("mark: %d %+v", status, out)
	}
	if status, out := call(http.MethodGet, viewed+"?"+url.Values{"scope": {"team/app"}}.Encode(), "", "same-origin"); status != http.StatusOK || len(out["marks"].(map[string]any)) != 1 {
		t.Errorf("marks: %d %+v", status, out)
	}
	if status, _ := call(http.MethodGet, viewed, "", "same-origin"); status != http.StatusBadRequest {
		t.Errorf("marks without a scope: %d", status)
	}
}
