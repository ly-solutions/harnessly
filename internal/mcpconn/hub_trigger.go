package mcpconn

// trigger_add: an agent makes a trigger of a connection's read-only tool — a poll recipe of the connector
// mcp-recipe — by the same way as connection_add: the recipe is run once (dry run), then the person says yes on a card
// in the chat, and only then the trigger exists. Every new thing the tool lists is then an event of the journal and
// the digest, in the mode the recipe asks for its first time (the person changes it on /events → Triggers).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TriggerPlan is what trigger_add would make: the card's lines, what the dry run found, and the way to make it.
type TriggerPlan struct {
	Name  string
	Pairs []Pair
	// Found is the dry run in a few lines: the newest things as they will be events.
	Found string
	Save  func(ctx context.Context) (string, error)
}

// TriggerPlanner checks a recipe for a connection (its slug) and runs it once; unmarked allows a tool the server does
// not mark read-only. The runtime gives it: the recipe connector lives beside the hub, not in it.
// replace names an existing trigger (its connection id) whose recipe becomes a new version.
type TriggerPlanner func(ctx context.Context, connection string, recipe json.RawMessage, unmarked bool, name, replace string) (TriggerPlan, error)

var triggerAddTool = map[string]any{"name": "trigger_add",
	"description": "Make a trigger of a connection: its read-only tool that lists things (messages, jobs, records) is polled by a recipe, and every new thing becomes an event of the person's journal and digest — or a notice, or a task for the agent. candidates true (with connection) lists the tools that can be a trigger, with reasons, and the arguments to fill. The recipe is run once first; then the person confirms on a card in this chat, and only then the trigger exists. dry_run only runs it and shows the newest things as they will be events. Never a tool that creates, sends or spends.",
	"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
		"connection": map[string]any{"type": "string", "description": "the connection's slug, as connections lists it"},
		"candidates": map[string]any{"type": "boolean", "description": "list the tools that can be a trigger (nothing is called)"},
		"recipe": map[string]any{"type": "object", "description": "{trigger: \"message.new\", tool, args: {… \"{{cursor}}\" (or \"{{cursor:number}}\" for a number) where it takes since/after …}, items: dotted path to the list (an object of id → thing works too), id, at, title, actor, url, thread: dotted paths of a thing's fields, cursor: the time or id field, every: \"10m\", filter: {path: text it must contain}, mode: digest|notify|agent, via: the gateway object a candidate gives, watch: [fields whose change on a thing seen is an event item.changed «was → now», e.g. status], facts: [up to 8 fields put into the event's facts for rules], next: {path: the next page's cursor in the answer, arg: the argument it goes in} (at most 5 pages a poll), path + query: an API connection's GET instead of a tool (\"{{cursor}}\" in them), script: a JS function (answer, memo) => ({items, memo}) that reshapes each answer in a sandbox with no network, files or timers (200 ms)}",
			"required": []string{"trigger", "id"}},
		"replace":  map[string]any{"type": "string", "description": "an existing trigger's connection id: the recipe becomes its new version (the person confirms it, a changed script above all)"},
		"name":     map[string]any{"type": "string", "description": "the person's name for the trigger"},
		"unmarked": map[string]any{"type": "boolean", "description": "the tool is not marked read-only but the person says it only reads"},
		"why":      map[string]any{"type": "string", "description": "one line for the card: what the person asked for"},
		"dry_run":  map[string]any{"type": "boolean", "description": "only run the recipe once and show what it finds"},
		"proposal": map[string]any{"type": "string", "description": "the id a call that said \"waiting\" gave: keep waiting for the person's yes"}}}}

func (h *Hub) triggerAdd(ctx context.Context, raw json.RawMessage) map[string]any {
	if h.Triggers == nil || h.Confirm == nil {
		return textResult("trigger_add is not available here", true)
	}
	var a struct {
		Connection string          `json:"connection"`
		Candidates bool            `json:"candidates"`
		Recipe     json.RawMessage `json:"recipe"`
		Name       string          `json:"name"`
		Unmarked   bool            `json:"unmarked"`
		Why        string          `json:"why"`
		DryRun     bool            `json:"dry_run"`
		Proposal   string          `json:"proposal"`
		Replace    string          `json:"replace"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return textResult("the arguments are not what trigger_add takes: "+err.Error(), true)
	}
	if a.Proposal != "" {
		ans, err := h.Confirm(ctx, ConfirmRequest{ProposalID: a.Proposal}, h.confirmWait())
		if err != nil {
			return textResult("that request is gone (answered, or out of time): call trigger_add again", true)
		}
		return h.answeredAs("trigger_add", ans)
	}
	srv, ok := h.serverBySlug(a.Connection)
	if !ok {
		return textResult(fmt.Sprintf("no connection %q: connections lists them", a.Connection), true)
	}
	if a.Candidates {
		cands, err := h.S.Candidates(ctx, srv.ID)
		if err != nil {
			return textResult("could not read its tools: "+Reason(err), true)
		}
		return textResult(candidatesText(cands), false)
	}
	if len(a.Recipe) == 0 {
		return textResult("give recipe (trigger_add candidates true lists the tools that fit)", true)
	}
	plan, err := h.Triggers(ctx, srv.Slug, a.Recipe, a.Unmarked, a.Name, a.Replace)
	if err != nil {
		return textResult("the recipe does not run: "+err.Error(), true)
	}
	if a.DryRun {
		return textResult("Would make (nothing was made):\n"+pairsText(plan.Pairs)+plan.Found, false)
	}
	title, titleRu := "Make a trigger", "Сделать триггер"
	if a.Replace != "" {
		title, titleRu = "A new version of a trigger", "Новая версия триггера"
	}
	req := ConfirmRequest{Title: title, TitleRu: titleRu, Name: plan.Name, Pairs: plan.Pairs, Why: a.Why, Run: plan.Save}
	ans, err := h.Confirm(ctx, req, h.confirmWait())
	if err != nil {
		return textResult("could not ask the person: "+err.Error(), true)
	}
	return h.answeredAs("trigger_add", ans)
}

func (h *Hub) serverBySlug(slug string) (Server, bool) {
	list, err := h.S.List()
	if err != nil {
		return Server{}, false
	}
	for _, s := range list {
		if s.Owner == "" && (s.Slug == slug || s.ID == slug) {
			return s, true
		}
	}
	return Server{}, false
}

func pairsText(pairs []Pair) string {
	var b strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Value)
	}
	return b.String()
}

// candidatesText is the classifier's verdicts for an agent: those that fit first, with what to fill.
func candidatesText(cands []Candidate) string {
	var b strings.Builder
	n := 0
	for _, c := range cands {
		if !c.Fits {
			continue
		}
		n++
		fmt.Fprintf(&b, "- %s — %s (score %d; + %s", c.Tool, c.Label, c.Score, strings.Join(c.Plus, "; "))
		if len(c.Minus) > 0 {
			fmt.Fprintf(&b, "; − %s", strings.Join(c.Minus, "; "))
		}
		b.WriteString(")")
		var need, cursor []string
		for _, a := range c.Args {
			if a.Cursor {
				t := "{{cursor}}"
				if a.Type == "integer" || a.Type == "number" {
					t = "{{cursor:number}}"
				}
				cursor = append(cursor, a.Name+": \""+t+"\"")
			} else if a.Required {
				need = append(need, a.Name)
			}
		}
		if len(cursor) > 0 {
			b.WriteString(" args " + strings.Join(cursor, ", "))
		}
		if len(need) > 0 {
			b.WriteString("; fill in: " + strings.Join(need, ", "))
		}
		if c.Gateway != nil {
			via, _ := json.Marshal(c.Gateway)
			b.WriteString("; via " + string(via))
		}
		if !c.ReadOnly {
			b.WriteString("; not marked read-only: unmarked true only if the person says it only reads")
		}
		b.WriteString("\n")
	}
	if n == 0 {
		return "No tool of this connection can be a trigger: none lists things and only reads.\n"
	}
	return fmt.Sprintf("%d tool(s) can be a trigger. Call one once (call) to see its answer, write the recipe, then trigger_add with dry_run.\n", n) + b.String()
}
