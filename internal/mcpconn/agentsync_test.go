package mcpconn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func remoteEntry(t *testing.T) CatalogEntry {
	for _, e := range catalog {
		if e.Transport == HTTP && e.URL != "" {
			return e
		}
	}
	t.Fatal("no remote entry")
	return CatalogEntry{}
}

// agentHome is a home folder with the three kinds of config: ~/.claude.json, a plugin (two versions, one active, one
// turned-off plugin), a project's .mcp.json and Codex's config.toml.
func agentHome(t *testing.T) (string, CatalogEntry) {
	home := t.TempDir()
	cat := remoteEntry(t)
	proj := filepath.Join(home, "work", "app")
	write(t, filepath.Join(home, ".claude.json"), `{
		"mcpServers": {
			"cat": {"type": "http", "url": "`+cat.URL+`"},
			"hub": {"type": "http", "url": "http://127.0.0.1:8022/api/cli/mcp-hub", "headers": {"X-Cli-Token": "own"}},
			"broken": {"type": "http"}
		},
		"projects": {"`+proj+`": {"mcpServers": {"gh": {"command": "npx", "args": ["-y", "gh-mcp@1.0.0"],
			"env": {"GH_TOKEN": "ghp_literal_secret_value", "GH_HOST": "${GH_HOST}"}}}}}
	}`)
	write(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers": {"wiki": {"type": "sse", "url": "https://wiki.example.com/sse",
		"headers": {"Authorization": "Bearer ${WIKI_TOKEN}"}}}}`)
	cache := filepath.Join(home, ".claude", "plugins", "cache", "mk")
	write(t, filepath.Join(cache, "infra", "1.0.0", ".mcp.json"), `{"old": {"type": "http", "url": "https://old.example.com/mcp"}}`)
	write(t, filepath.Join(cache, "infra", "1.1.0", ".mcp.json"), `{
		"crm": {"type": "http", "url": "http://127.0.0.1:${BRIDGE_PORT:-17380}/crm/mcp"},
		"tool": {"command": "${CLAUDE_PLUGIN_ROOT}/bin/tool", "args": ["--x"]}
	}`)
	write(t, filepath.Join(cache, "off", "1.0.0", ".mcp.json"), `{"nope": {"type": "http", "url": "https://nope.example.com/mcp"}}`)
	write(t, filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), `{"version": 2, "plugins": {
		"infra@mk": [{"scope": "user", "installPath": "`+filepath.Join(cache, "infra", "1.1.0")+`", "version": "1.1.0"}],
		"off@mk": [{"scope": "user", "installPath": "`+filepath.Join(cache, "off", "1.0.0")+`"}]}}`)
	write(t, filepath.Join(home, ".claude", "settings.json"), `{"enabledPlugins": {"off@mk": false}}`)
	write(t, filepath.Join(home, ".codex", "config.toml"), `
[mcp_servers.crm]
url = "http://127.0.0.1:17380/crm/mcp"

[mcp_servers.search]
url = "https://search.example.com/mcp"
bearer_token_env_var = "SEARCH_KEY"

[mcp_servers.fs]
command = "uvx"
args = ["fs-mcp==2.0"]
env = { ROOT = "/data" }

[mcp_servers.disabled]
url = "https://disabled.example.com/mcp"
enabled = false
`)
	return home, cat
}

func scan(home string) ([]Found, []Skipped) {
	return ScanAgents(ScanOptions{Home: home, Getenv: func(string) (string, bool) { return "", false },
		Own: func(f Found) string {
			if strings.Contains(f.URL, "/api/cli/") {
				return "harnessly's own"
			}
			return ""
		}})
}

func TestScanAgentsReadsTheThreeFormats(t *testing.T) {
	home, cat := agentHome(t)
	found, skipped := scan(home)
	by := map[string]Found{}
	for _, f := range found {
		by[f.From+"/"+f.Name] = f
	}
	proj := filepath.Join(home, "work", "app")
	cases := map[string]func(Found) bool{
		"claude user/cat": func(f Found) bool {
			return f.Transport == HTTP && strings.TrimRight(f.URL, "/") == strings.TrimRight(cat.URL, "/")
		},
		"project " + proj + "/gh": func(f Found) bool {
			return f.Transport == Stdio && strings.Join(f.Command, " ") == "npx -y gh-mcp@1.0.0" && f.Env["GH_HOST"] == "${GH_HOST}"
		},
		"project " + proj + "/wiki": func(f Found) bool { return f.Transport == SSE && f.Headers["Authorization"] == "Bearer ${WIKI_TOKEN}" },
		"plugin infra/crm":          func(f Found) bool { return f.URL == "http://127.0.0.1:17380/crm/mcp" },
		"plugin infra/tool": func(f Found) bool {
			return f.Command[0] == filepath.Join(home, ".claude", "plugins", "cache", "mk", "infra", "1.1.0")+"/bin/tool"
		},
		"codex/crm":    func(f Found) bool { return f.URL == "http://127.0.0.1:17380/crm/mcp" },
		"codex/search": func(f Found) bool { return f.Headers["Authorization"] == "Bearer ${SEARCH_KEY}" },
		"codex/fs":     func(f Found) bool { return f.Transport == Stdio && f.Env["ROOT"] == "/data" },
	}
	for k, ok := range cases {
		f, has := by[k]
		if !has || !ok(f) {
			t.Errorf("%s: %+v (has %v)", k, f, has)
		}
	}
	for _, f := range found {
		if f.Name == "old" || f.Name == "nope" || f.Name == "hub" || f.Name == "disabled" {
			t.Errorf("taken, should not be: %s %s", f.From, f.Name)
		}
	}
	why := map[string]string{}
	for _, s := range skipped {
		why[s.Name] = s.Why
	}
	if why["hub"] != "harnessly's own" || why["broken"] == "" || why["disabled"] == "" {
		t.Errorf("skipped: %+v", skipped)
	}
}

func TestSyncAgentsMakesConnectionsOnceKeepsKeysInTheSafe(t *testing.T) {
	home, cat := agentHome(t)
	sec := newSecrets()
	s := New(t.TempDir(), sec, nil)
	found, skipped := scan(home)
	rep, err := s.SyncAgents(found, skipped)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := s.List()
	// cat, gh, wiki, crm (plugin and Codex: one), tool, search, fs.
	if len(list) != 7 {
		names := []string{}
		for _, x := range list {
			names = append(names, x.Name)
		}
		t.Fatalf("connections: %d %v", len(list), names)
	}
	get := func(name string) Server {
		for _, x := range list {
			for _, n := range x.Agent.Names {
				if n == name {
					return x
				}
			}
		}
		t.Fatalf("no %s", name)
		return Server{}
	}
	if c := get("cat"); c.CatalogID != cat.ID || c.Custom != nil {
		t.Errorf("catalog match: %+v", c)
	}
	crm := get("crm")
	if len(crm.Agent.From) != 2 || !crm.HasAgent("claude") || !crm.HasAgent("codex") || !strings.HasPrefix(crm.CatalogID, CustomPrefix) {
		t.Errorf("crm: %+v", crm.Agent)
	}
	if search := get("search"); search.HasAgent("claude") || !search.HasAgent("codex") {
		t.Errorf("search is Codex's alone: %+v", search.Agent)
	}
	gh := get("gh")
	if gh.State != StateSignIn || len(gh.Agent.Missing) != 1 || gh.Agent.Missing[0].Key != "GH_HOST" {
		t.Errorf("gh: %+v %+v", gh, gh.Agent)
	}
	wiki := get("wiki")
	if wiki.Auth != AuthHeader || wiki.HeaderName != "Authorization" || len(wiki.Agent.Missing) != 1 {
		t.Errorf("wiki: %+v", wiki)
	}
	// The literal key is in the safe, never in the file.
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(s.file), "mcp-connections.json"))
	if strings.Contains(string(raw), "ghp_literal_secret_value") {
		t.Fatal("a key is in mcp-connections.json")
	}
	if c, err := s.readCred(gh.ID); err != nil || c.Env["GH_TOKEN"] != "ghp_literal_secret_value" {
		t.Fatalf("safe: %+v %v", c, err)
	}
	asks := map[string]bool{}
	for _, a := range rep.Ask {
		asks[a.Secret] = true
	}
	if !asks[wiki.Agent.Missing[0].Secret] || !asks[gh.Agent.Missing[0].Secret] || len(rep.New) != 7 {
		t.Fatalf("asks %v new %v", rep.Ask, rep.New)
	}

	// The person renames one; a rescan neither doubles nor undoes it.
	if _, err := s.Rename(crm.ID, "Our CRM"); err != nil {
		t.Fatal(err)
	}
	rep2, err := s.SyncAgents(scan(home))
	if err != nil {
		t.Fatal(err)
	}
	list2, _ := s.List()
	if len(list2) != 7 || len(rep2.New) != 0 {
		t.Fatalf("rescan made new: %d %v", len(list2), rep2.New)
	}
	for _, x := range list2 {
		if x.ID == crm.ID && x.Name != "Our CRM" {
			t.Fatalf("rename undone: %s", x.Name)
		}
	}

	// The key the person put in the safe is taken by the next sync.
	_ = sec.Put(wiki.Agent.Missing[0].Secret, "tok123")
	rep3, _ := s.SyncAgents(scan(home))
	if c, _ := s.readCred(wiki.ID); c.Header != "Bearer tok123" {
		t.Fatalf("filled header: %q", c.Header)
	}
	if !hasOutcome(rep3, "wiki", SyncKeyFilled) {
		t.Fatalf("outcomes: %+v", rep3.Outcomes)
	}

	// Gone from the agent: kept and marked. Removed by the person: not made again.
	write(t, filepath.Join(home, ".codex", "config.toml"), "")
	rep4, _ := s.SyncAgents(scan(home))
	if !hasOutcome(rep4, "search", SyncGone) {
		t.Fatalf("gone: %+v", rep4.Outcomes)
	}
	for _, x := range mustList(t, s) {
		if x.Agent != nil && x.Agent.Names[0] == "search" && (x.Agent.Present || x.Agent.GoneAt.IsZero()) {
			t.Fatalf("search not marked: %+v", x.Agent)
		}
	}
	if err := s.Remove(get("cat").ID); err != nil {
		t.Fatal(err)
	}
	rep5, _ := s.SyncAgents(scan(home))
	if len(rep5.New) != 0 || !hasOutcome(rep5, "cat", SyncSkipped) {
		t.Fatalf("removed came back: %+v", rep5)
	}
}

func TestSyncAgentsAdoptsTheServerThePersonConnected(t *testing.T) {
	s := New(t.TempDir(), newSecrets(), nil)
	if err := s.writeAll(stored{Servers: []Server{{ID: "m1", Name: "Mine", Slug: "mine", URL: "https://mine.example.com/mcp", Transport: HTTP, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	rep, err := s.SyncAgents([]Found{{Name: "m", From: "codex", Transport: HTTP, URL: "https://mine.example.com/mcp"}}, nil)
	if err != nil || len(rep.New) != 0 {
		t.Fatal(rep, err)
	}
	srv, _ := s.Get("m1")
	if srv.Agent == nil || !srv.Agent.Adopted || srv.Name != "Mine" || !srv.HasAgent("codex") {
		t.Fatalf("%+v", srv.Agent)
	}
}

func TestExpandValue(t *testing.T) {
	env := func(k string) (string, bool) {
		if k == "SET" {
			return "v", true
		}
		return "", false
	}
	if v, why, def := expandValue("a${SET}b$SET${UNSET:-d}", env); v != "avbvd" || why != "" || len(def) != 1 || def[0] != "UNSET" {
		t.Fatal(v, why, def)
	}
	if _, why, _ := expandValue("${UNSET}", env); !strings.Contains(why, "UNSET") {
		t.Fatal(why)
	}
	if !HasVarRef("Bearer ${X}") || HasVarRef("plain") || strings.Join(VarRefs("$A ${B} $A"), ",") != "A,B" {
		t.Fatal("refs")
	}
}

func hasOutcome(r SyncReport, name, outcome string) bool {
	for _, o := range r.Outcomes {
		if o.Name == name && o.Outcome == outcome {
			return true
		}
	}
	return false
}

func mustList(t *testing.T, s *Service) []Server {
	l, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// The port of a local bridge comes from the agent's environment; a default is marked; another value moves the
// connection the sync made, and a loopback port that does not answer MCP is not taken.
func TestSyncAgentsFollowsTheAgentsEnvironment(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".claude", "plugins", "cache", "mk", "infra", "1.0.0")
	write(t, filepath.Join(cache, ".mcp.json"), `{"crm": {"type": "http", "url": "http://127.0.0.1:${BRIDGE_PORT:-17380}/crm/mcp"}}`)
	write(t, filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), `{"version": 2, "plugins": {
		"infra@mk": [{"installPath": "`+cache+`"}]}}`)
	port := ""
	live := map[string]bool{"http://127.0.0.1:17380/crm/mcp": true, "http://127.0.0.1:18376/crm/mcp": true}
	opts := ScanOptions{Home: home,
		Getenv: func(k string) (string, bool) { return port, k == "BRIDGE_PORT" && port != "" },
		Reach: func(f Found) string {
			if !live[f.URL] {
				return "nothing answers"
			}
			return ""
		}}
	s := New(t.TempDir(), newSecrets(), nil)

	rep, err := s.SyncAgents(ScanAgents(opts))
	if err != nil || len(rep.New) != 1 {
		t.Fatal(rep, err)
	}
	if !strings.Contains(rep.Outcomes[0].Why, "expanded by default: $BRIDGE_PORT") {
		t.Fatalf("default not marked: %+v", rep.Outcomes)
	}
	id := rep.New[0]

	port = "18376"
	rep, _ = s.SyncAgents(ScanAgents(opts))
	srv, _ := s.Get(id)
	if srv.URL != "http://127.0.0.1:18376/crm/mcp" || len(mustList(t, s)) != 1 || !hasOutcome(rep, "crm", SyncMoved) {
		t.Fatalf("not moved: %s %+v", srv.URL, rep.Outcomes)
	}
	if strings.Contains(rep.Outcomes[0].Why, "by default") {
		t.Fatalf("a set variable marked as default: %s", rep.Outcomes[0].Why)
	}

	// The port the agent's environment names now does not answer: not taken, the connection stays as it was.
	port = "19999"
	rep, _ = s.SyncAgents(ScanAgents(opts))
	srv, _ = s.Get(id)
	if !hasOutcome(rep, "crm", SyncSkipped) || srv.URL != "http://127.0.0.1:18376/crm/mcp" || !srv.Agent.Present {
		t.Fatalf("unreachable: %s %+v %+v", srv.URL, srv.Agent, rep.Outcomes)
	}
}

// A connection made before the template was kept (on the default port) is moved too, not doubled.
func TestSyncAgentsMovesAConnectionOfTheOldFormat(t *testing.T) {
	s := New(t.TempDir(), newSecrets(), nil)
	old := Server{ID: "m1", Name: "crm", Slug: "crm", URL: "http://127.0.0.1:17380/crm/mcp", Transport: HTTP, Enabled: true,
		CatalogID: "custom-crm", Custom: &CustomMeta{Category: KindUtility},
		Agent: &AgentOrigin{Key: "url:http://127.0.0.1:17380/crm/mcp", Names: []string{"crm"}, From: []string{"plugin infra", "codex"}, Present: true}}
	if err := s.writeAll(stored{Servers: []Server{old}}); err != nil {
		t.Fatal(err)
	}
	rep, err := s.SyncAgents([]Found{{Name: "crm", From: "plugin infra", Transport: HTTP, URL: "http://127.0.0.1:18376/crm/mcp",
		Template: "http://127.0.0.1:${BRIDGE_PORT:-17380}/crm/mcp"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := s.Get("m1")
	if len(mustList(t, s)) != 1 || srv.URL != "http://127.0.0.1:18376/crm/mcp" || srv.Agent.Template == "" || !hasOutcome(rep, "crm", SyncMoved) {
		t.Fatalf("%+v %+v", srv, rep.Outcomes)
	}
}

// Codex writes the old port literally, the plugin's template expands to the agent's: one connection, at the agent's.
func TestSyncAgentsOneLocalServerTwoPorts(t *testing.T) {
	s := New(t.TempDir(), newSecrets(), nil)
	rep, err := s.SyncAgents([]Found{
		{Name: "crm", From: "codex", Transport: HTTP, URL: "http://127.0.0.1:17380/crm/mcp"},
		{Name: "crm", From: "plugin infra", Transport: HTTP, URL: "http://127.0.0.1:18376/crm/mcp", Template: "http://127.0.0.1:${P:-17380}/crm/mcp"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := mustList(t, s)
	if len(list) != 1 || list[0].URL != "http://127.0.0.1:18376/crm/mcp" || !list[0].HasAgent("codex") || !list[0].HasAgent("claude") {
		t.Fatalf("%+v", list)
	}
	if !strings.Contains(rep.Outcomes[0].Why, "codex writes http://127.0.0.1:17380/crm/mcp") {
		t.Fatalf("note: %+v", rep.Outcomes)
	}
}
