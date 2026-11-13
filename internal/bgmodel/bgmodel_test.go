package bgmodel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/extproviders"
)

const testKey = "the-test-key"

type fakeProviders map[string]extproviders.Provider

func (f fakeProviders) Get(id string) (extproviders.Provider, error) {
	p, ok := f[id]
	if !ok {
		return extproviders.Provider{}, errors.New("no provider")
	}
	return p, nil
}

func (f fakeProviders) Key(extproviders.Provider) (string, error) { return testKey, nil }

func TestCompleteSpeaksBothForms(t *testing.T) {
	var got struct {
		path string
		body map[string]any
		hdr  http.Header
	}
	answer := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.path, got.hdr, got.body = r.URL.Path, r.Header, nil
		_ = json.Unmarshal(raw, &got.body)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	c := &Client{}

	answer = `{"choices":[{"message":{"role":"assistant","content":"  Сделано.  "}}]}`
	text, err := c.Complete(context.Background(), Request{BaseURL: srv.URL + "/v1", API: extproviders.APIOpenAI, Key: testKey, Model: "m", System: "s", Prompt: "p", MaxTokens: 50})
	if err != nil || text != "Сделано." {
		t.Fatalf("openai: %q, %v", text, err)
	}
	messages, _ := got.body["messages"].([]any)
	if got.path != "/v1/chat/completions" || got.hdr.Get("Authorization") != "Bearer "+testKey || got.body["model"] != "m" || got.body["max_tokens"] != float64(50) || len(messages) != 2 {
		t.Fatalf("openai request: %s %v", got.path, got.body)
	}

	answer = `{"type":"message","content":[{"type":"thinking","thinking":"…"},{"type":"text","text":"Готово"}]}`
	text, err = c.Complete(context.Background(), Request{BaseURL: srv.URL, API: extproviders.APIAnthropic, Key: testKey, Model: "m", System: "s", Prompt: "p"})
	if err != nil || text != "Готово" {
		t.Fatalf("anthropic: %q, %v", text, err)
	}
	if got.path != "/v1/messages" || got.hdr.Get("x-api-key") != testKey || got.body["system"] != "s" || got.body["max_tokens"] != float64(DefaultMaxTokens) {
		t.Fatalf("anthropic request: %s %v", got.path, got.body)
	}

	answer = `{"choices":[{"message":{"content":""}}]}`
	if _, err := c.Complete(context.Background(), Request{BaseURL: srv.URL, API: extproviders.APIOpenAI, Model: "m", Prompt: "p"}); ReasonOf(err) != ReasonEmpty {
		t.Fatalf("an empty answer: %v", err)
	}
}

func TestCompleteSaysWhyItFailed(t *testing.T) {
	status := http.StatusUnauthorized
	slow := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slow {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"` + testKey + `"}`))
	}))
	defer srv.Close()
	c := &Client{}
	req := Request{BaseURL: srv.URL, API: extproviders.APIOpenAI, Key: testKey, Model: "m", Prompt: "p"}
	for code, want := range map[int]string{401: ReasonUnauthorized, 403: ReasonUnauthorized, 429: ReasonRateLimited, 500: ReasonRejected, 404: ReasonRejected} {
		status = code
		_, err := c.Complete(context.Background(), req)
		if ReasonOf(err) != want || strings.Contains(err.Error(), testKey) {
			t.Errorf("%d: %v", code, err)
		}
	}
	slow = true
	req.Timeout = 50 * time.Millisecond
	if _, err := c.Complete(context.Background(), req); ReasonOf(err) != ReasonTimeout {
		t.Fatalf("a slow provider: %v", err)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	req.BaseURL = dead.URL
	dead.Close()
	if _, err := c.Complete(context.Background(), req); ReasonOf(err) != ReasonUnreachable {
		t.Fatalf("a dead provider: %v", err)
	}
}

func TestRunnerAsNowCallsNothing(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Починил\n вход и  добавил тест"}}]}`))
	}))
	defer srv.Close()
	var logged []string
	r := &Runner{
		Store:     &Store{Path: filepath.Join(t.TempDir(), "v2", "background-tasks.json")},
		Providers: fakeProviders{"local": {ID: "local", BaseURL: srv.URL, API: extproviders.APIOpenAI}},
		Client:    &Client{},
		Log: func(feature string, c Choice, err error) {
			logged = append(logged, feature+" "+c.String()+" "+ReasonOf(err))
		},
	}
	ctx := context.Background()

	// «As now»: nothing is picked, nothing is called.
	if r.Picked(FeatureNotifySummary) {
		t.Fatal("picked before anyone picked")
	}
	if _, used, err := r.NotifySummary(ctx, "long answer"); used || err != nil || calls != 0 {
		t.Fatalf("as now: used=%v err=%v calls=%d", used, err, calls)
	}
	var none *Runner
	if none.Picked(FeatureNotifySummary) {
		t.Fatal("a runner that is not there picks nothing")
	}

	// A choice is checked before it is kept.
	if _, err := r.Pick("nonsense", &Choice{Provider: "local", Model: "m"}); !errors.Is(err, ErrUnknownFeature) {
		t.Fatalf("an unknown feature: %v", err)
	}
	if _, err := r.Pick(FeatureNotifySummary, &Choice{Provider: "gone", Model: "m"}); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("an unknown provider: %v", err)
	}
	if _, err := r.Pick(FeatureNotifySummary, &Choice{Provider: "local", Model: " "}); !errors.Is(err, ErrBadModel) {
		t.Fatalf("no model: %v", err)
	}
	if before, err := r.Pick(FeatureNotifySummary, &Choice{Provider: "local", Model: "small"}); err != nil || before != nil {
		t.Fatalf("picked: %v, %v", before, err)
	}
	summary, used, err := r.NotifySummary(ctx, "long answer")
	if !used || err != nil || summary != "Починил вход и добавил тест" || calls != 1 {
		t.Fatalf("picked: %q used=%v err=%v calls=%d", summary, used, err, calls)
	}
	if last := r.Last()[FeatureNotifySummary]; !last.OK || last.At == 0 {
		t.Fatalf("the last call: %+v", last)
	}

	// The provider fails: the caller is told, and the failure is on record.
	srv.Close()
	if _, used, err := r.NotifySummary(ctx, "long answer"); !used || ReasonOf(err) != ReasonUnreachable {
		t.Fatalf("a dead provider: used=%v err=%v", used, err)
	}
	if last := r.Last()[FeatureNotifySummary]; last.OK || last.Reason != ReasonUnreachable || last.Message == "" {
		t.Fatalf("the last call: %+v", last)
	}
	if len(logged) != 1 || logged[0] != "notify-summary local/small unreachable" {
		t.Fatalf("the record: %v", logged)
	}

	// The provider is removed: the feature is back to «as now».
	_, _ = r.Pick(FeatureMemory, &Choice{Provider: "local", Model: "big"})
	if got := r.Forget("local"); strings.Join(got, ",") != "memory,notify-summary" || r.Picked(FeatureNotifySummary) {
		t.Fatalf("forgotten: %v", got)
	}
}

func TestChoice(t *testing.T) {
	c, ok := ParseChoice("openrouter/meta/llama-4")
	if !ok || c.Provider != "openrouter" || c.Model != "meta/llama-4" || c.String() != "openrouter/meta/llama-4" {
		t.Fatalf("%+v %v", c, ok)
	}
	for _, bad := range []string{"", "a", "a/", "/b"} {
		if _, ok := ParseChoice(bad); ok {
			t.Errorf("%q was read", bad)
		}
	}
}
