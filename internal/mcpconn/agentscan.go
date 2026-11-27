package mcpconn

// The agents' own MCP servers: what Claude Code and Codex are configured with outside harnessly — the
// person's ~/.claude.json (its root and its projects), the .mcp.json of the installed plugins (only the active version
// of each) and of the projects, and ~/.codex/config.toml. The scan only reads these files; SyncAgents makes each server
// a connection of mcp-connections.json. docs/mcp-connections.md, «The agents' servers».

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Found is one MCP server an agent is configured with.
type Found struct {
	// Name is the agent's name of the server (its key in the config).
	Name string
	// From says where it was found: "claude user", "project <path>", "plugin <name>", "codex".
	From      string
	Transport Transport
	URL       string
	Command   []string
	// Env and Headers are as the config writes them: a literal value, or a reference like ${VAR} or $VAR.
	Env     map[string]string
	Headers map[string]string
	// Template is the address or command line as the config writes it, when it has variables: a sync that expands it
	// to another value moves the connection it made, it does not make a second one.
	Template string
	// Defaulted are the variables the agent's environment does not have, whose ${VAR:-default} was used.
	Defaulted []string
}

// Skipped is a server the scan did not take, and why.
type Skipped struct {
	Name string `json:"name"`
	From string `json:"from"`
	Why  string `json:"why"`
	// Key and Template are set for a server that was read but not reachable now: its connection, if any, is not marked
	// gone for that.
	Key      string `json:"-"`
	Template string `json:"-"`
}

// ScanOptions say where the agents' files are and what is harnessly's own.
type ScanOptions struct {
	// Home is the person's home folder (~/.claude.json, ~/.claude/plugins, ~/.codex).
	Home string
	// CodexHome is Codex's folder; "" — Home/.codex (or $CODEX_HOME).
	CodexHome string
	// Getenv expands ${VAR} in an address or a command line (never in a key: a key reference asks for the key).
	Getenv func(string) (string, bool)
	// Own says why a server is harnessly's own (the hub, the runtime's proxy, its preview server); "" — it is not.
	Own func(f Found) string
	// Reach says why a server on this machine's loopback does not answer MCP ("" — it does); nil — not asked.
	Reach func(f Found) string
}

// ScanAgents reads the agents' configs. A file that is missing or unreadable is passed over; a server that cannot be
// used (no address, an address with an unset variable, harnessly's own) is in skipped with its reason.
func ScanAgents(o ScanOptions) (found []Found, skipped []Skipped) {
	if o.Getenv == nil {
		o.Getenv = os.LookupEnv
	}
	add := func(f Found, err string) {
		if err == "" && o.Own != nil {
			err = o.Own(f)
		}
		if err != "" {
			skipped = append(skipped, Skipped{Name: f.Name, From: f.From, Why: err})
			return
		}
		if o.Reach != nil && f.URL != "" && loopback(f.URL) {
			if why := o.Reach(f); why != "" {
				skipped = append(skipped, Skipped{Name: f.Name, From: f.From, Why: why, Key: agentKey(f), Template: f.Template})
				return
			}
		}
		found = append(found, f)
	}
	claudeJSON := filepath.Join(o.Home, ".claude.json")
	projects := map[string]bool{}
	if raw, err := os.ReadFile(claudeJSON); err == nil {
		var cfg struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
			Projects   map[string]struct {
				MCPServers map[string]json.RawMessage `json:"mcpServers"`
			} `json:"projects"`
		}
		if json.Unmarshal(raw, &cfg) == nil {
			for _, name := range sortedKeys(cfg.MCPServers) {
				add(claudeServer(name, "claude user", cfg.MCPServers[name], "", o.Getenv))
			}
			for _, dir := range sortedKeys(cfg.Projects) {
				projects[dir] = true
				p := cfg.Projects[dir]
				for _, name := range sortedKeys(p.MCPServers) {
					add(claudeServer(name, "project "+dir, p.MCPServers[name], "", o.Getenv))
				}
			}
		}
	}
	for _, dir := range sortedKeys(projects) {
		for _, s := range mcpJSON(filepath.Join(dir, ".mcp.json")) {
			add(claudeServer(s.name, "project "+dir, s.raw, "", o.Getenv))
		}
	}
	for _, p := range activePlugins(o.Home) {
		servers := mcpJSON(filepath.Join(p.path, ".mcp.json"))
		servers = append(servers, pluginManifestServers(p.path)...)
		for _, s := range servers {
			add(claudeServer(s.name, "plugin "+p.name, s.raw, p.path, o.Getenv))
		}
	}
	codexHome := o.CodexHome
	if codexHome == "" {
		if v, ok := o.Getenv("CODEX_HOME"); ok && v != "" {
			codexHome = v
		} else {
			codexHome = filepath.Join(o.Home, ".codex")
		}
	}
	for _, f := range codexServers(filepath.Join(codexHome, "config.toml"), o.Getenv) {
		add(f.Found, f.err)
	}
	return found, skipped
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type rawServer struct {
	name string
	raw  json.RawMessage
}

// mcpJSON reads a .mcp.json: {"mcpServers": {...}}, or the servers at its root as plugins may write them.
func mcpJSON(path string) []rawServer {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return serversOf(raw)
}

func serversOf(raw []byte) []rawServer {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return nil
	}
	if inner, ok := top["mcpServers"]; ok {
		top = nil
		if json.Unmarshal(inner, &top) != nil {
			return nil
		}
	}
	var out []rawServer
	for _, name := range sortedKeys(top) {
		if len(top[name]) > 0 && top[name][0] == '{' {
			out = append(out, rawServer{name, top[name]})
		}
	}
	return out
}

// pluginManifestServers are the servers a plugin's manifest declares inline (.claude-plugin/plugin.json mcpServers).
func pluginManifestServers(dir string) []rawServer {
	raw, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return nil
	}
	var m struct {
		MCPServers json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &m) != nil || len(m.MCPServers) == 0 || m.MCPServers[0] != '{' {
		return nil
	}
	return serversOf([]byte(`{"mcpServers":` + string(m.MCPServers) + `}`))
}

type plugin struct{ name, path string }

// activePlugins are the installed plugins at the version Claude Code runs (installed_plugins.json), less those the
// person turned off in ~/.claude/settings.json (enabledPlugins: false).
func activePlugins(home string) []plugin {
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"))
	if err != nil {
		return nil
	}
	var inst struct {
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if json.Unmarshal(raw, &inst) != nil {
		return nil
	}
	off := map[string]bool{}
	if raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); err == nil {
		var st struct {
			Enabled map[string]bool `json:"enabledPlugins"`
		}
		if json.Unmarshal(raw, &st) == nil {
			for k, v := range st.Enabled {
				off[k] = !v
			}
		}
	}
	var out []plugin
	for _, key := range sortedKeys(inst.Plugins) {
		if off[key] {
			continue
		}
		var entries []struct {
			InstallPath string `json:"installPath"`
		}
		// Version 1 kept one object per plugin, version 2 a list (one per scope).
		if json.Unmarshal(inst.Plugins[key], &entries) != nil {
			var one struct {
				InstallPath string `json:"installPath"`
			}
			if json.Unmarshal(inst.Plugins[key], &one) != nil {
				continue
			}
			entries = append(entries, one)
		}
		name, _, _ := strings.Cut(key, "@")
		seen := map[string]bool{}
		for _, e := range entries {
			if e.InstallPath == "" || seen[e.InstallPath] {
				continue
			}
			seen[e.InstallPath] = true
			out = append(out, plugin{name: name, path: e.InstallPath})
		}
	}
	return out
}

// claudeServer reads one server of Claude Code's format: {type, url, headers} or {command, args, env}.
func claudeServer(name, from string, raw json.RawMessage, pluginRoot string, getenv func(string) (string, bool)) (Found, string) {
	var c struct {
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	f := Found{Name: name, From: from}
	if err := json.Unmarshal(raw, &c); err != nil {
		return f, "the entry does not read"
	}
	env := withRoot(getenv, pluginRoot)
	switch {
	case c.Command != "" && (c.Type == "" || c.Type == "stdio"):
		f.Transport = Stdio
		raw := append([]string{c.Command}, c.Args...)
		line, why, def := expandLine(raw, env)
		if why != "" {
			return f, why
		}
		f.Command, f.Env, f.Defaulted = line, c.Env, def
		if HasVarRef(strings.Join(raw, " ")) {
			f.Template = strings.Join(raw, " ")
		}
		return f, ""
	case c.URL != "":
		f.Transport = HTTP
		if c.Type == "sse" {
			f.Transport = SSE
		}
		u, why, def := expandValue(c.URL, env)
		if why != "" {
			return f, "the address " + why
		}
		canon, _, err := canonical(u)
		if err != nil {
			return f, "the address is not http(s)"
		}
		f.URL, f.Headers, f.Defaulted = canon, c.Headers, def
		if HasVarRef(c.URL) {
			f.Template = c.URL
		}
		return f, ""
	}
	return f, "no command and no address"
}

// withRoot adds ${CLAUDE_PLUGIN_ROOT} for a plugin's own files.
func withRoot(getenv func(string) (string, bool), root string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		if k == "CLAUDE_PLUGIN_ROOT" && root != "" {
			return root, true
		}
		return getenv(k)
	}
}

type codexFound struct {
	Found
	err string
}

// codexServers reads [mcp_servers.<name>] of Codex's config: url (with http_headers, env_http_headers,
// bearer_token_env_var) or command, args and env.
func codexServers(path string, getenv func(string) (string, bool)) []codexFound {
	var cfg struct {
		Servers map[string]struct {
			URL            string            `toml:"url"`
			Command        string            `toml:"command"`
			Args           []string          `toml:"args"`
			Env            map[string]string `toml:"env"`
			Headers        map[string]string `toml:"http_headers"`
			EnvHeaders     map[string]string `toml:"env_http_headers"`
			BearerTokenEnv string            `toml:"bearer_token_env_var"`
			Enabled        *bool             `toml:"enabled"`
		} `toml:"mcp_servers"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil
	}
	var out []codexFound
	for _, name := range sortedKeys(cfg.Servers) {
		s := cfg.Servers[name]
		f := Found{Name: name, From: "codex"}
		if s.Enabled != nil && !*s.Enabled {
			out = append(out, codexFound{f, "turned off in Codex"})
			continue
		}
		switch {
		case s.Command != "":
			f.Transport = Stdio
			raw := append([]string{s.Command}, s.Args...)
			line, why, def := expandLine(raw, getenv)
			if why != "" {
				out = append(out, codexFound{f, why})
				continue
			}
			f.Command, f.Env, f.Defaulted = line, s.Env, def
			if HasVarRef(strings.Join(raw, " ")) {
				f.Template = strings.Join(raw, " ")
			}
		case s.URL != "":
			f.Transport = HTTP
			u, why, def := expandValue(s.URL, getenv)
			canon, _, err := canonical(u)
			if why != "" || err != nil {
				out = append(out, codexFound{f, "the address is not usable"})
				continue
			}
			f.URL, f.Defaulted = canon, def
			if HasVarRef(s.URL) {
				f.Template = s.URL
			}
			f.Headers = map[string]string{}
			for k, v := range s.Headers {
				f.Headers[k] = v
			}
			for k, v := range s.EnvHeaders {
				f.Headers[k] = "${" + v + "}"
			}
			if s.BearerTokenEnv != "" {
				f.Headers["Authorization"] = "Bearer ${" + s.BearerTokenEnv + "}"
			}
			if len(f.Headers) == 0 {
				f.Headers = nil
			}
		default:
			out = append(out, codexFound{f, "no command and no address"})
			continue
		}
		out = append(out, codexFound{f, ""})
	}
	return out
}

// varRe is ${VAR}, ${VAR:-default} or $VAR.
var varRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// HasVarRef says whether a value refers to a variable (a key the config does not hold).
func HasVarRef(v string) bool { return varRe.MatchString(v) }

// VarRefs are the variables a value refers to.
func VarRefs(v string) []string {
	var out []string
	for _, m := range varRe.FindAllStringSubmatch(v, -1) {
		name := m[1]
		if name == "" {
			name = m[4]
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// expandValue puts the variables' values in, a default where one is written; why names a variable that is not set;
// defaulted are the variables whose default was used.
func expandValue(v string, getenv func(string) (string, bool)) (string, string, []string) {
	why := ""
	var defaulted []string
	out := varRe.ReplaceAllStringFunc(v, func(m string) string {
		sm := varRe.FindStringSubmatch(m)
		name, def, hasDef := sm[1], sm[3], sm[2] != ""
		if name == "" {
			name = sm[4]
		}
		if val, ok := getenv(name); ok && val != "" {
			return val
		}
		if hasDef {
			if !slices.Contains(defaulted, name) {
				defaulted = append(defaulted, name)
			}
			return def
		}
		if why == "" {
			why = "needs $" + name + ", which is not set"
		}
		return m
	})
	return out, why, defaulted
}

func expandLine(line []string, getenv func(string) (string, bool)) ([]string, string, []string) {
	out := make([]string, 0, len(line))
	var defaulted []string
	for _, a := range line {
		v, why, def := expandValue(a, getenv)
		if why != "" {
			return nil, "the command " + why, nil
		}
		for _, d := range def {
			if !slices.Contains(defaulted, d) {
				defaulted = append(defaulted, d)
			}
		}
		out = append(out, v)
	}
	return out, "", defaulted
}

// loopback says whether an address is on this machine (127.0.0.0/8, ::1, localhost).
func loopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := u.Hostname()
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
