package mcprecipe

// A draft recipe from a candidate tool: the tool is called once, only when the person (or an agent) asks,
// and the answer's shape tells where the list is and which fields are a thing's id, time, title, author, link and
// thread. Try runs a recipe once and shows the last things as they will be events. Neither keeps anything.

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
)

// Sample is one thing as it will be an event.
type Sample struct {
	ID     string    `json:"id"`
	At     time.Time `json:"at,omitzero"`
	Actor  string    `json:"actor,omitempty"`
	Title  string    `json:"title,omitempty"`
	URL    string    `json:"url,omitempty"`
	Thread string    `json:"thread,omitempty"`
}

// sampleMax things are shown by a check.
const sampleMax = 3

func instanceOf(server string, r Recipe, unmarked bool) (connectors.Instance, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return connectors.Instance{}, err
	}
	allow := "no"
	if unmarked {
		allow = "yes"
	}
	return connectors.Instance{Connector: ID, Config: map[string]string{"server": server, "recipe": string(raw), "unmarked": allow}}, nil
}

// Try runs the recipe once (its guard too) and gives the newest things as they will be events.
func Try(ctx context.Context, server string, r Recipe, unmarked bool) ([]Sample, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	in, err := instanceOf(server, r, unmarked)
	if err != nil {
		return nil, err
	}
	res, err := run(ctx, in, r, "", nil)
	if err != nil {
		return nil, err
	}
	return samples(res.things), nil
}

func samples(things []thing) []Sample {
	out := make([]Sample, 0, len(things))
	for _, t := range things {
		out = append(out, Sample{ID: t.id, At: t.at, Actor: t.actor, Title: clip(t.title, 200), URL: t.url, Thread: t.thread})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if len(out) > sampleMax {
		out = out[:sampleMax]
	}
	return out
}

// DraftInput is what a draft is made of: the candidate tool, its arguments the person filled in (a channel), the
// gateway it is behind.
type DraftInput struct {
	Server      string           `json:"server"`
	Tool        string           `json:"tool"`
	Description string           `json:"description,omitempty"`
	Via         *mcpconn.Gateway `json:"via,omitempty"`
	Args        map[string]any   `json:"args,omitempty"`
	// CursorArg takes the cursor; CursorNumber passes it as a number (the schema says number or integer).
	CursorArg    string `json:"cursorArg,omitempty"`
	CursorNumber bool   `json:"cursorNumber,omitempty"`
	Unmarked     bool   `json:"unmarked,omitempty"`
}

// Draft calls the tool once and guesses the recipe from its answer: the list, the fields, the cursor. The answer must
// be JSON with a list of things that have an id.
func Draft(ctx context.Context, in DraftInput) (Recipe, []Sample, error) {
	r := Recipe{Trigger: mcpconn.TriggerName(in.Tool, in.Description), Tool: in.Tool, Via: in.Via, Args: map[string]any{}}
	for k, v := range in.Args {
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		r.Args[k] = v
	}
	srv, err := server(in.Server)
	if err != nil {
		return r, nil, err
	}
	inst, err := instanceOf(in.Server, Recipe{Trigger: "draft", Tool: in.Tool, ID: "id"}, in.Unmarked)
	if err != nil {
		return r, nil, err
	}
	if err := guard(ctx, inst, srv, in.Tool, in.Via); err != nil {
		return r, nil, err
	}
	args, err := json.Marshal(r.Args)
	if err != nil {
		return r, nil, err
	}
	callTool, callArgs := in.Tool, json.RawMessage(args)
	if in.Via != nil {
		callTool = in.Via.Call
		if callArgs, err = in.Via.CallArgs(in.Tool, args); err != nil {
			return r, nil, err
		}
	}
	raw, err := Service.CallTool(ctx, srv.ID, callTool, callArgs)
	if err != nil {
		return r, nil, connectors.Sayf("%s did not answer the tool %q", srv.Name, in.Tool)
	}
	body, err := answer(raw)
	if err != nil {
		return r, nil, err
	}
	g, ok := Guess(body)
	if !ok {
		return r, nil, connectors.Sayf("The answer of %q has no list of things with an id: it cannot be a trigger as it is", in.Tool)
	}
	r.Items, r.ID, r.At, r.Title, r.Actor, r.URL, r.Thread, r.Cursor = g.Items, g.ID, g.At, g.Title, g.Actor, g.URL, g.Thread, g.Cursor
	if in.CursorArg != "" && r.Cursor != "" {
		if in.CursorNumber {
			r.Args[in.CursorArg] = "{{cursor:number}}"
		} else {
			r.Args[in.CursorArg] = "{{cursor}}"
		}
	}
	if len(r.Args) == 0 {
		r.Args = nil
	}
	list, _ := listOf(pick(body, r.Items))
	var things []thing
	for _, v := range list {
		t := thing{id: str(pick(v, r.ID)), actor: str(pick(v, r.Actor)), title: str(pick(v, r.Title)), url: str(pick(v, r.URL)), thread: str(pick(v, r.Thread))}
		if r.At != "" {
			t.at = when(pick(v, r.At))
		}
		if t.id != "" {
			things = append(things, t)
		}
	}
	return r, samples(things), nil
}

// Guessed are the paths of a draft.
type Guessed struct {
	Items, ID, At, Title, Actor, URL, Thread, Cursor string
}

// listNames are where an answer usually keeps its list, best first.
var listNames = []string{"posts", "items", "data", "results", "messages", "records", "entries", "events", "jobs", "list", "values", "rows", "hits", "issues", "tasks", "files", "nodes", "edges"}

// Guess finds the list of things in an answer and the fields of a thing.
func Guess(body any) (Guessed, bool) {
	items, list, ok := findList(body, "", 0)
	if !ok || len(list) == 0 {
		return Guessed{}, false
	}
	first, _ := list[0].(map[string]any)
	idKeys, timeKeys, titleKeys, actorKeys, urlKeys, threadKeys := mcpconn.GuessKeys()
	g := Guessed{Items: items}
	g.ID = keyOf(first, idKeys, func(v any) bool { return str(v) != "" })
	if g.ID == "" {
		return Guessed{}, false
	}
	g.At = keyOf(first, timeKeys, func(v any) bool { return !when(v).IsZero() })
	g.Title = keyOf(first, titleKeys, func(v any) bool { s, ok := v.(string); return ok && s != "" })
	g.URL = keyOf(first, urlKeys, func(v any) bool { s, ok := v.(string); return ok && strings.HasPrefix(s, "http") })
	g.Thread = keyOf(first, threadKeys, func(v any) bool { return true })
	g.Actor = keyOf(first, actorKeys, func(v any) bool { _, s := v.(string); return s })
	if g.Actor == "" {
		// An author that is an object: its name, login or id.
		for _, k := range actorKeys {
			if m, ok := first[k].(map[string]any); ok {
				for _, sub := range []string{"name", "username", "login", "display_name", "email", "id"} {
					if s, ok := m[sub].(string); ok && s != "" {
						g.Actor = k + "." + sub
						break
					}
				}
			}
			if g.Actor != "" {
				break
			}
		}
	}
	g.Cursor = g.At
	return g, true
}

func keyOf(m map[string]any, keys []string, ok func(any) bool) string {
	for _, k := range keys {
		if v, has := m[k]; has && v != nil && ok(v) {
			return k
		}
	}
	return ""
}

// findList is the first list of objects in an answer (or an object of id → object), preferring the usual names.
func findList(v any, path string, depth int) (string, []any, bool) {
	if depth > 4 {
		return "", nil, false
	}
	if l, ok := v.([]any); ok {
		if len(l) > 0 {
			if _, obj := l[0].(map[string]any); obj {
				return path, l, true
			}
		}
		return "", nil, false
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "", nil, false
	}
	join := func(k string) string {
		if path == "" {
			return k
		}
		return path + "." + k
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := slices.Index(listNames, keys[i]), slices.Index(listNames, keys[j])
		if a < 0 {
			a = len(listNames)
		}
		if b < 0 {
			b = len(listNames)
		}
		if a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		// An object of id → thing (Mattermost's posts): every value an object with an id. Before going inside it.
		if obj, ok := m[k].(map[string]any); ok && len(obj) > 0 && slices.Contains(listNames, k) {
			if l, ok := listOf(obj); ok {
				if first, _ := l[0].(map[string]any); first != nil && first["id"] != nil {
					return join(k), l, true
				}
			}
		}
		if p, l, ok := findList(m[k], join(k), depth+1); ok {
			return p, l, true
		}
	}
	return "", nil, false
}
