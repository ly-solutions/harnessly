package mcprecipe

// One run of a recipe: the source asked — an MCP tool (directly or through a gateway) or an API connection's
// GET — page after page while the answer names a next page (at most pageMax), each answer reshaped by the recipe's
// script when it has one, and the things mapped: id, time, author, title, link, thread, the cursor, the facts the
// recipe names and the fields it watches.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
	"github.com/ly-solutions/harnessly/internal/scriptbox"
)

// pageMax pages are read in one poll; more is said in the trace and the connection's warning, never dropped silently.
const pageMax = 5

// APICaller is what an API source needs of the runtime's connections (mcpconn.Service has it).
type APICaller interface {
	CallAPI(ctx context.Context, id string, in mcpconn.APIRequest) (mcpconn.APIAnswer, error)
}

// ScriptOptions run the recipes' scripts; the runtime leaves the defaults, a test names its sandbox helper.
var ScriptOptions scriptbox.Options

// thing is one item of the answer, mapped by the recipe.
type thing struct {
	id, actor, title, url, thread, cursor string
	at                                    time.Time
	facts                                 map[string]string
	watch                                 map[string]string
}

// runResult is a run: the things, whether pages were left unread, the script's memo after it.
type runResult struct {
	things []thing
	capped bool
	memo   json.RawMessage
}

// run asks the source (every page of it) and maps the answers. memo is the script's memo of the last poll.
func run(ctx context.Context, in connectors.Instance, r Recipe, cursor string, memo json.RawMessage) (runResult, error) {
	var res runResult
	srv, err := server(in.Config["server"])
	if err != nil {
		return res, err
	}
	if r.Path != "" {
		if srv.Transport != mcpconn.API {
			return res, connectors.Sayf("%s is not an API connection: a recipe with a path asks an API", srv.Name)
		}
	} else if err := guard(ctx, in, srv, r.Tool, r.Via); err != nil {
		trace(in, connectors.TraceWarn, "guard refused "+r.Tool+": "+connectors.Say(err), map[string]any{"server": srv.Name, "tool": r.Tool, "reason": connectors.Say(err)})
		return res, err
	}
	res.memo = memo
	page := ""
	for n := 1; ; n++ {
		body, call, err := fetch(ctx, in, srv, r, cursor, page)
		if err != nil {
			return res, err
		}
		list, err := r.listOfAnswer(ctx, in, body, &res.memo)
		if err != nil {
			return res, err
		}
		call["items"] = len(list)
		what := r.Tool
		if r.Path != "" {
			what = "GET " + r.Path
		}
		trace(in, connectors.TraceInfo, fmt.Sprintf("%s %s: %d items in %dms", srv.Name, what, len(list), call["ms"]), call)
		for _, v := range list {
			if t, ok := r.mapThing(v); ok {
				res.things = append(res.things, t)
			}
		}
		if r.Next == nil {
			break
		}
		next := str(pick(body, r.Next.Path))
		if next == "" || next == page {
			break
		}
		if n >= pageMax {
			res.capped = true
			trace(in, connectors.TraceWarn, fmt.Sprintf("%d pages read, more are there: the rest waits for the next poll", pageMax),
				map[string]any{"pages": pageMax, "next": next})
			break
		}
		page = next
	}
	return res, nil
}

// fetch asks the source once: the tool with the arguments, or the API's GET with the path and query, the cursor and
// the page put in.
func fetch(ctx context.Context, in connectors.Instance, srv mcpconn.Server, r Recipe, cursor, page string) (any, map[string]any, error) {
	began := time.Now()
	if r.Path != "" {
		api, ok := Service.(APICaller)
		if !ok {
			return nil, nil, connectors.Sayf("API connections are not available on this machine")
		}
		query := map[string]string{}
		for k, v := range r.Query {
			if v == "{{cursor}}" && cursor == "" {
				continue
			}
			query[k] = strings.ReplaceAll(v, "{{cursor}}", cursor)
		}
		if page != "" {
			query[r.Next.Arg] = page
		}
		path := strings.ReplaceAll(r.Path, "{{cursor}}", url.PathEscape(cursor))
		// Only GET: a recipe reads, it never changes anything at the service.
		a, err := api.CallAPI(ctx, srv.ID, mcpconn.APIRequest{Method: "GET", Path: path, Query: query})
		took := time.Since(began)
		call := map[string]any{"get": path, "query": query, "ms": took.Milliseconds(), "status": a.Status}
		if err != nil {
			call["error"] = err.Error()
			trace(in, connectors.TraceError, fmt.Sprintf("%s GET %s failed: %v", srv.Name, path, err), call)
			return nil, nil, connectors.Sayf("%s did not answer GET %s", srv.Name, path)
		}
		call["answer"] = json.RawMessage(clipJSON(a.Body))
		if a.Status < 200 || a.Status > 299 {
			trace(in, connectors.TraceError, fmt.Sprintf("%s GET %s answered %d", srv.Name, path, a.Status), call)
			return nil, nil, connectors.Sayf("%s answered HTTP %d to GET %s", srv.Name, a.Status, path)
		}
		var body any
		if err := json.Unmarshal(a.Body, &body); err != nil {
			trace(in, connectors.TraceError, "the answer is not JSON", call)
			return nil, nil, connectors.Sayf("The answer of GET %s is not JSON: a recipe reads JSON only", path)
		}
		return body, call, nil
	}
	callArgs := withCursor(r.Args, cursor)
	if page != "" {
		callArgs[r.Next.Arg] = page
	}
	args, err := json.Marshal(callArgs)
	if err != nil {
		return nil, nil, err
	}
	callTool, callRaw := r.Tool, json.RawMessage(args)
	if r.Via != nil {
		callTool = r.Via.Call
		if callRaw, err = r.Via.CallArgs(r.Tool, args); err != nil {
			return nil, nil, err
		}
	}
	raw, err := Service.CallTool(ctx, srv.ID, callTool, callRaw)
	took := time.Since(began)
	if err != nil {
		trace(in, connectors.TraceError, fmt.Sprintf("%s %s failed in %s: %v", srv.Name, r.Tool, took.Round(time.Millisecond), err),
			map[string]any{"tool": r.Tool, "args": json.RawMessage(args), "ms": took.Milliseconds(), "error": err.Error()})
		return nil, nil, connectors.Sayf("%s did not answer the tool %q", srv.Name, r.Tool)
	}
	// The service's answer stays on the machine: the trace is the owner's, read by the page of this runtime only.
	call := map[string]any{"tool": r.Tool, "args": json.RawMessage(args), "ms": took.Milliseconds(), "answer": json.RawMessage(raw)}
	body, err := answer(raw)
	if err != nil {
		call["error"] = connectors.Say(err)
		trace(in, connectors.TraceError, r.Tool+": "+connectors.Say(err), call)
		return nil, nil, err
	}
	return body, call, nil
}

func clipJSON(b []byte) []byte {
	if json.Valid(b) {
		return b
	}
	q, _ := json.Marshal(string(b))
	return q
}

// listOfAnswer is the things of one answer: the script's items when the recipe has a script, else the list at items.
func (r Recipe) listOfAnswer(ctx context.Context, in connectors.Instance, body any, memo *json.RawMessage) ([]any, error) {
	if r.Script != "" {
		began := time.Now()
		out, err := scriptbox.Run(ctx, ScriptOptions, r.Script, body, *memo)
		if err != nil {
			trace(in, connectors.TraceError, "the script: "+err.Error(), map[string]any{"version": r.Version})
			return nil, connectors.Sayf("The recipe's script: %s", err.Error())
		}
		*memo = out.Memo
		trace(in, connectors.TraceInfo, fmt.Sprintf("the script gave %d items in %s", len(out.Items), time.Since(began).Round(time.Millisecond)),
			map[string]any{"version": r.Version, "items": len(out.Items)})
		return out.Items, nil
	}
	got := pick(body, r.Items)
	if got == nil {
		return nil, nil // an empty answer: nothing there now
	}
	list, ok := listOf(got)
	if !ok {
		trace(in, connectors.TraceError, fmt.Sprintf("the answer has no list at %q", r.Items), nil)
		return nil, connectors.Sayf("The answer has no list at %q", r.Items)
	}
	return list, nil
}

// factMax is how long a fact's value may be.
const factMax = 120

// mapThing is a thing of the answer as the recipe maps it; false when it has no id or the filter leaves it out.
func (r Recipe) mapThing(v any) (thing, bool) {
	t := thing{id: str(pick(v, r.ID)), actor: str(pick(v, r.Actor)), title: str(pick(v, r.Title)),
		url: str(pick(v, r.URL)), thread: str(pick(v, r.Thread))}
	if t.id == "" || !r.passes(v) {
		return t, false
	}
	if r.At != "" {
		t.at = when(pick(v, r.At))
	}
	if r.Cursor != "" {
		t.cursor = str(pick(v, r.Cursor))
	}
	for _, f := range r.Facts {
		if s := str(pick(v, f)); s != "" {
			if t.facts == nil {
				t.facts = map[string]string{}
			}
			t.facts[f] = clip(s, factMax)
		}
	}
	for _, f := range r.Watch {
		if t.watch == nil {
			t.watch = map[string]string{}
		}
		t.watch[f] = str(pick(v, f))
	}
	return t, true
}
