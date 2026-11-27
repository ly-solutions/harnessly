package mcpconn

// Which tools of a connection can be a trigger (docs/connectors.md «MCP recipe»): a classifier without a model
// gives each tool a score and its reasons for and against. A trigger is deterministic: a read-only tool that lists
// things, polled by a recipe. A gateway server (list_tools / call_tool, one MCP server in front of many) is unfolded
// through its meta tools: the candidate is the tool behind it, and the recipe calls it through call_tool.

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Gateway is how to reach a tool behind a gateway server: the meta tool that lists them and the one that calls one.
type Gateway struct {
	List string `json:"list"`
	Call string `json:"call"`
	// NameArg and ArgsArg are the call tool's arguments for the tool's name and its arguments; ServerArg names the
	// server behind the gateway when the gateway groups its tools by server.
	NameArg   string `json:"nameArg"`
	ArgsArg   string `json:"argsArg"`
	ServerArg string `json:"serverArg,omitempty"`
	Server    string `json:"server,omitempty"`
}

// CallArgs are the call tool's arguments for the tool behind the gateway.
func (g Gateway) CallArgs(tool string, args json.RawMessage) (json.RawMessage, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	m := map[string]any{g.NameArg: tool, g.ArgsArg: args}
	if g.ServerArg != "" && g.Server != "" {
		m[g.ServerArg] = g.Server
	}
	return json.Marshal(m)
}

var (
	gatewayLists = []string{"list_tools", "list-tools", "list_all_tools", "list-all-tools", "get_tools", "tools_list", "listtools"}
	gatewayCalls = []string{"call_tool", "call-tool", "invoke_tool", "execute_tool", "run_tool", "use_tool", "calltool"}
	nameArgs     = []string{"name", "tool", "tool_name", "toolName", "toolname"}
	argsArgs     = []string{"arguments", "args", "tool_args", "toolArgs", "input", "params", "parameters"}
	serverArgs   = []string{"server", "server_name", "serverName", "servername"}
)

// properties are the names of a schema's properties.
func properties(schema json.RawMessage) map[string]json.RawMessage {
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	_ = json.Unmarshal(schema, &s)
	return s.Properties
}

func required(schema json.RawMessage) []string {
	var s struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(schema, &s)
	return s.Required
}

func firstOf(props map[string]json.RawMessage, names []string) string {
	for _, n := range names {
		if _, ok := props[n]; ok {
			return n
		}
	}
	return ""
}

// DetectGateway says whether a server is a gateway: it lists its tools and calls one by name.
func DetectGateway(specs []ToolSpec) (Gateway, bool) {
	var g Gateway
	for _, t := range specs {
		n := strings.ToLower(t.Name)
		switch {
		case g.List == "" && slices.Contains(gatewayLists, n):
			g.List = t.Name
		case g.Call == "" && slices.Contains(gatewayCalls, n):
			props := properties(t.InputSchema)
			g.Call, g.NameArg, g.ArgsArg, g.ServerArg = t.Name, firstOf(props, nameArgs), firstOf(props, argsArgs), firstOf(props, serverArgs)
		}
	}
	return g, g.List != "" && g.Call != "" && g.NameArg != "" && g.ArgsArg != ""
}

// NestedTool is a tool behind a gateway and the server it belongs to there.
type NestedTool struct {
	ToolSpec
	Server string
}

// ParseGatewayTools reads the tools a gateway's list tool answers: its structured content or its text as JSON, a list
// of tools, a list of servers with their tools, or an object of server → tools.
func ParseGatewayTools(raw json.RawMessage) []NestedTool {
	body, err := ToolAnswer(raw)
	if err != nil {
		return nil
	}
	var out []NestedTool
	var walk func(v any, server string, depth int)
	walk = func(v any, server string, depth int) {
		if depth > 6 {
			return
		}
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e, server, depth+1)
			}
		case map[string]any:
			if name, ok := x["name"].(string); ok && name != "" && (x["inputSchema"] != nil || x["input_schema"] != nil || x["parameters"] != nil || x["description"] != nil) && x["tools"] == nil {
				t := NestedTool{Server: server}
				t.Name = name
				t.Description, _ = x["description"].(string)
				for _, k := range []string{"inputSchema", "input_schema", "parameters"} {
					if s, ok := x[k]; ok && s != nil {
						t.InputSchema, _ = json.Marshal(s)
						break
					}
				}
				if a, ok := x["annotations"]; ok {
					b, _ := json.Marshal(a)
					_ = json.Unmarshal(b, &t.Annotations)
				}
				if s, ok := x["outputSchema"]; ok {
					t.OutputSchema, _ = json.Marshal(s)
				}
				out = append(out, t)
				return
			}
			if tools, ok := x["tools"]; ok {
				srv := server
				for _, k := range []string{"server", "serverName", "server_name", "name"} {
					if s, ok := x[k].(string); ok && s != "" {
						srv = s
						break
					}
				}
				walk(tools, srv, depth+1)
				return
			}
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				srv := server
				if _, isList := x[k].([]any); isList && server == "" && k != "tools" && k != "result" && k != "data" && k != "items" {
					srv = k
				}
				walk(x[k], srv, depth+1)
			}
		}
	}
	walk(body, "", 0)
	return out
}

// ToolAnswer is a tool's result as data: its structuredContent, else its first text read as JSON.
func ToolAnswer(raw json.RawMessage) (any, error) {
	var res struct {
		IsError    bool            `json:"isError"`
		Structured json.RawMessage `json:"structuredContent"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, errors.New("the answer is not an MCP result")
	}
	if res.IsError {
		return nil, errors.New("the tool answered with an error")
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
			return nil, errors.New("the tool's text is not JSON")
		}
	}
	return nil, errors.New("the answer is empty")
}

// Candidate is one tool judged as a trigger.
type Candidate struct {
	Tool string `json:"tool"`
	// Gateway is set for a tool behind a gateway server: the recipe calls it through Gateway.Call.
	Gateway     *Gateway `json:"gateway,omitempty"`
	Description string   `json:"description,omitempty"`
	Score       int      `json:"score"`
	// Fits: the tool can be a trigger (not excluded, enough for it). Excluded: it may change something.
	Fits     bool     `json:"fits"`
	Excluded bool     `json:"excluded,omitempty"`
	Plus     []string `json:"plus"`
	Minus    []string `json:"minus"`
	Label    string   `json:"label"`
	LabelRu  string   `json:"labelRu"`
	// ReadOnly: the server marks it read-only; a recipe of one that is not marked needs the person's allowance.
	ReadOnly bool `json:"readOnly"`
	// Args are the tool's arguments: which takes the cursor, which the person must fill in.
	Args []CandidateArg `json:"args,omitempty"`
}

// CandidateArg is one argument of a candidate tool.
type CandidateArg struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
	// Cursor: the argument takes «since» — the recipe passes its cursor here.
	Cursor      bool   `json:"cursor,omitempty"`
	Description string `json:"description,omitempty"`
}

var (
	writeVerbs = []string{"create", "send", "post", "delete", "update", "generate", "upload", "edit", "remove", "add", "set",
		"archive", "unarchive", "pin", "unpin", "join", "leave", "reply", "react", "invite", "move", "rename", "mark", "write",
		"put", "patch", "insert", "drop", "kick", "ban", "approve", "merge", "close", "reopen", "assign", "publish", "submit",
		"cancel", "start", "stop", "run", "execute", "trigger", "transfer", "pay", "buy", "sell", "order", "book", "schedule"}
	// cursorArgs take «since», best first: a time over an id over a page cursor.
	cursorArgs = []string{"since", "updated_after", "updatedafter", "updated_since", "modified_since", "created_after",
		"createdafter", "start_time", "starttime", "min_time", "oldest", "from", "after", "after_id", "since_id", "min_id", "cursor"}
	pageArgs   = []string{"page", "limit", "per_page", "perpage", "page_size", "pagesize", "count", "max_results", "top", "offset", "size"}
	listNames  = []string{"list", "search", "find", "query", "browse"}
	freshWords = []string{"updates", "history", "recent", "feed", "new", "unread", "latest", "changes", "activity", "events", "timeline", "notifications", "inbox", "unreads"}
	splitRe    = regexp.MustCompile(`[^a-z0-9]+`)
	camelRe    = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

func nameWords(name string) []string {
	name = camelRe.ReplaceAllString(name, "${1}_${2}")
	var out []string
	for _, w := range splitRe.Split(strings.ToLower(name), -1) {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// Classify judges each tool. Tools are returned best first; those that do not fit are kept with their reasons.
func Classify(specs []ToolSpec) []Candidate {
	out := make([]Candidate, 0, len(specs))
	for _, t := range specs {
		out = append(out, classify(t))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Fits != out[j].Fits {
			return out[i].Fits
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func classify(t ToolSpec) Candidate {
	c := Candidate{Tool: t.Name, Description: firstSentence(t.Description), Plus: []string{}, Minus: []string{}}
	ws := nameWords(t.Name)
	a := t.Annotations
	if a != nil && a.DestructiveHint != nil && *a.DestructiveHint && (a.ReadOnlyHint == nil || !*a.ReadOnlyHint) {
		c.Minus = append(c.Minus, "destructiveHint")
		c.Excluded = true
	}
	if a != nil && a.ReadOnlyHint != nil && *a.ReadOnlyHint {
		c.ReadOnly = true
		c.Score += 3
		c.Plus = append(c.Plus, "readOnlyHint")
	} else {
		c.Minus = append(c.Minus, "not marked read-only")
	}
	if len(ws) > 0 && slices.Contains(writeVerbs, ws[0]) {
		c.Minus = append(c.Minus, "the name says it changes something: "+ws[0])
		c.Excluded = true
	} else {
		for _, w := range ws[1:] {
			if slices.Contains(writeVerbs, w) && !slices.Contains([]string{"post", "order", "schedule", "start", "run"}, w) {
				c.Minus = append(c.Minus, "the name says it changes something: "+w)
				c.Excluded = true
				break
			}
		}
	}
	listy := false
	if len(ws) > 0 && slices.Contains(listNames, ws[0]) {
		c.Score++
		listy = true
		c.Plus = append(c.Plus, "the name lists things: "+ws[0]+"_")
	}
	for _, w := range ws {
		if slices.Contains(freshWords, w) {
			c.Score += 2
			listy = true
			c.Plus = append(c.Plus, "the name is about what is new: "+w)
			break
		}
	}
	props := properties(t.InputSchema)
	req := required(t.InputSchema)
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	best := -1 // the argument that takes the cursor: the first of cursorArgs the tool has
	for _, n := range names {
		if i := slices.Index(cursorArgs, strings.ToLower(n)); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	pageSeen := false
	for _, n := range names {
		var p struct {
			Type        any    `json:"type"`
			Description string `json:"description"`
		}
		_ = json.Unmarshal(props[n], &p)
		arg := CandidateArg{Name: n, Type: typeName(p.Type), Required: slices.Contains(req, n), Description: firstSentence(p.Description)}
		ln := strings.ToLower(n)
		if best >= 0 && ln == cursorArgs[best] {
			arg.Cursor = true
			c.Score += 2
			c.Plus = append(c.Plus, "takes "+n+": only what is new can be asked")
			listy = true
		} else if !slices.Contains(cursorArgs, ln) && slices.Contains(pageArgs, ln) && !pageSeen {
			c.Score++
			pageSeen, listy = true, true
			c.Plus = append(c.Plus, "takes "+n+": a list in pages")
		}
		c.Args = append(c.Args, arg)
	}
	if listWithIDAndTime(t.OutputSchema) {
		c.Score += 2
		listy = true
		c.Plus = append(c.Plus, "its output is a list of things with an id and a time")
	}
	if !listy {
		c.Minus = append(c.Minus, "nothing says it lists things")
	}
	c.Fits = !c.Excluded && listy && c.Score >= 2
	c.Label, c.LabelRu = labelOf(ws, t.Description)
	return c
}

func typeName(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && s != "null" {
				return s
			}
		}
	}
	return ""
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\n"); i > 0 {
		s = s[:i]
	}
	if len([]rune(s)) > 160 {
		s = string([]rune(s)[:160]) + "…"
	}
	return s
}

// listWithIDAndTime says whether an output schema is a list (or holds one) whose items have an id and a time.
func listWithIDAndTime(schema json.RawMessage) bool {
	if len(schema) == 0 {
		return false
	}
	var v any
	if json.Unmarshal(schema, &v) != nil {
		return false
	}
	var find func(v any, depth int) bool
	find = func(v any, depth int) bool {
		m, ok := v.(map[string]any)
		if !ok || depth > 5 {
			return false
		}
		if m["type"] == "array" {
			if items, ok := m["items"].(map[string]any); ok {
				props, _ := items["properties"].(map[string]any)
				hasID, hasTime := false, false
				for k := range props {
					if slices.Contains(idKeys, k) {
						hasID = true
					}
					if slices.Contains(timeKeys, k) {
						hasTime = true
					}
				}
				return hasID && hasTime
			}
		}
		if props, ok := m["properties"].(map[string]any); ok {
			for _, p := range props {
				if find(p, depth+1) {
					return true
				}
			}
		}
		return false
	}
	return find(v, 0)
}

// The keys a draft recipe looks for in a thing, best first.
var (
	idKeys     = []string{"id", "uuid", "_id", "key", "guid", "message_id", "post_id", "event_id"}
	timeKeys   = []string{"created_at", "create_at", "createdAt", "updated_at", "update_at", "updatedAt", "ts", "time", "timestamp", "date", "created", "updated", "published_at", "done_at", "finished_at"}
	titleKeys  = []string{"message", "title", "name", "text", "subject", "summary", "prompt", "body", "content", "description"}
	actorKeys  = []string{"user_id", "author", "user", "username", "from", "sender", "owner", "creator", "author_id", "created_by"}
	urlKeys    = []string{"permalink", "url", "link", "html_url", "web_url", "href", "webUrl", "htmlUrl"}
	threadKeys = []string{"root_id", "thread_id", "threadId", "parent_id", "conversation_id", "thread_ts"}
)

// GuessKeys are what a draft recipe takes as a thing's fields.
func GuessKeys() (id, at, title, actor, url, thread []string) {
	return idKeys, timeKeys, titleKeys, actorKeys, urlKeys, threadKeys
}

// nouns are the things a tool's name may list, in English and Russian (plural), with the singular for a trigger's name.
var nouns = []struct{ word, en, ru, one string }{
	{"posts", "messages", "сообщения", "message"}, {"messages", "messages", "сообщения", "message"},
	{"message", "messages", "сообщения", "message"}, {"post", "messages", "сообщения", "message"},
	{"chats", "chats", "чаты", "chat"}, {"mentions", "mentions", "упоминания", "mention"},
	{"issues", "issues", "задачи", "issue"}, {"tasks", "tasks", "задачи", "task"}, {"tickets", "tickets", "тикеты", "ticket"},
	{"comments", "comments", "комментарии", "comment"}, {"events", "events", "события", "event"},
	{"files", "files", "файлы", "file"}, {"documents", "documents", "документы", "document"}, {"docs", "documents", "документы", "document"},
	{"jobs", "jobs", "задания", "job"}, {"generations", "generations", "генерации", "generation"},
	{"orders", "orders", "заказы", "order"}, {"payments", "payments", "платежи", "payment"},
	{"transactions", "transactions", "операции", "transaction"}, {"operations", "operations", "операции", "operation"},
	{"emails", "emails", "письма", "email"}, {"mails", "emails", "письма", "email"}, {"letters", "emails", "письма", "email"},
	{"commits", "commits", "коммиты", "commit"}, {"pulls", "pull requests", "пул-реквесты", "pull"},
	{"deals", "deals", "сделки", "deal"}, {"leads", "leads", "лиды", "lead"}, {"contacts", "contacts", "контакты", "contact"},
	{"alerts", "alerts", "алерты", "alert"}, {"incidents", "incidents", "инциденты", "incident"},
	{"notifications", "notifications", "уведомления", "notification"}, {"unreads", "unread messages", "непрочитанные", "unread"},
	{"records", "records", "записи", "record"}, {"items", "items", "записи", "item"}, {"entries", "entries", "записи", "entry"},
	{"users", "users", "пользователи", "user"}, {"members", "members", "участники", "member"},
	{"channels", "channels", "каналы", "channel"}, {"teams", "teams", "команды", "team"}, {"projects", "projects", "проекты", "project"},
	{"results", "results", "результаты", "result"}, {"reviews", "reviews", "отзывы", "review"}, {"invoices", "invoices", "счета", "invoice"},
	{"articles", "articles", "статьи", "article"}, {"videos", "videos", "видео", "video"}, {"images", "images", "картинки", "image"},
}

var places = []struct{ word, en, ru string }{
	{"channel", "in a channel", "в канале"}, {"chat", "in a chat", "в чате"}, {"project", "in a project", "в проекте"},
	{"repo", "in a repository", "в репозитории"}, {"repository", "in a repository", "в репозитории"},
	{"board", "on a board", "на доске"}, {"folder", "in a folder", "в папке"}, {"team", "in a team", "в команде"},
	{"inbox", "in the inbox", "во входящих"}, {"mailbox", "in the mailbox", "в почте"}, {"account", "on an account", "на счёте"},
	{"calendar", "in a calendar", "в календаре"}, {"thread", "in a thread", "в треде"}, {"space", "in a space", "в пространстве"},
	{"workspace", "in a workspace", "в рабочем пространстве"}, {"group", "in a group", "в группе"},
}

// labelOf is a person's name of what the trigger catches, from the tool's name (and its description when the name
// says too little): «New messages in a channel» / «Новые сообщения в канале».
func labelOf(ws []string, description string) (string, string) {
	noun, place := -1, -1
	search := slices.Contains(ws, "search") || slices.Contains(ws, "find")
	for _, w := range ws {
		for i, n := range nouns {
			if noun < 0 && w == n.word {
				noun = i
			}
		}
		for i, p := range places {
			if place < 0 && w == p.word && (noun < 0 || nouns[noun].word != w) {
				place = i
			}
		}
	}
	if noun < 0 {
		for _, w := range nameWords(description) {
			for i, n := range nouns {
				if noun < 0 && w == n.word {
					noun = i
				}
			}
		}
	}
	if noun < 0 {
		human := strings.Join(ws, " ")
		return "New in «" + human + "»", "Новое в «" + human + "»"
	}
	en, ru := "New "+nouns[noun].en, "Новые "+nouns[noun].ru
	if place >= 0 && places[place].word+"s" != nouns[noun].word {
		en += " " + places[place].en
		ru += " " + places[place].ru
	}
	if search {
		en += " by a search"
		ru += " по поиску"
	}
	return en, ru
}

// TriggerName is a recipe's trigger made of a candidate: «message.new».
func TriggerName(tool string, description string) string {
	ws := nameWords(tool)
	for _, w := range append(ws, nameWords(description)...) {
		for _, n := range nouns {
			if w == n.word {
				return n.one + ".new"
			}
		}
	}
	base := "item"
	for i := len(ws) - 1; i >= 0; i-- {
		if !slices.Contains(listNames, ws[i]) && ws[i] != "get" {
			base = ws[i]
			break
		}
	}
	return base + ".new"
}

// Candidates are the tools of a connection judged as triggers; a gateway's tools behind it are unfolded (its list
// tool is a read, called once).
func (s *Service) Candidates(ctx context.Context, id string) ([]Candidate, error) {
	specs, err := s.ToolSpecs(ctx, id)
	if err != nil {
		return nil, err
	}
	gw, ok := DetectGateway(specs)
	if !ok {
		return Classify(specs), nil
	}
	raw, err := s.CallTool(ctx, id, gw.List, nil)
	if err != nil {
		return Classify(specs), nil
	}
	nested := ParseGatewayTools(raw)
	out := make([]Candidate, 0, len(nested))
	for _, t := range nested {
		c := classify(t.ToolSpec)
		g := gw
		g.Server = t.Server
		c.Gateway = &g
		c.Plus = append(c.Plus, "behind the gateway's "+gw.Call)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Fits != out[j].Fits {
			return out[i].Fits
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}

// NestedSpec is the spec of a tool behind a gateway, read from its list tool now.
func (s *Service) NestedSpec(ctx context.Context, id string, gw Gateway, tool string) (ToolSpec, bool, error) {
	raw, err := s.CallTool(ctx, id, gw.List, nil)
	if err != nil {
		return ToolSpec{}, false, err
	}
	for _, t := range ParseGatewayTools(raw) {
		if t.Name == tool && (gw.Server == "" || t.Server == gw.Server) {
			return t.ToolSpec, true, nil
		}
	}
	return ToolSpec{}, false, nil
}
