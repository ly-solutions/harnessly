package main

// The agents' own MCP servers into Settings → Connections (docs/mcp-connections.md «The agents' servers"):
// at start, then once a minute when one of the agents' config files changed (or a key the person was asked for may
// have come), and on the page's «Rescan» (POST <platform>/mcp/sync), the runtime reads Claude Code's and Codex's
// configs and makes each server a connection (mcpconn.SyncAgents). It never changes the agents' files. Every outcome
// goes to the developer mode's trace (connectors.TraceSyncSeen); a key the config only refers to is asked in the safe.

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/locale"
	"github.com/ly-solutions/harnessly/internal/mcpconn"
	"github.com/ly-solutions/harnessly/internal/secrets"
)

// agentSyncEvery is how often the agents' files are looked at.
const agentSyncEvery = time.Minute

// ownMCPNames are the names harnessly gives its own servers in an agent's config; the runtime's own MCP config
// (preview-mcp.json) adds the names it gives there.
var ownMCPNames = []string{mcpHubName, "harnessly", "harnessly-preview"}

// ownMCPConfigNames are the server names of the runtime's own MCP config.
func ownMCPConfigNames(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		out = append(out, name)
	}
	return out
}

type agentSyncer struct {
	d   deps
	ms  *mcpconn.Service
	ask func(mcpconn.AskKey)
	env *agentEnv
	// reach probes a loopback address; nil — mcpconn's probe.
	reach func(ctx context.Context, url string) error

	mu      sync.Mutex
	files   string // the signature of the files the last sync read
	last    mcpconn.SyncReport
	lastAt  time.Time
	traced  string
	checked map[string]bool
}

var (
	agentSyncersMu sync.Mutex
	agentSyncers   = map[string]*agentSyncer{}
)

// agentSyncFor is the runtime's one syncer; it starts its loop the first time.
func agentSyncFor(d deps) *agentSyncer {
	agentSyncersMu.Lock()
	defer agentSyncersMu.Unlock()
	if s := agentSyncers[d.DataDir]; s != nil {
		return s
	}
	sf := safeFor(d)
	s := &agentSyncer{d: d, ms: mcpFor(d), env: newAgentEnv(d.Home), checked: map[string]bool{}, ask: func(k mcpconn.AskKey) {
		why := locale.Pick("The key of "+k.Server+" ("+k.Key+"): the agent's config refers to it, harnessly needs its value",
			"Ключ "+k.Server+" ("+k.Key+"): в конфиге агента на него ссылка, harnessly нужно значение")
		_, _, err := sf.asks.OpenScoped(secrets.Ask{Name: k.Secret, Why: why, Title: locale.Pick("Connections", "Подключения")})
		if err != nil {
			agent.Log("info", "mcp sync: key not asked", agent.F{K: "secret", V: k.Secret}, agent.F{K: "error", V: err.Error()})
		}
	}}
	agentSyncers[d.DataDir] = s
	go s.loop(context.Background())
	return s
}

func (s *agentSyncer) loop(ctx context.Context) {
	_, _ = s.Run(ctx, false) // a failure is logged and traced by Run
	t := time.NewTicker(agentSyncEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.Run(ctx, false)
		}
	}
}

func (s *agentSyncer) scanOptions() mcpconn.ScanOptions {
	own := filepath.Join(s.d.DataDir, "preview-mcp.json")
	base, _, _ := runtimeMCPEndpoint(own)
	getenv := os.LookupEnv
	if s.env != nil {
		getenv = s.env.Getenv
	}
	return mcpconn.ScanOptions{Home: s.d.Home, Getenv: getenv, Own: ownMCP(base, ownMCPConfigNames(own)), Reach: s.reachable}
}

// reachLimit is how long a loopback address has to answer MCP: a local bridge may start its upstream on the first call.
const reachLimit = 20 * time.Second

// reachable says why a server on this machine's loopback is not taken: nothing answers there, or not MCP — a port a
// default pointed at may be an old process, or someone else's.
func (s *agentSyncer) reachable(f mcpconn.Found) string {
	ctx, cancel := context.WithTimeout(context.Background(), reachLimit)
	defer cancel()
	probe := s.reach
	if probe == nil {
		probe = func(ctx context.Context, u string) error { _, err := s.ms.Probe(ctx, u); return err }
	}
	if err := probe(ctx, f.URL); err != nil {
		why := "nothing on this machine answers MCP at " + f.URL + " (" + mcpconn.Reason(err) + ")"
		if len(f.Defaulted) > 0 {
			why += "; the address was expanded by default: $" + strings.Join(f.Defaulted, ", $") + " is not in the agent's environment"
		}
		return why
	}
	return ""
}

// ownMCP says why a server is harnessly's own: the runtime's address, its proxy or hub, its own names and commands.
func ownMCP(runtimeBase string, names []string) func(mcpconn.Found) string {
	names = append(slices.Clone(ownMCPNames), names...)
	var host string
	if u, err := url.Parse(runtimeBase); err == nil {
		host = u.Host
	}
	return func(f mcpconn.Found) string {
		if slices.Contains(names, f.Name) {
			return "harnessly's own server (" + f.Name + ")"
		}
		if f.URL != "" {
			u, err := url.Parse(f.URL)
			if err == nil && host != "" && strings.EqualFold(u.Host, host) {
				return "points at this runtime"
			}
			if err == nil && (strings.HasPrefix(u.Path, "/api/cli/") || strings.HasPrefix(u.Path, "/agents/api/cli/")) {
				return "a harnessly runtime's proxy"
			}
		}
		if len(f.Command) > 0 {
			switch filepath.Base(f.Command[0]) {
			case "harnessly", "harnessly-runtime":
				return "a harnessly command"
			}
		}
		return ""
	}
}

// signature is what the agents' files look like now: their paths, sizes and times.
func signature(home string) string {
	paths := []string{filepath.Join(home, ".claude.json"), filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), filepath.Join(home, ".codex", "config.toml")}
	if v := os.Getenv("CODEX_HOME"); v != "" {
		paths = append(paths, filepath.Join(v, "config.toml"))
	}
	var b strings.Builder
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			b.WriteString(p + ":" + fi.ModTime().UTC().Format(time.RFC3339Nano) + ":" + strconv.FormatInt(fi.Size(), 10) + ";")
		}
	}
	return b.String()
}

// Run syncs when the files changed, when a key is awaited, or always when forced (the page's «Rescan»).
func (s *agentSyncer) Run(ctx context.Context, force bool) (mcpconn.SyncReport, error) {
	if strings.TrimSpace(s.d.Home) == "" {
		return mcpconn.SyncReport{}, nil
	}
	sig := signature(s.d.Home)
	if s.env != nil {
		sig += s.env.signature() // the agent's environment may give a variable another value
	}
	s.mu.Lock()
	waiting := s.awaitsKeys()
	if !force && sig == s.files && !waiting {
		rep := s.last
		s.mu.Unlock()
		return rep, nil
	}
	found, skipped := mcpconn.ScanAgents(s.scanOptions())
	// A local server that did not answer now is tried again at the next tick, not only when a file changes.
	if slices.ContainsFunc(skipped, func(x mcpconn.Skipped) bool { return x.Key != "" }) {
		sig = "retry"
	}
	rep, err := s.ms.SyncAgents(found, skipped)
	if err != nil {
		s.mu.Unlock()
		agent.Log("warn", "mcp sync: failed", agent.F{K: "error", V: err.Error()})
		connectors.Trace.Add(connectors.TraceSync, "sync", connectors.TraceError, "sync failed: "+err.Error(), nil)
		return rep, err
	}
	s.files, s.last, s.lastAt = sig, rep, time.Now()
	var toCheck []string
	for _, id := range rep.New {
		if !s.checked[id] {
			s.checked[id] = true
			toCheck = append(toCheck, id)
		}
	}
	s.mu.Unlock()
	for _, k := range rep.Ask {
		s.ask(k)
	}
	for _, o := range rep.Outcomes {
		if o.Outcome == mcpconn.SyncKeyFilled {
			toCheck = append(toCheck, o.ID)
		}
	}
	s.trace(rep, force)
	if len(rep.New) > 0 {
		agent.Log("info", "mcp sync: servers taken from the agents", agent.F{K: "new", V: len(rep.New)})
	}
	if len(toCheck) > 0 {
		go s.check(toCheck)
	}
	return rep, nil
}

// awaitsKeys says whether a connection waits for a key the person was asked for (the safe may have it now).
func (s *agentSyncer) awaitsKeys() bool {
	list, err := s.ms.List()
	if err != nil {
		return false
	}
	return slices.ContainsFunc(list, func(x mcpconn.Server) bool { return x.Agent != nil && len(x.Agent.Missing) > 0 })
}

// check tries the new connections one after another: their state and tools on the page.
func (s *agentSyncer) check(ids []string) {
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		_, err := s.ms.Check(ctx, id)
		cancel()
		if err != nil {
			agent.Log("info", "mcp sync: check failed", agent.F{K: "id", V: id}, agent.F{K: "reason", V: mcpconn.Reason(err)})
		}
	}
}

// trace writes the outcomes to the developer mode's trace when they differ from the last written (or when asked).
func (s *agentSyncer) trace(rep mcpconn.SyncReport, force bool) {
	if !connectors.Trace.On() {
		return
	}
	items := make([]connectors.SyncItem, 0, len(rep.Outcomes))
	var key strings.Builder
	for _, o := range rep.Outcomes {
		taken := o.Outcome == mcpconn.SyncTaken || o.Outcome == mcpconn.SyncNeedsKey || o.Outcome == mcpconn.SyncKeyFilled || o.Outcome == mcpconn.SyncAlready
		items = append(items, connectors.SyncItem{ID: o.ID, Name: o.Name, Kind: o.Outcome, From: o.From, Taken: taken, Why: o.Why})
		key.WriteString(o.Name + "|" + o.From + "|" + o.Outcome + "|" + o.Why + ";")
	}
	s.mu.Lock()
	same := key.String() == s.traced
	s.traced = key.String()
	s.mu.Unlock()
	if same && !force {
		return
	}
	connectors.TraceSyncSeen("agents' configs", items)
}

// agentSyncerOf is the runtime's syncer when it runs; nil before.
func agentSyncerOf(d deps) *agentSyncer {
	agentSyncersMu.Lock()
	defer agentSyncersMu.Unlock()
	return agentSyncers[d.DataDir]
}

// Replay writes the last sync to the trace again (the developer mode was turned on after it).
func (s *agentSyncer) Replay() {
	s.mu.Lock()
	rep := s.last
	s.mu.Unlock()
	s.trace(rep, true)
}
