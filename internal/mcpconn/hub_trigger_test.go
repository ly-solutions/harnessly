package mcpconn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func resultText(t *testing.T, res map[string]any) (string, bool) {
	t.Helper()
	content, _ := res["content"].([]map[string]any)
	if len(content) == 0 {
		raw, _ := json.Marshal(res)
		t.Fatalf("no content: %s", raw)
	}
	text, _ := content[0]["text"].(string)
	isErr, _ := res["isError"].(bool)
	return text, isErr
}

func TestTriggerAddAsksThePersonFirst(t *testing.T) {
	s := New(t.TempDir(), newSecrets(), nil)
	if err := s.writeAll(stored{Servers: []Server{{ID: "m1", Name: "Mattermost", Slug: "mattermost", URL: "stdio:mattermost-mcp", Transport: Stdio, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	made := 0
	planned := 0
	answer := "done"
	lastReplace := ""
	var asked ConfirmRequest
	h := &Hub{S: s, ConfirmWait: time.Second,
		Triggers: func(_ context.Context, slug string, recipe json.RawMessage, unmarked bool, name, replace string) (TriggerPlan, error) {
			lastReplace = replace
			planned++
			if slug != "mattermost" || !strings.Contains(string(recipe), "get_channel_posts") {
				t.Fatalf("plan of %s %s", slug, recipe)
			}
			return TriggerPlan{Name: "Mattermost · message.new", Pairs: []Pair{{Name: "Tool", Value: "get_channel_posts"}}, Found: "- p2 · «second»\n",
				Save: func(context.Context) (string, error) { made++; return "Made the trigger", nil }}, nil
		},
		Confirm: func(ctx context.Context, req ConfirmRequest, _ time.Duration) (ConfirmAnswer, error) {
			asked = req
			if answer == "done" {
				res, err := req.Run(ctx)
				return ConfirmAnswer{ID: "p1", State: "done", Result: res}, err
			}
			return ConfirmAnswer{ID: "p1", State: answer}, nil
		}}
	call := func(args string) (string, bool) {
		return resultText(t, h.triggerAdd(context.Background(), json.RawMessage(args)))
	}
	recipe := `"recipe":{"trigger":"message.new","tool":"get_channel_posts","args":{"channel_id":"c1","since":"{{cursor:number}}"},"items":"posts","id":"id"}`

	text, isErr := call(`{"connection":"mattermost",` + recipe + `,"dry_run":true}`)
	if isErr || !strings.Contains(text, "nothing was made") || !strings.Contains(text, "second") || made != 0 {
		t.Fatalf("dry run: %s", text)
	}
	text, isErr = call(`{"connection":"mattermost",` + recipe + `,"why":"new messages of #dev"}`)
	if isErr || text != "Made the trigger" || made != 1 || asked.TitleRu != "Сделать триггер" || asked.Why != "new messages of #dev" {
		t.Fatalf("yes: %s %+v", text, asked)
	}
	answer = "declined"
	text, isErr = call(`{"connection":"mattermost",` + recipe + `}`)
	if !isErr || !strings.Contains(text, "declined") || made != 1 {
		t.Fatalf("no: %s", text)
	}
	answer = "waiting"
	text, _ = call(`{"connection":"mattermost",` + recipe + `}`)
	if !strings.Contains(text, "Call trigger_add again with proposal") {
		t.Fatalf("waiting: %s", text)
	}
	if text, isErr = call(`{"connection":"nope",` + recipe + `}`); !isErr || !strings.Contains(text, "no connection") {
		t.Fatalf("unknown: %s", text)
	}
	// A new version of a trigger: the card says so, the planner gets the trigger it replaces.
	answer = "done"
	if text, isErr = call(`{"connection":"mattermost",` + recipe + `,"replace":"cx1"}`); isErr || lastReplace != "cx1" || asked.TitleRu != "Новая версия триггера" {
		t.Fatalf("replace: %s %s %+v", text, lastReplace, asked)
	}
	// Without a planner the tool is not offered at all.
	if names := toolNames((&Hub{S: s, Confirm: h.Confirm}).tools()); strings.Contains(names, "trigger_add") {
		t.Fatalf("offered without a planner: %s", names)
	}
	if names := toolNames(h.tools()); !strings.Contains(names, "trigger_add") {
		t.Fatalf("not offered: %s", names)
	}
}

func toolNames(tools []map[string]any) string {
	var out []string
	for _, t := range tools {
		out = append(out, t["name"].(string))
	}
	return strings.Join(out, ",")
}
