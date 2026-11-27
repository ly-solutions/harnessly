package mcprecipe

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
)

// mattermostTools are the tools of @bryanberger/mattermost-mcp@0.5.0 that matter here, as its tools/list gives them
// (zod → JSON Schema; readOnlyHint = not a write, destructiveHint for deletes).
const mattermostTools = `[
 {"name":"get_channel_posts","description":"Fetch posts from a channel (GET /channels/{id}/posts).",
  "inputSchema":{"type":"object","properties":{"channel_id":{"type":"string","description":"Channel id."},
   "page":{"type":"integer","minimum":0,"default":0},"per_page":{"type":"integer","exclusiveMinimum":0,"default":60},
   "since":{"type":"integer","minimum":0,"description":"Unix timestamp in ms — return posts after this time (mutually exclusive with before/after)."},
   "before":{"type":"string"},"after":{"type":"string"}},"required":["channel_id"]},
  "annotations":{"title":"Get channel posts","readOnlyHint":true,"destructiveHint":false}},
 {"name":"post_message","description":"Post a message to a channel.","inputSchema":{"type":"object","properties":{"channel_id":{"type":"string"},"message":{"type":"string"}},"required":["channel_id","message"]},
  "annotations":{"readOnlyHint":false,"destructiveHint":false}},
 {"name":"delete_post","description":"Delete a post.","inputSchema":{"type":"object","properties":{"post_id":{"type":"string"}}},
  "annotations":{"readOnlyHint":false,"destructiveHint":true}},
 {"name":"get_me","description":"The signed-in user.","inputSchema":{"type":"object","properties":{}},"annotations":{"readOnlyHint":true,"destructiveHint":false}},
 {"name":"list_channels","description":"List channels of a team.","inputSchema":{"type":"object","properties":{"team_id":{"type":"string"},"page":{"type":"integer"}}},
  "annotations":{"readOnlyHint":true,"destructiveHint":false}}
]`

// mmMCP answers like the Mattermost server: get_channel_posts gives a PostList (posts by id, create_at in ms).
type mmMCP struct {
	specs   []mcpconn.ToolSpec
	posts   map[string]map[string]any
	calls   []string
	gateway bool
}

func newMM(t *testing.T, gateway bool) *mmMCP {
	var specs []mcpconn.ToolSpec
	if err := json.Unmarshal([]byte(mattermostTools), &specs); err != nil {
		t.Fatal(err)
	}
	m := &mmMCP{specs: specs, gateway: gateway, posts: map[string]map[string]any{
		"p1": {"id": "p1", "create_at": 1795248000000.0, "user_id": "u1", "channel_id": "c1", "root_id": "", "message": "first",
			"metadata": map[string]any{"embeds": []any{map[string]any{"type": "link", "url": "https://x"}}}},
		"p2": {"id": "p2", "create_at": 1795248600000.0, "user_id": "u2", "channel_id": "c1", "root_id": "p1", "message": "second"},
	}}
	return m
}

func (m *mmMCP) List() ([]mcpconn.Server, error) {
	return []mcpconn.Server{{ID: "s1", Slug: "mattermost", Name: "Mattermost"}}, nil
}

func (m *mmMCP) ToolSpecs(context.Context, string) ([]mcpconn.ToolSpec, error) {
	if m.gateway {
		return []mcpconn.ToolSpec{
			{Name: "list_tools", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
			{Name: "call_tool", InputSchema: json.RawMessage(`{"type":"object","properties":{"server":{"type":"string"},"name":{"type":"string"},"arguments":{"type":"object"}}}`)},
		}, nil
	}
	return m.specs, nil
}

func textResult(v any) json.RawMessage {
	body, _ := json.Marshal(v)
	out, _ := json.Marshal(map[string]any{"content": []map[string]any{{"type": "text", "text": string(body)}}})
	return out
}

func (m *mmMCP) CallTool(_ context.Context, _, tool string, args json.RawMessage) (json.RawMessage, error) {
	m.calls = append(m.calls, tool+" "+string(args))
	if m.gateway {
		switch tool {
		case "list_tools":
			return textResult(map[string]any{"servers": []any{map[string]any{"name": "mattermost", "tools": m.specs}}}), nil
		case "call_tool":
			var a struct {
				Server    string          `json:"server"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(args, &a)
			if a.Server != "mattermost" {
				return textResult(map[string]any{"error": "no server"}), nil
			}
			tool, args = a.Name, a.Arguments
		}
	}
	if tool != "get_channel_posts" {
		return textResult(map[string]any{"error": "not here"}), nil
	}
	var a struct {
		Since *float64 `json:"since"`
	}
	_ = json.Unmarshal(args, &a)
	posts := map[string]any{}
	order := []string{}
	for id, p := range m.posts {
		if a.Since == nil || p["create_at"].(float64) > *a.Since {
			posts[id] = p
			order = append(order, id)
		}
	}
	return textResult(map[string]any{"order": order, "posts": posts, "next_post_id": "", "prev_post_id": ""}), nil
}

func TestMattermostChannelPostsAreATrigger(t *testing.T) {
	mm := newMM(t, false)
	cands := mcpconn.Classify(mm.specs)
	by := map[string]mcpconn.Candidate{}
	for _, c := range cands {
		by[c.Tool] = c
	}
	posts := by["get_channel_posts"]
	if !posts.Fits || cands[0].Tool != "get_channel_posts" || posts.LabelRu != "Новые сообщения в канале" || posts.Label != "New messages in a channel" {
		t.Fatalf("posts: %+v", posts)
	}
	if !by["post_message"].Excluded || !by["delete_post"].Excluded || by["get_me"].Fits {
		t.Fatalf("excluded: %+v %+v %+v", by["post_message"], by["delete_post"], by["get_me"])
	}
	var since mcpconn.CandidateArg
	for _, a := range posts.Args {
		if a.Cursor {
			since = a
		}
	}
	if since.Name != "since" || since.Type != "integer" {
		t.Fatalf("cursor arg: %+v", posts.Args)
	}

	Service = mm
	defer func() { Service = nil }()
	ctx := context.Background()
	r, sample, err := Draft(ctx, DraftInput{Server: "mattermost", Tool: "get_channel_posts", Description: posts.Description,
		Args: map[string]any{"channel_id": "c1"}, CursorArg: "since", CursorNumber: true})
	if err != nil {
		t.Fatal(err)
	}
	want := Recipe{Trigger: "message.new", Tool: "get_channel_posts", Items: "posts", ID: "id", At: "create_at", Actor: "user_id",
		Title: "message", Thread: "root_id", Cursor: "create_at"}
	if r.Trigger != want.Trigger || r.Items != want.Items || r.ID != want.ID || r.At != want.At || r.Actor != want.Actor ||
		r.Title != want.Title || r.Thread != want.Thread || r.Cursor != want.Cursor || r.Args["since"] != "{{cursor:number}}" || r.Args["channel_id"] != "c1" {
		t.Fatalf("draft: %+v", r)
	}
	if len(sample) != 2 || sample[0].ID != "p2" || sample[0].At.UnixMilli() != 1795248600000 || sample[0].Title != "second" {
		t.Fatalf("sample: %+v", sample)
	}

	// The recipe polls: no since on the first poll, then the newest create_at as a number.
	raw, _ := json.Marshal(r)
	memo := map[string]json.RawMessage{}
	in := connectors.Instance{ID: "c9", Connector: ID, Config: map[string]string{"server": "mattermost", "recipe": string(raw)},
		Load: func(v any) error {
			if b, ok := memo["m"]; ok {
				return json.Unmarshal(b, v)
			}
			return nil
		},
		Store: func(v any) error { b, err := json.Marshal(v); memo["m"] = b; return err }}
	if evs, _, err := (conn{}).Poll(ctx, in, ""); err != nil || len(evs) != 0 {
		t.Fatalf("first poll: %v %v", evs, err)
	}
	if strings.Contains(mm.calls[len(mm.calls)-1], "since") {
		t.Fatalf("since on the first poll: %s", mm.calls[len(mm.calls)-1])
	}
	mm.posts["p3"] = map[string]any{"id": "p3", "create_at": 1795249200000.0, "user_id": "u1", "channel_id": "c1", "root_id": "", "message": "third"}
	evs, _, err := (conn{}).Poll(ctx, in, "")
	if err != nil || len(evs) != 1 || evs[0].Title != "third" || evs[0].At.UnixMilli() != 1795249200000 {
		t.Fatalf("second poll: %+v %v", evs, err)
	}
	if !strings.Contains(mm.calls[len(mm.calls)-1], `"since":1795248600000`) {
		t.Fatalf("the cursor is not a number: %s", mm.calls[len(mm.calls)-1])
	}
}

func TestGatewayToolIsTheCandidate(t *testing.T) {
	mm := newMM(t, true)
	gw, ok := mcpconn.DetectGateway(must(mm.ToolSpecs(context.Background(), "s1")))
	if !ok || gw.Call != "call_tool" || gw.NameArg != "name" || gw.ArgsArg != "arguments" || gw.ServerArg != "server" {
		t.Fatalf("gateway: %+v %v", gw, ok)
	}
	raw, _ := mm.CallTool(context.Background(), "s1", "list_tools", nil)
	nested := mcpconn.ParseGatewayTools(raw)
	if len(nested) != 5 || nested[0].Server != "mattermost" {
		t.Fatalf("nested: %+v", nested)
	}
	Service = mm
	defer func() { Service = nil }()
	gw.Server = "mattermost"
	ctx := context.Background()
	r, sample, err := Draft(ctx, DraftInput{Server: "mattermost", Tool: "get_channel_posts", Via: &gw,
		Args: map[string]any{"channel_id": "c1"}, CursorArg: "since", CursorNumber: true})
	if err != nil || r.Via == nil || len(sample) != 2 {
		t.Fatalf("draft through the gateway: %+v %v %v", r, sample, err)
	}
	if got, err := Try(ctx, "mattermost", r, false); err != nil || len(got) != 2 {
		t.Fatalf("try: %+v %v", got, err)
	}
	if !strings.Contains(mm.calls[len(mm.calls)-1], `"name":"get_channel_posts"`) {
		t.Fatalf("not called through call_tool: %s", mm.calls[len(mm.calls)-1])
	}
	// The guard judges the tool behind the gateway: a destructive one is refused although call_tool is unmarked.
	if _, err := Try(ctx, "mattermost", Recipe{Trigger: "x.new", Tool: "delete_post", ID: "id", Via: &gw}, true); err == nil || !strings.Contains(err.Error(), "destructive") {
		t.Fatalf("destructive behind the gateway: %v", err)
	}
}

func TestRecipeFilterAndMode(t *testing.T) {
	mm := newMM(t, false)
	Service = mm
	defer func() { Service = nil }()
	r := Recipe{Trigger: "message.new", Tool: "get_channel_posts", Args: map[string]any{"channel_id": "c1"}, Items: "posts", ID: "id",
		Title: "message", Filter: map[string]string{"message": "SEC"}, Mode: "notify"}
	got, err := Try(context.Background(), "mattermost", r, false)
	if err != nil || len(got) != 1 || got[0].ID != "p2" {
		t.Fatalf("filter: %+v %v", got, err)
	}
	r.Mode = "loud"
	if _, err := Try(context.Background(), "mattermost", r, false); err == nil {
		t.Fatal("a bad mode taken")
	}
}

func TestGuessPaths(t *testing.T) {
	g, ok := Guess(map[string]any{"data": map[string]any{"items": []any{map[string]any{"uuid": "a", "createdAt": "2026-11-21T09:00:00Z",
		"title": "T", "author": map[string]any{"login": "bob"}, "html_url": "https://x/1"}}}})
	if !ok || g.Items != "data.items" || g.ID != "uuid" || g.At != "createdAt" || g.Title != "T" && g.Title != "title" || g.Actor != "author.login" || g.URL != "html_url" {
		t.Fatalf("%+v", g)
	}
	if _, ok := Guess(map[string]any{"ok": true}); ok {
		t.Fatal("no list")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
