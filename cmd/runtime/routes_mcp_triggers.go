package main

// Triggers of an MCP connection (docs/connectors.md «MCP recipe»): which of its tools can be a trigger (a
// classifier, no model, nothing called), a draft recipe of one (its tool called once, only when asked), a check of a
// recipe (run once, the newest things as they will be events), and the trigger made: a connection of the connector
// mcp-recipe. The page's session only; an agent makes a trigger through the hub's trigger_add, which the person
// confirms on a card.
//
//	GET  <platform>/mcp/<id>/triggers         {candidates}
//	POST <platform>/mcp/<id>/triggers/draft   {tool, via, description, args, cursorArg, cursorNumber, unmarked} → {recipe, samples}
//	POST <platform>/mcp/<id>/triggers/try     {recipe, unmarked} → {samples}
//	POST <platform>/mcp/<id>/triggers         {recipe, unmarked, name, replace} → {connection} (replace: a new version)

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/connectors/mcprecipe"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
)

// saveTrigger makes the trigger: a connection of the connector mcp-recipe whose check runs the recipe once more. With
// replace it is a new version of that trigger: its recipe changes, its cursor and memo stay.
func saveTrigger(ctx context.Context, d deps, slug string, r mcprecipe.Recipe, unmarked bool, name, replace string) (connectors.Conn, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return connectors.Conn{}, err
	}
	allow := "no"
	if unmarked {
		allow = "yes"
	}
	values := map[string]string{"server": slug, "recipe": string(raw), "unmarked": allow}
	if replace != "" {
		return connectorsFor(d).Reconfigure(ctx, replace, values)
	}
	if strings.TrimSpace(name) == "" {
		name = slug + " · " + r.Trigger
	}
	return connectorsFor(d).Connect(ctx, mcprecipe.ID, name, values)
}

// nextVersion is the recipe as the new version of the trigger replace: its version one more than the kept one's, and
// what changed for the card — the script above all, which only the person's yes lets run.
func nextVersion(d deps, slug string, r mcprecipe.Recipe, replace string) (mcprecipe.Recipe, []mcpconn.Pair, error) {
	if replace == "" {
		if r.Version == 0 {
			r.Version = 1
		}
		var pairs []mcpconn.Pair
		if r.Script != "" {
			pairs = append(pairs, mcpconn.Pair{Name: "Script", Value: fmt.Sprintf("%d bytes, %s", len(r.Script), scriptHash(r.Script))})
		}
		return r, pairs, nil
	}
	c, err := connectorsFor(d).Get(replace)
	if err != nil || c.Connector != mcprecipe.ID {
		return r, nil, fmt.Errorf("no trigger %q to replace", replace)
	}
	if c.Config["server"] != slug {
		return r, nil, fmt.Errorf("the trigger %q polls %s, not %s", replace, c.Config["server"], slug)
	}
	var old mcprecipe.Recipe
	_ = json.Unmarshal([]byte(c.Config["recipe"]), &old)
	r.Version = max(old.Version, 1) + 1
	pairs := []mcpconn.Pair{{Name: "Trigger replaced", Value: c.Name}, {Name: "Version", Value: fmt.Sprintf("%d → %d", max(old.Version, 1), r.Version)}}
	switch {
	case old.Script != r.Script && r.Script == "":
		pairs = append(pairs, mcpconn.Pair{Name: "Script", Value: "removed"})
	case old.Script != r.Script:
		pairs = append(pairs, mcpconn.Pair{Name: "Script", Value: fmt.Sprintf("changed: %s → %s", scriptHash(old.Script), scriptHash(r.Script))})
	}
	return r, pairs, nil
}

func scriptHash(s string) string {
	if s == "" {
		return "none"
	}
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:6])
}

func samplesText(samples []mcprecipe.Sample) string {
	if len(samples) == 0 {
		return "The dry run found nothing now: events come with the next new things.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The newest things, as they will be events:\n")
	for _, s := range samples {
		line := "- " + s.ID
		if !s.At.IsZero() {
			line += " · " + s.At.UTC().Format("2006-01-02 15:04")
		}
		if s.Actor != "" {
			line += " · " + s.Actor
		}
		if s.Title != "" {
			line += " · «" + s.Title + "»"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// triggerPlanner is trigger_add's: the recipe is run once, the card says what will be made, the yes makes it.
func triggerPlanner(d deps) mcpconn.TriggerPlanner {
	return func(ctx context.Context, slug string, raw json.RawMessage, unmarked bool, name, replace string) (mcpconn.TriggerPlan, error) {
		var r mcprecipe.Recipe
		if err := json.Unmarshal(raw, &r); err != nil {
			return mcpconn.TriggerPlan{}, fmt.Errorf("the recipe is not JSON of a recipe: %v", err)
		}
		r, versionPairs, err := nextVersion(d, slug, r, replace)
		if err != nil {
			return mcpconn.TriggerPlan{}, err
		}
		samples, err := mcprecipe.Try(ctx, slug, r, unmarked)
		if err != nil {
			return mcpconn.TriggerPlan{}, fmt.Errorf("%s", connectors.Say(err))
		}
		mode := r.Mode
		if mode == "" {
			mode = "digest"
		}
		every := r.Every
		if every == "" {
			every = "10m"
		}
		tool := r.Tool
		if r.Via != nil {
			tool += " (through " + r.Via.Call + ")"
		}
		if r.Path != "" {
			tool = "GET " + r.Path
		}
		if name == "" {
			name = slug + " · " + r.Trigger
		}
		pairs := []mcpconn.Pair{{Name: "Connection", Value: slug}, {Name: "Tool", Value: tool}, {Name: "Trigger", Value: r.Trigger},
			{Name: "Every", Value: every}, {Name: "Mode", Value: mode}, {Name: "Found now", Value: fmt.Sprintf("%d", len(samples))}}
		if len(samples) > 0 && samples[0].Title != "" {
			pairs = append(pairs, mcpconn.Pair{Name: "Newest", Value: samples[0].Title})
		}
		if unmarked {
			pairs = append(pairs, mcpconn.Pair{Name: "Not marked read-only", Value: "allowed"})
		}
		if len(r.Watch) > 0 {
			pairs = append(pairs, mcpconn.Pair{Name: "Watch", Value: strings.Join(r.Watch, ", ")})
		}
		pairs = append(pairs, versionPairs...)
		return mcpconn.TriggerPlan{Name: name, Pairs: pairs, Found: samplesText(samples),
			Save: func(ctx context.Context) (string, error) {
				c, err := saveTrigger(ctx, d, slug, r, unmarked, name, replace)
				if err != nil {
					return "", fmt.Errorf("%s", connectors.Say(err))
				}
				agent.Log("info", "mcp: a trigger made", agent.F{K: "connection", V: c.ID}, agent.F{K: "server", V: slug})
				if replace != "" {
					return fmt.Sprintf("The trigger %q (connection %s) runs version %d now.", c.Name, c.ID, r.Version), nil
				}
				return fmt.Sprintf("Made the trigger %q (connection %s): every new thing of %s is an event of the journal, mode %s; the person changes it on /events → Triggers.", name, c.ID, r.Tool, mode), nil
			}}, nil
	}
}

// mcpTriggersRoute is <platform>/mcp/<id>/triggers[/draft|/try].
func mcpTriggersRoute(w http.ResponseWriter, r *http.Request, d deps, ms *mcpconn.Service, id, sub string, ctxOf func(*http.Request) (context.Context, context.CancelFunc)) {
	srv, err := ms.Get(id)
	if err != nil {
		mcpRefusal(w, err, nil)
		return
	}
	ctx, cancel := ctxOf(r)
	defer cancel()
	said := func(err error) {
		connJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "said", "message": connectors.Say(err)})
	}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		cands, err := ms.Candidates(ctx, id)
		if err != nil {
			mcpRefusal(w, err, nil)
			return
		}
		traceCandidates(srv, cands)
		connJSON(w, http.StatusOK, map[string]any{"candidates": cands})
	case sub == "draft" && r.Method == http.MethodPost:
		var in mcprecipe.DraftInput
		if !readSecretBody(r, &in) || in.Tool == "" {
			connJSON(w, http.StatusBadRequest, map[string]string{"error": "bad-request"})
			return
		}
		in.Server = srv.Slug
		recipe, samples, err := mcprecipe.Draft(ctx, in)
		if err != nil {
			said(err)
			return
		}
		connJSON(w, http.StatusOK, map[string]any{"recipe": recipe, "samples": nonNilSamples(samples)})
	case sub == "try" && r.Method == http.MethodPost:
		var b struct {
			Recipe   mcprecipe.Recipe `json:"recipe"`
			Unmarked bool             `json:"unmarked"`
		}
		if !readSecretBody(r, &b) {
			connJSON(w, http.StatusBadRequest, map[string]string{"error": "bad-request"})
			return
		}
		samples, err := mcprecipe.Try(ctx, srv.Slug, b.Recipe, b.Unmarked)
		if err != nil {
			said(err)
			return
		}
		connJSON(w, http.StatusOK, map[string]any{"samples": nonNilSamples(samples)})
	case sub == "" && r.Method == http.MethodPost:
		var b struct {
			Recipe   mcprecipe.Recipe `json:"recipe"`
			Unmarked bool             `json:"unmarked"`
			Name     string           `json:"name"`
			// Replace makes it a new version of that trigger: the person's own form is the confirmation.
			Replace string `json:"replace"`
		}
		if !readSecretBody(r, &b) {
			connJSON(w, http.StatusBadRequest, map[string]string{"error": "bad-request"})
			return
		}
		recipe, _, err := nextVersion(d, srv.Slug, b.Recipe, b.Replace)
		if err != nil {
			said(err)
			return
		}
		c, err := saveTrigger(ctx, d, srv.Slug, recipe, b.Unmarked, b.Name, b.Replace)
		if err != nil {
			said(err)
			return
		}
		agent.Log("info", "mcp: a trigger made on the page", agent.F{K: "connection", V: c.ID}, agent.F{K: "server", V: srv.Slug})
		connJSON(w, http.StatusOK, map[string]any{"connection": c})
	default:
		connJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
	}
}

func nonNilSamples(s []mcprecipe.Sample) []mcprecipe.Sample {
	if s == nil {
		return []mcprecipe.Sample{}
	}
	return s
}

// traceCandidates writes the classifier's reasons to the developer mode's trace, under the MCP connection.
func traceCandidates(srv mcpconn.Server, cands []mcpconn.Candidate) {
	if !connectors.Trace.On() {
		return
	}
	fits := 0
	for _, c := range cands {
		if c.Fits {
			fits++
		}
	}
	connectors.Trace.Add("mcp:"+srv.Slug, "classify", connectors.TraceInfo,
		fmt.Sprintf("%s: %d of %d tools can be a trigger", srv.Name, fits, len(cands)), cands)
}
