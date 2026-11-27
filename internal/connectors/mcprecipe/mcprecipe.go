// Package mcprecipe is the connector of a poll recipe over an MCP server (docs/connectors.md, «MCP
// recipe»): a service that gives no events — only tools — becomes a source anyway. A recipe is data, not code: which
// read-only tool of a connected MCP server to call, with which arguments (the cursor in them), where the list of
// things is in the answer and which fields of a thing are its id, time, author, title and link. An agent writes the
// recipe once, when the service is connected; from then on the runtime calls the tool by itself on a schedule, with no
// model in the loop, and every thing not seen before is an event — journaled by the hub, digested by the inbox, its
// trigger in the catalog like any other.
//
// The first poll only learns what is there (no flood of the past). A tool the server does not mark read-only is
// refused unless the person allowed it; one it marks destructive never runs.
package mcprecipe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
	"github.com/ly-solutions/harnessly/internal/scriptbox"
)

// ID names the connector in every event.
const ID = "mcp-recipe"

// EventItem is the one type of its events; the recipe's own name of what happened is the fact «trigger».
const EventItem = "feed.item"

// MCP is what the connector needs of the runtime's MCP connections (mcpconn.Service).
type MCP interface {
	List() ([]mcpconn.Server, error)
	ToolSpecs(ctx context.Context, id string) ([]mcpconn.ToolSpec, error)
	CallTool(ctx context.Context, id, tool string, args json.RawMessage) (json.RawMessage, error)
}

// Service is the runtime's MCP connections, set at start; nil: the connector cannot run.
var Service MCP

// Recipe is the poll of one kind of thing of an MCP server.
type Recipe struct {
	// Trigger is the recipe's name of what happened: «generation.done», «message.new». Goes to the event as a fact.
	Trigger string         `json:"trigger"`
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args,omitempty"` // "{{cursor}}" in a string value is the cursor ("" on the first poll)
	// Items is the dotted path to the list in the answer ("" — the answer is the list); the answer is the tool's
	// structuredContent, else its first text content read as JSON.
	Items string `json:"items,omitempty"`
	// The dotted paths of a thing's fields; ID is required.
	ID     string `json:"id"`
	At     string `json:"at,omitempty"`
	Actor  string `json:"actor,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
	Thread string `json:"thread,omitempty"`
	// Cursor is the field whose greatest value is passed back as {{cursor}} ("" — none: things are told apart by id).
	Cursor string `json:"cursor,omitempty"`
	// Every is how often to poll (Go duration, 1m to 24h; 10m by default).
	Every string `json:"every,omitempty"`
	// Via calls Tool through a gateway server's call tool: Tool is the tool behind it.
	Via *mcpconn.Gateway `json:"via,omitempty"`
	// Filter keeps only the things whose field (a dotted path) contains the text, case aside; all must match.
	Filter map[string]string `json:"filter,omitempty"`
	// Mode is the trigger's mode the first time the hub sees it (digest, notify, agent); the person changes it on /events.
	Mode string `json:"mode,omitempty"`

	// Watch are fields (dotted paths) whose change on a thing already seen is an event item.changed, with the fact
	// «was → now». Their hash is kept, and a short value that looks like a status.
	Watch []string `json:"watch,omitempty"`
	// Facts are fields put into every event's facts (at most 8, values clipped), for rules to filter by.
	Facts []string `json:"facts,omitempty"`
	// Next reads further pages: the path of the next page's cursor in the answer and the argument it goes in.
	Next *Next `json:"next,omitempty"`
	// Path and Query make the source an API connection's GET instead of a tool: "{{cursor}}" in them is the cursor.
	Path  string            `json:"path,omitempty"`
	Query map[string]string `json:"query,omitempty"`
	// Script reshapes each answer: a JavaScript function (answer, memo) => ({items, memo}), run in a sandbox with no
	// network, files or timers (internal/scriptbox). A new script is a new version, confirmed by the person.
	Script  string `json:"script,omitempty"`
	Version int    `json:"version,omitempty"`
}

// Next is how to ask for the next page.
type Next struct {
	Path string `json:"path"`
	Arg  string `json:"arg"`
}

// EventChanged is the event of a watched field that changed on a thing already seen.
const EventChanged = "item.changed"

var triggerRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z0-9_-]+)*$`)

func (r Recipe) check() error {
	switch {
	case !triggerRe.MatchString(r.Trigger) || len(r.Trigger) > 48:
		return connectors.Sayf("The recipe's trigger: lower-case words with dots, like generation.done")
	case r.Tool == "" && r.Path == "":
		return connectors.Sayf("The recipe names no tool (or path of an API)")
	case r.Tool != "" && r.Path != "":
		return connectors.Sayf("The recipe names a tool and a path: one source")
	case r.Path != "" && r.Via != nil:
		return connectors.Sayf("The recipe's via is for a tool, not a path")
	case len(r.Facts) > 8:
		return connectors.Sayf("The recipe's facts: at most 8 fields")
	case len(r.Watch) > 8:
		return connectors.Sayf("The recipe's watch: at most 8 fields")
	case r.Next != nil && (r.Next.Path == "" || r.Next.Arg == ""):
		return connectors.Sayf("The recipe's next names the path of the next page's cursor and the argument it goes in")
	case len(r.Script) > scriptbox.ScriptMax:
		return connectors.Sayf("The recipe's script is longer than %d bytes", scriptbox.ScriptMax)
	case r.ID == "":
		return connectors.Sayf("The recipe names no id field of a thing")
	case r.Mode != "" && !slices.Contains([]string{"digest", "notify", "agent"}, r.Mode):
		return connectors.Sayf("The recipe's mode: digest, notify or agent")
	case r.Via != nil && (r.Via.Call == "" || r.Via.List == "" || r.Via.NameArg == "" || r.Via.ArgsArg == ""):
		return connectors.Sayf("The recipe's via names the gateway's list and call tools and their arguments")
	}
	if r.Every != "" {
		d, err := time.ParseDuration(r.Every)
		if err != nil || d < time.Minute || d > 24*time.Hour {
			return connectors.Sayf("The recipe's every: from 1m to 24h")
		}
	}
	return nil
}

func (r Recipe) every() time.Duration {
	if d, err := time.ParseDuration(r.Every); err == nil && d >= time.Minute && d <= 24*time.Hour {
		return d
	}
	return 10 * time.Minute
}

type conn struct{}

func init() { connectors.Register(conn{}) }

func (conn) Spec() connectors.Spec {
	return connectors.Spec{
		ID:    ID,
		Kind:  connectors.Feed,
		Inbox: "feed",
		Title: connectors.T("MCP recipe", "Рецепт MCP"),
		Summary: connectors.T("Updates of a service connected through MCP: a read-only tool polled by a recipe",
			"Обновления сервиса, подключённого через MCP: инструмент на чтение, опрашиваемый по рецепту"),
		Fields: []connectors.Field{
			{ID: "server", Type: connectors.FieldChoice, Discover: true, Label: connectors.T("MCP server", "MCP-сервер")},
			{ID: "recipe", Type: connectors.FieldText, Label: connectors.T("Recipe", "Рецепт"),
				Placeholder: `{"trigger":"generation.done","tool":"list_jobs","args":{"since":"{{cursor}}"},"items":"jobs","id":"id","at":"created_at","title":"prompt","cursor":"created_at"}`,
				Hint: connectors.T("JSON: which tool, where the list is, which fields are the id, time, title, link. An agent writes it for you",
					"JSON: какой инструмент, где список, какие поля — id, время, заголовок, ссылка. Его пишет агент")},
			{ID: "unmarked", Type: connectors.FieldChoice, Optional: true, Default: "no",
				Label: connectors.T("A tool not marked read-only", "Инструмент без пометки «только чтение»"),
				Hint: connectors.T("Only if you know the tool only reads: the server did not say so",
					"Только если вы знаете, что инструмент лишь читает: сервер этого не сказал"),
				Choices: []connectors.Choice{
					{Value: "no", Label: connectors.T("Refuse", "Отказать")},
					{Value: "yes", Label: connectors.T("Allow", "Разрешить")},
				}},
		},
		Events: []string{EventItem, EventChanged},
	}
}

func recipeOf(in connectors.Instance) (Recipe, error) {
	var r Recipe
	if err := json.Unmarshal([]byte(in.Config["recipe"]), &r); err != nil {
		return r, connectors.Sayf("The recipe is not JSON: %s", strings.SplitN(err.Error(), ":", 2)[0])
	}
	return r, r.check()
}

// server finds the MCP connection by its id or slug.
func server(ref string) (mcpconn.Server, error) {
	if Service == nil {
		return mcpconn.Server{}, connectors.Sayf("MCP connections are not available on this machine")
	}
	list, err := Service.List()
	if err != nil {
		return mcpconn.Server{}, err
	}
	for _, s := range list {
		if s.ID == ref || s.Slug == ref {
			return s, nil
		}
	}
	return mcpconn.Server{}, connectors.Sayf("No MCP server %q is connected", ref)
}

// Discover lists the connected MCP servers for the field «server».
func (conn) Discover(_ context.Context, _ connectors.Instance, field string) ([]connectors.Choice, error) {
	if field != "server" || Service == nil {
		return nil, nil
	}
	list, err := Service.List()
	if err != nil {
		return nil, err
	}
	var out []connectors.Choice
	for _, s := range list {
		out = append(out, connectors.Choice{Value: s.Slug, Label: connectors.T(s.Name, s.Name)})
	}
	return out, nil
}

// guard refuses a tool that may change something: destructive always, not marked read-only unless allowed. A tool
// behind a gateway is judged by what the gateway's list says of it, not by the call tool (which may do anything).
func guard(ctx context.Context, in connectors.Instance, srv mcpconn.Server, tool string, via *mcpconn.Gateway) error {
	var specs []mcpconn.ToolSpec
	if via != nil {
		raw, err := Service.CallTool(ctx, srv.ID, via.List, json.RawMessage("{}"))
		if err != nil {
			return connectors.Sayf("%s did not list the tools behind it", srv.Name)
		}
		for _, t := range mcpconn.ParseGatewayTools(raw) {
			if via.Server == "" || t.Server == via.Server {
				specs = append(specs, t.ToolSpec)
			}
		}
	} else {
		var err error
		if specs, err = Service.ToolSpecs(ctx, srv.ID); err != nil {
			return err
		}
	}
	i := slices.IndexFunc(specs, func(t mcpconn.ToolSpec) bool { return t.Name == tool })
	if i < 0 {
		return connectors.Sayf("%s has no tool %q", srv.Name, tool)
	}
	a := specs[i].Annotations
	if a != nil && a.DestructiveHint != nil && *a.DestructiveHint && (a.ReadOnlyHint == nil || !*a.ReadOnlyHint) {
		return connectors.Sayf("%s marks %q as destructive: a recipe never calls it", srv.Name, tool)
	}
	if (a == nil || a.ReadOnlyHint == nil || !*a.ReadOnlyHint) && in.Config["unmarked"] != "yes" {
		return connectors.Sayf("%s does not mark %q as read-only: allow it in the form only if you know it only reads", srv.Name, tool)
	}
	return nil
}

// answer is the tool's result as data: its structuredContent, else its first text read as JSON.
func answer(raw json.RawMessage) (any, error) {
	var res struct {
		IsError    bool            `json:"isError"`
		Structured json.RawMessage `json:"structuredContent"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, connectors.Sayf("The tool's answer is not an MCP result")
	}
	if res.IsError {
		return nil, connectors.Sayf("The tool answered with an error")
	}
	var v any
	if len(res.Structured) > 0 && string(res.Structured) != "null" {
		if err := json.Unmarshal(res.Structured, &v); err == nil {
			return v, nil
		}
	}
	for _, c := range res.Content {
		if c.Type == "text" {
			if err := json.Unmarshal([]byte(c.Text), &v); err == nil {
				return v, nil
			}
			return nil, connectors.Sayf("The tool's text is not JSON: a recipe reads JSON only")
		}
	}
	return nil, connectors.Sayf("The tool's answer is empty")
}

// pick follows a dotted path ("data.items", "items.0.id"); "" is the value itself.
func pick(v any, path string) any {
	if path == "" {
		return v
	}
	for _, k := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[k]
		case []any:
			i, err := strconv.Atoi(k)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// when reads a time: RFC 3339, or Unix seconds or milliseconds.
func when(v any) time.Time {
	switch x := v.(type) {
	case string:
		if t, ok := parseTime(x); ok {
			return t
		}
		if n, err := strconv.ParseFloat(x, 64); err == nil {
			return when(n)
		}
	case float64:
		if x > 1e12 {
			return time.UnixMilli(int64(x))
		}
		if x > 1e8 {
			return time.Unix(int64(x), 0)
		}
	}
	return time.Time{}
}

// withCursor puts the cursor where a string argument says {{cursor}}.
// withCursor puts the cursor into the arguments. "{{cursor}}" alone is left out on the first poll (no cursor yet),
// "{{cursor:number}}" passes it as a number (a time in milliseconds, an id) and is left out until there is one.
func withCursor(args map[string]any, cursor string) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		s, ok := v.(string)
		switch {
		case !ok:
			out[k] = v
		case s == "{{cursor:number}}":
			if n, err := strconv.ParseFloat(cursor, 64); err == nil {
				out[k] = n
			}
		case s == "{{cursor}}" && cursor == "":
		default:
			out[k] = strings.ReplaceAll(s, "{{cursor}}", cursor)
		}
	}
	return out
}

// listOf is the things of an answer: a list, or an object of id → thing (Mattermost's posts), its values in key order.
func listOf(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k, e := range x {
			if _, obj := e.(map[string]any); !obj {
				return nil, false
			}
			keys = append(keys, k)
		}
		slices.Sort(keys)
		out := make([]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, x[k])
		}
		return out, true
	}
	return nil, false
}

// passes says whether a thing matches the recipe's filter.
func (r Recipe) passes(v any) bool {
	for path, want := range r.Filter {
		if !strings.Contains(strings.ToLower(str(pick(v, path))), strings.ToLower(want)) {
			return false
		}
	}
	return true
}

// Check is the dry run of the recipe when the person connects it: the tool is called, the answer mapped, and the form
// shows how many things it found and the first one — what the events will look like.
func (conn) Check(ctx context.Context, in connectors.Instance) (connectors.Account, error) {
	r, err := recipeOf(in)
	if err != nil {
		return connectors.Account{}, err
	}
	res, err := run(ctx, in, r, "", nil)
	if err != nil {
		return connectors.Account{}, err
	}
	things := res.things
	label := fmt.Sprintf("%s · %s: %d now", in.Config["server"], r.Trigger, len(things))
	if len(things) > 0 && things[0].title != "" {
		label += " · «" + clip(things[0].title, 60) + "»"
	}
	return connectors.Account{Label: label}, nil
}

// Interval is the recipe's every.
func (conn) Interval(in connectors.Instance) time.Duration {
	r, _ := recipeOf(in)
	return r.every()
}

// memo is what the connection keeps between polls.
type memo struct {
	Cursor string   `json:"cursor"`
	Seen   []string `json:"seen"` // the newest ids, to tell new things from old without a cursor
	Primed bool     `json:"primed"`
	// Watched is, per thing, its watched fields as hashes (and a short status-like value, to say «was»).
	Watched map[string]map[string]string `json:"watched,omitempty"`
	// Script is the recipe's script's own memo.
	Script json.RawMessage `json:"script,omitempty"`
}

const seenMax = 1000

// statusLike is a value short and plain enough to keep as it is, to say «was → now»: a status, a stage, a number.
var statusLike = regexp.MustCompile(`^[\p{L}\p{N}_. :/-]{1,32}$`)

// watchMark is what the memo keeps of a watched value: its hash, and the value itself when it is status-like.
func watchMark(v string) string {
	sum := sha256.Sum256([]byte(v))
	h := hex.EncodeToString(sum[:6])
	if statusLike.MatchString(v) {
		return h + "=" + v
	}
	return h
}

// wasOf is the value a mark kept, or "…" when only its hash is.
func wasOf(mark string) string {
	if _, v, ok := strings.Cut(mark, "="); ok {
		return v
	}
	return "…"
}

func sameMark(a, b string) bool {
	ha, _, _ := strings.Cut(a, "=")
	hb, _, _ := strings.Cut(b, "=")
	return ha == hb
}

// Poll asks the source and gives the things not seen before as events, and item.changed for a watched field that
// changed. The first poll gives none: it learns what is there already, so connecting a service does not pour its past
// into the digest.
func (conn) Poll(ctx context.Context, in connectors.Instance, from connectors.Cursor) ([]connectors.Event, connectors.Cursor, error) {
	r, err := recipeOf(in)
	if err != nil {
		return nil, from, err
	}
	var m memo
	if in.Load != nil {
		_ = in.Load(&m)
	}
	before := m.Cursor
	res, err := run(ctx, in, r, m.Cursor, m.Script)
	if err != nil {
		return nil, from, err
	}
	things := res.things
	m.Script = res.memo
	if in.Warn != nil {
		if res.capped {
			in.Warn(fmt.Sprintf("More than %d pages at once: the rest is read at the next poll", pageMax))
		} else {
			in.Warn("")
		}
	}
	seen := map[string]bool{}
	for _, id := range m.Seen {
		seen[id] = true
	}
	if m.Watched == nil {
		m.Watched = map[string]map[string]string{}
	}
	base := func() map[string]string {
		facts := map[string]string{"trigger": r.Trigger, "server": in.Config["server"]}
		if r.Mode != "" {
			facts["mode"] = r.Mode // the trigger's first mode in the hub's catalog
		}
		return facts
	}
	var events []connectors.Event
	changed := 0
	for _, t := range things {
		marks := map[string]string{}
		for f, v := range t.watch {
			marks[f] = watchMark(v)
		}
		if seen[t.id] {
			old := m.Watched[t.id]
			var fields []string
			for f, mark := range marks {
				if prev, ok := old[f]; ok && !sameMark(prev, mark) {
					fields = append(fields, f)
				}
			}
			if len(marks) > 0 {
				m.Watched[t.id] = marks
			}
			if len(fields) == 0 || !m.Primed {
				continue
			}
			slices.Sort(fields)
			facts := base()
			for k, v := range t.facts {
				facts[k] = v
			}
			var key []string
			for _, f := range fields {
				facts[f] = wasOf(old[f]) + " → " + clip(t.watch[f], factMax)
				key = append(key, strings.SplitN(marks[f], "=", 2)[0])
			}
			facts["changed"] = strings.Join(fields, ",")
			events = append(events, connectors.Event{Type: EventChanged, ID: r.Trigger + ":" + t.id + ":" + strings.Join(key, "."),
				At: t.at, Actor: t.actor, Title: t.title, URL: t.url, Thread: t.thread, Facts: facts})
			changed++
			continue
		}
		seen[t.id] = true
		m.Seen = append(m.Seen, t.id)
		if len(marks) > 0 {
			m.Watched[t.id] = marks
		}
		if t.cursor != "" && later(t.cursor, m.Cursor) {
			m.Cursor = t.cursor
		}
		if !m.Primed {
			continue
		}
		facts := base()
		for k, v := range t.facts {
			facts[k] = v
		}
		events = append(events, connectors.Event{Type: EventItem, ID: r.Trigger + ":" + t.id, At: t.at, Actor: t.actor,
			Title: t.title, URL: t.url, Thread: t.thread, Facts: facts})
	}
	if connectors.Trace.On() {
		fresh := make([]string, 0, len(events))
		for _, ev := range events {
			fresh = append(fresh, ev.ID)
		}
		line := fmt.Sprintf("%d things, %d new, %d changed", len(things), len(fresh)-changed, changed)
		if !m.Primed {
			line = fmt.Sprintf("first poll learned %d things, no events", len(things))
		}
		trace(in, connectors.TraceInfo, line, map[string]any{"things": len(things), "new": fresh,
			"cursorBefore": before, "cursorAfter": m.Cursor, "primed": m.Primed, "capped": res.capped})
	}
	m.Primed = true
	if len(m.Seen) > seenMax {
		m.Seen = m.Seen[len(m.Seen)-seenMax:]
	}
	if len(m.Watched) > seenMax {
		keep := map[string]bool{}
		for _, id := range m.Seen {
			keep[id] = true
		}
		for id := range m.Watched {
			if !keep[id] {
				delete(m.Watched, id)
			}
		}
	}
	if in.Store != nil {
		if err := in.Store(m); err != nil {
			return nil, from, err
		}
	}
	return events, from, nil
}

// later says a is after b: times as times (RFC 3339 of any precision and zone), numbers as numbers, the rest as text.
func later(a, b string) bool {
	if b == "" {
		return true
	}
	if x, ok := parseTime(a); ok {
		if y, ok := parseTime(b); ok {
			return x.After(y)
		}
	}
	x, e1 := strconv.ParseFloat(a, 64)
	y, e2 := strconv.ParseFloat(b, 64)
	if e1 == nil && e2 == nil {
		return x > y
	}
	return a > b
}

// timeLayouts are the written times a cursor may be.
var timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z0700", "2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999", "2006-01-02"}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 10 || s[4] != '-' {
		return time.Time{}, false
	}
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// trace writes to the developer mode's trace of the connection.
func trace(in connectors.Instance, level, line string, data any) {
	connectors.Trace.Add(in.ID, "recipe", level, line, data)
}
