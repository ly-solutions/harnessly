package mcprecipe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/designkit/kitadd"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
	"github.com/ly-solutions/harnessly/internal/scriptbox"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == kitadd.SandboxHelperArg {
		if err := kitadd.RunSandboxHelper(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	ScriptOptions = scriptbox.Options{Helper: []string{os.Args[0], kitadd.SandboxHelperArg}}
	os.Exit(m.Run())
}

// srcMCP answers list_jobs with jobs (by pages of two when paged) and an API's GET with the same jobs.
type srcMCP struct {
	jobs  []map[string]any
	paged bool
	api   bool
	calls []string
	gets  []mcpconn.APIRequest
}

func (f *srcMCP) List() ([]mcpconn.Server, error) {
	t := mcpconn.HTTP
	if f.api {
		t = mcpconn.API
	}
	return []mcpconn.Server{{ID: "s1", Slug: "gen", Name: "Gen", Transport: t}}, nil
}

func (f *srcMCP) ToolSpecs(context.Context, string) ([]mcpconn.ToolSpec, error) {
	return []mcpconn.ToolSpec{{Name: "list_jobs", Annotations: &mcpconn.ToolAnnotations{ReadOnlyHint: yes()}}}, nil
}

func (f *srcMCP) page(token string) map[string]any {
	if !f.paged {
		return map[string]any{"jobs": f.jobs}
	}
	i := 0
	if token != "" {
		_, _ = fmt.Sscan(token, &i)
	}
	end := min(i+2, len(f.jobs))
	out := map[string]any{"jobs": f.jobs[i:end]}
	if end < len(f.jobs) {
		out["next"] = fmt.Sprint(end)
	}
	return out
}

func (f *srcMCP) CallTool(_ context.Context, _, _ string, args json.RawMessage) (json.RawMessage, error) {
	f.calls = append(f.calls, string(args))
	var a struct {
		Page string `json:"page"`
	}
	_ = json.Unmarshal(args, &a)
	body, _ := json.Marshal(f.page(a.Page))
	return json.Marshal(map[string]any{"content": []map[string]any{{"type": "text", "text": string(body)}}})
}

func (f *srcMCP) CallAPI(_ context.Context, _ string, in mcpconn.APIRequest) (mcpconn.APIAnswer, error) {
	f.gets = append(f.gets, in)
	body, _ := json.Marshal(f.page(in.Query["page"]))
	return mcpconn.APIAnswer{Status: 200, ContentType: "application/json", Body: body}, nil
}

func recipeInstance(t *testing.T, r Recipe) (connectors.Instance, *[]string) {
	t.Helper()
	raw, _ := json.Marshal(r)
	memo := map[string]json.RawMessage{}
	var warns []string
	return connectors.Instance{ID: "c1", Connector: ID, Config: map[string]string{"server": "gen", "recipe": string(raw)},
		Load: func(v any) error {
			if b, ok := memo["m"]; ok {
				return json.Unmarshal(b, v)
			}
			return nil
		},
		Store: func(v any) error { b, err := json.Marshal(v); memo["m"] = b; return err },
		Warn:  func(msg string) { warns = append(warns, msg) },
	}, &warns
}

func poll(t *testing.T, in connectors.Instance) []connectors.Event {
	t.Helper()
	evs, _, err := (conn{}).Poll(context.Background(), in, "")
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestWatchGivesItemChanged(t *testing.T) {
	f := &srcMCP{jobs: []map[string]any{{"id": "j1", "status": "running", "prompt": "a long prompt that is not a status at all, it has many words"}}}
	Service = f
	defer func() { Service = nil }()
	in, _ := recipeInstance(t, Recipe{Trigger: "generation.done", Tool: "list_jobs", Items: "jobs", ID: "id", Title: "prompt",
		Watch: []string{"status", "prompt"}, Facts: []string{"status"}})
	if evs := poll(t, in); len(evs) != 0 {
		t.Fatalf("first: %+v", evs)
	}
	f.jobs[0]["status"] = "done"
	evs := poll(t, in)
	if len(evs) != 1 || evs[0].Type != EventChanged || evs[0].Facts["status"] != "running → done" || evs[0].Facts["changed"] != "status" {
		t.Fatalf("changed: %+v", evs)
	}
	if evs := poll(t, in); len(evs) != 0 {
		t.Fatalf("again: %+v", evs)
	}
	// The long field is kept as its hash only: «was» is not known, the memo has no value.
	f.jobs[0]["prompt"] = "another long prompt that is not a status at all, with many words"
	evs = poll(t, in)
	if len(evs) != 1 || evs[0].Facts["prompt"] != "… → another long prompt that is not a status at all, with many words" {
		t.Fatalf("long: %+v", evs)
	}
	var m memo
	_ = in.Load(&m)
	raw, _ := json.Marshal(m.Watched)
	if strings.Contains(string(raw), "long prompt") || !strings.Contains(string(raw), "=done") {
		t.Fatalf("memo keeps values: %s", raw)
	}
}

func TestFactsGoIntoEvents(t *testing.T) {
	f := &srcMCP{jobs: []map[string]any{{"id": "j1", "status": "done", "model": "kling", "cost": 3.5}}}
	Service = f
	defer func() { Service = nil }()
	in, _ := recipeInstance(t, Recipe{Trigger: "generation.done", Tool: "list_jobs", Items: "jobs", ID: "id", Facts: []string{"status", "model", "cost", "missing"}})
	poll(t, in)
	f.jobs = append(f.jobs, map[string]any{"id": "j2", "status": "done", "model": strings.Repeat("x", 500), "cost": 7.0})
	evs := poll(t, in)
	if len(evs) != 1 || evs[0].Facts["status"] != "done" || evs[0].Facts["cost"] != "7" || len([]rune(evs[0].Facts["model"])) > factMax+1 || evs[0].Facts["missing"] != "" {
		t.Fatalf("%+v", evs)
	}
	nine := Recipe{Trigger: "a.b", Tool: "t", ID: "id", Facts: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}}
	if err := nine.check(); err == nil {
		t.Fatal("nine facts taken")
	}
}

func TestPagesUpToTheCeiling(t *testing.T) {
	f := &srcMCP{paged: true}
	for i := range 14 {
		f.jobs = append(f.jobs, map[string]any{"id": fmt.Sprint("j", i)})
	}
	Service = f
	defer func() { Service = nil }()
	in, warns := recipeInstance(t, Recipe{Trigger: "job.new", Tool: "list_jobs", Items: "jobs", ID: "id", Next: &Next{Path: "next", Arg: "page"}})
	old := connectors.Trace
	connectors.Trace = connectors.NewTracer(50)
	connectors.Trace.SetEnabled(func() bool { return true })
	defer func() { connectors.Trace = old }()
	poll(t, in)
	if len(f.calls) != pageMax || !strings.Contains(f.calls[1], `"page":"2"`) {
		t.Fatalf("pages: %v", f.calls)
	}
	if len(*warns) == 0 || !strings.Contains((*warns)[len(*warns)-1], "pages") {
		t.Fatalf("no warning: %v", *warns)
	}
	found := false
	for _, e := range connectors.Trace.Recent("c1", 0) {
		if e.Level == connectors.TraceWarn && strings.Contains(e.Message, "pages read") {
			found = true
		}
	}
	if !found {
		t.Fatal("no trace of the ceiling")
	}
	// Fewer pages than the ceiling: the warning is taken back.
	f.jobs = f.jobs[:4]
	f.calls = nil
	poll(t, in)
	if len(f.calls) != 2 || (*warns)[len(*warns)-1] != "" {
		t.Fatalf("under the ceiling: %v %v", f.calls, *warns)
	}
}

func TestTimeCursorComparedAsTime(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2026-11-21T10:00:00+03:00", "2026-11-21T08:00:00.5Z", false}, // 07:00Z is before 08:00:00.5Z
		{"2026-11-21T08:00:00.123456Z", "2026-11-21T08:00:00Z", true},
		{"2026-11-21T09:00:00Z", "2026-11-21T08:59:59.999+00:00", true},
		{"10", "9", true},
		{"b", "a", true},
	}
	for _, c := range cases {
		if got := later(c.a, c.b); got != c.want {
			t.Errorf("later(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestAPISourceGETOnly(t *testing.T) {
	f := &srcMCP{api: true, jobs: []map[string]any{{"id": "j1", "at": "2026-11-21T09:00:00Z"}}}
	Service = f
	defer func() { Service = nil }()
	in, _ := recipeInstance(t, Recipe{Trigger: "job.new", Path: "/v1/jobs", Query: map[string]string{"since": "{{cursor}}", "status": "done"},
		Items: "jobs", ID: "id", At: "at", Cursor: "at"})
	poll(t, in)
	if len(f.gets) != 1 || f.gets[0].Method != "GET" || f.gets[0].Query["since"] != "" || f.gets[0].Query["status"] != "done" {
		t.Fatalf("first GET: %+v", f.gets)
	}
	f.jobs = append(f.jobs, map[string]any{"id": "j2", "at": "2026-11-21T10:00:00Z"})
	evs := poll(t, in)
	if len(evs) != 1 || f.gets[1].Query["since"] != "2026-11-21T09:00:00Z" || f.gets[1].Path != "/v1/jobs" {
		t.Fatalf("second: %+v %+v", evs, f.gets)
	}
	// A path on a connection that is not an API is refused.
	f.api = false
	if _, _, err := (conn{}).Poll(context.Background(), in, ""); err == nil || !strings.Contains(err.Error(), "not an API") {
		t.Fatalf("not an API: %v", err)
	}
	if err := (Recipe{Trigger: "a.b", Tool: "t", Path: "/x", ID: "id"}).check(); err == nil {
		t.Fatal("tool and path taken")
	}
}

func TestScriptReshapesWithItsMemo(t *testing.T) {
	if !scriptbox.Available() {
		t.Skip("no node or no Landlock here")
	}
	f := &srcMCP{jobs: []map[string]any{{"id": "j1", "v": 1.0}}}
	Service = f
	defer func() { Service = nil }()
	script := `(answer, memo) => ({ items: answer.jobs.map(j => ({ key: "k" + j.id, title: "v=" + j.v })), memo: { polls: ((memo && memo.polls) || 0) + 1 } })`
	in, _ := recipeInstance(t, Recipe{Trigger: "job.new", Tool: "list_jobs", ID: "key", Title: "title", Script: script, Version: 2})
	poll(t, in)
	f.jobs = append(f.jobs, map[string]any{"id": "j2", "v": 2.0})
	evs := poll(t, in)
	if len(evs) != 1 || evs[0].ID != "job.new:kj2" || evs[0].Title != "v=2" {
		t.Fatalf("%+v", evs)
	}
	var m memo
	_ = in.Load(&m)
	if string(m.Script) != `{"polls":2}` {
		t.Fatalf("script memo: %s", m.Script)
	}
	for name, bad := range map[string]string{
		"network": `() => { fetch("http://127.0.0.1:1"); return { items: [] } }`,
		"loop":    `() => { for (;;) {} }`,
	} {
		in, _ := recipeInstance(t, Recipe{Trigger: "job.new", Tool: "list_jobs", ID: "key", Script: bad})
		if _, _, err := (conn{}).Poll(context.Background(), in, ""); err == nil || !strings.Contains(err.Error(), "script") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
