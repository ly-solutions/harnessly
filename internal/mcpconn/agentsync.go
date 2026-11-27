package mcpconn

// SyncAgents: every MCP server the agents are configured with becomes a connection at once, with no button.
// A rescan does not double them and does not undo the person's changes: a server it made is found again by its key
// (the address, or the command line) and only its agent mark changes; a server the person removed is not made again;
// one that is gone from the agent's config stays, marked. Keys written literally in the agent's config go to the safe
// (the file keeps none); a key the config only refers to (${VAR}) leaves the connection waiting for it and asks the
// person for it in the safe, under a name the next sync takes it from.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// AgentOrigin marks a connection that came from an agent's own config.
type AgentOrigin struct {
	// Key is what the server is found again by: "url:<address>" or "cmd:<command line>".
	Key string `json:"key"`
	// Template is the address or command line with its variables as the config writes it ("" — none): when the agent's
	// environment gives them another value, the next sync moves the connection to it.
	Template string `json:"template,omitempty"`
	// Names are the agent's names of it; From where it was found ("plugin example-infra", "codex", …).
	Names []string `json:"names,omitempty"`
	From  []string `json:"from"`
	// Present: the agent has it in its config now, so it reaches the server directly; harnessly does not hand it to
	// that agent a second time.
	Present bool      `json:"present"`
	SeenAt  time.Time `json:"seenAt"`
	GoneAt  time.Time `json:"goneAt,omitzero"`
	// Adopted: the person had connected it before; the sync only marked it.
	Adopted bool `json:"adopted,omitempty"`
	// Missing are the keys the config refers to and the safe does not have yet.
	Missing []MissingKey `json:"missing,omitempty"`
	// Note says what of the agent's entry was not taken (a second header).
	Note string `json:"note,omitempty"`
}

// MissingKey is a key a connection waits for.
type MissingKey struct {
	// Key is the header's name or the environment variable.
	Key string `json:"key"`
	// Header: it is the header's value, else an environment variable of a local server.
	Header bool `json:"header,omitempty"`
	// Template is the value as the config writes it ("Bearer ${TOKEN}"); the secret's value takes the place of its one
	// reference, or of the whole value when it has several.
	Template string `json:"template"`
	// Secret is the name the person puts the key under in the safe.
	Secret string `json:"secret"`
}

// HasAgent says whether the agent ("claude" or "codex") reaches the server directly from its own config.
func (srv Server) HasAgent(agent string) bool {
	if srv.Agent == nil || !srv.Agent.Present {
		return false
	}
	for _, f := range srv.Agent.From {
		if (f == "codex") == (agent == "codex") {
			return true
		}
	}
	return false
}

// Sync outcomes.
const (
	SyncTaken     = "taken"
	SyncAlready   = "already"
	SyncSkipped   = "skipped"
	SyncNeedsKey  = "needs-key"
	SyncKeyFilled = "key-filled"
	SyncGone      = "gone"
	// SyncMoved: the agent's environment expands the address (or command line) to another value now; the connection
	// follows it. This is harnessly's own expansion, not the person's change.
	SyncMoved = "moved"
)

// SyncOutcome is what the sync did with one server.
type SyncOutcome struct {
	Name    string `json:"name"`
	From    string `json:"from"`
	ID      string `json:"id,omitempty"`
	Outcome string `json:"outcome"`
	Why     string `json:"why,omitempty"`
}

// SyncReport is one sync: the outcomes, the keys to ask the person for, the servers made (to check them).
type SyncReport struct {
	Outcomes []SyncOutcome
	Ask      []AskKey
	New      []string
}

// AskKey is a key to ask for in the safe.
type AskKey struct {
	Server string
	MissingKey
}

func agentKey(f Found) string {
	if f.Transport == Stdio {
		return "cmd:" + strings.Join(f.Command, " ")
	}
	return "url:" + strings.TrimRight(f.URL, "/")
}

// merged is the servers found, one per key, with every name and place they were found under.
type merged struct {
	Found
	names, from []string
	// notes say what the merge left out: another port another agent writes for the same local server.
	notes []string
}

// localKey is a server on this machine's loopback by its name and path, the port aside: two agents may write two ports
// for one local bridge (one expanded from the agent's environment, one written literally long ago).
func localKey(f Found) string {
	if f.Transport == Stdio || !loopback(f.URL) {
		return ""
	}
	u, err := url.Parse(f.URL)
	if err != nil {
		return ""
	}
	return "lo:" + f.Name + "|" + u.Hostname() + u.Path
}

func mergeFound(found []Found) []merged {
	byKey, byLocal := map[string]int{}, map[string]int{}
	var out []merged
	for _, f := range found {
		k := agentKey(f)
		i, ok := byKey[k]
		if !ok && localKey(f) != "" {
			if j, has := byLocal[localKey(f)]; has {
				m := &out[j]
				// The address expanded from the agent's environment wins over one written literally.
				if f.Template != "" && m.Template == "" {
					m.notes = append(m.notes, strings.Join(m.from, ", ")+" writes "+m.URL)
					m.URL, m.Template, m.Defaulted = f.URL, f.Template, f.Defaulted
					byKey[agentKey(m.Found)] = j
				} else {
					m.notes = append(m.notes, f.From+" writes "+f.URL+": another port, the one of "+strings.Join(m.from, ", ")+" is taken")
				}
				i, ok = j, true
			}
		}
		if ok {
			m := &out[i]
			if !slices.Contains(m.names, f.Name) {
				m.names = append(m.names, f.Name)
			}
			if !slices.Contains(m.from, f.From) {
				m.from = append(m.from, f.From)
			}
			if m.Template == "" {
				m.Template = f.Template
			}
			for _, d := range f.Defaulted {
				if !slices.Contains(m.Defaulted, d) {
					m.Defaulted = append(m.Defaulted, d)
				}
			}
			for k, v := range f.Env {
				if _, has := m.Env[k]; !has {
					m.Env = withKV(m.Env, k, v)
				}
			}
			for k, v := range f.Headers {
				if _, has := m.Headers[k]; !has {
					m.Headers = withKV(m.Headers, k, v)
				}
			}
			continue
		}
		byKey[k] = len(out)
		if lk := localKey(f); lk != "" {
			byLocal[lk] = len(out)
		}
		out = append(out, merged{Found: f, names: []string{f.Name}, from: []string{f.From}})
	}
	return out
}

func withKV(m map[string]string, k, v string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	m[k] = v
	return m
}

// pkgName is a package argument without its version: "@scope/pkg@1.2.3" → "@scope/pkg", "pkg==1.0" → "pkg".
func pkgName(arg string) string {
	if i := strings.Index(arg, "=="); i > 0 {
		return arg[:i]
	}
	if i := strings.LastIndex(arg, "@"); i > 0 {
		return arg[:i]
	}
	return arg
}

var pkgArgRe = regexp.MustCompile(`^(@[a-z0-9._~-]+/)?[a-z0-9._~-]+(@[0-9][^ ]*|==[0-9][^ ]*)?$`)

// catalogMatch is the catalog's entry of a found server: the same address, or the same package of a local server.
func catalogMatch(f Found) (CatalogEntry, bool) {
	for _, e := range catalog {
		switch {
		case f.Transport != Stdio && e.URL != "" && e.Transport != API:
			if c, _, err := canonical(e.URL); err == nil && strings.TrimRight(c, "/") == strings.TrimRight(f.URL, "/") {
				return e, true
			}
		case f.Transport == Stdio && e.Transport == Stdio && len(e.Command) > 1:
			pkg := ""
			for _, a := range e.Command[1:] {
				if !strings.HasPrefix(a, "-") && pkgArgRe.MatchString(a) && (strings.Contains(a, "@") || strings.Contains(a, "==")) {
					pkg = pkgName(a)
					break
				}
			}
			if pkg == "" || len(f.Command) == 0 || f.Command[0] != e.Command[0] {
				continue
			}
			for _, a := range f.Command[1:] {
				if !strings.HasPrefix(a, "-") && pkgName(a) == pkg {
					return e, true
				}
			}
		}
	}
	return CatalogEntry{}, false
}

// askName is the safe's name of a key a connection waits for.
func askName(slug, key string) string {
	return "mcp-agent-" + slug + "-" + strings.Trim(slugOf(strings.ToLower(key)), "-")
}

// SyncAgents makes connections of the servers found, marks those gone, and takes the keys the person has put in the
// safe since the last sync. skipped are the scan's own refusals, passed into the outcomes.
func (s *Service) SyncAgents(found []Found, skipped []Skipped) (SyncReport, error) {
	var rep SyncReport
	// A server read but not reachable now keeps its connection as it is: not gone, not moved.
	seen, seenTpl := map[string]bool{}, map[string]bool{}
	for _, sk := range skipped {
		rep.Outcomes = append(rep.Outcomes, SyncOutcome{Name: sk.Name, From: sk.From, Outcome: SyncSkipped, Why: sk.Why})
		if sk.Key != "" {
			seen[sk.Key] = true
		}
		if sk.Template != "" {
			seenTpl[sk.Template] = true
		}
	}
	type credWrite struct {
		id string
		c  cred
	}
	var creds []credWrite
	now := s.now().UTC()

	s.mu.Lock()
	f, err := s.readAll()
	if err != nil {
		s.mu.Unlock()
		return rep, err
	}
	before, _ := json.Marshal(f)
	for _, m := range mergeFound(found) {
		key := agentKey(m.Found)
		seen[key] = true
		if m.Template != "" {
			seenTpl[m.Template] = true
		}
		from := strings.Join(m.from, ", ")
		if slices.Contains(f.Dismissed, key) || m.Template != "" && slices.Contains(f.Dismissed, "tpl:"+m.Template) {
			rep.Outcomes = append(rep.Outcomes, SyncOutcome{Name: m.Name, From: from, Outcome: SyncSkipped, Why: "the person removed this connection"})
			continue
		}
		if i := slices.IndexFunc(f.Servers, func(x Server) bool { return x.Agent != nil && x.Agent.Key == key }); i >= 0 {
			srv := &f.Servers[i]
			if !srv.Agent.Present {
				srv.Agent.SeenAt, srv.Agent.GoneAt = now, time.Time{}
			}
			srv.Agent.Names, srv.Agent.From, srv.Agent.Present = m.names, m.from, true
			if m.Template != "" {
				srv.Agent.Template = m.Template
			}
			rep.Outcomes = append(rep.Outcomes, SyncOutcome{Name: m.Name, From: from, ID: srv.ID, Outcome: SyncAlready, Why: "connected before" + defaultedNote(m.Found) + mergeNote(m)})
			continue
		}
		if i := slices.IndexFunc(f.Servers, func(x Server) bool { return x.Owner == "" && x.Agent != nil && sameTemplate(x, m) }); i >= 0 {
			srv := &f.Servers[i]
			was := srv.URL
			if srv.Transport == Stdio {
				was = strings.Join(srv.Command, " ")
				srv.Command = slices.Clone(m.Command)
			} else {
				srv.URL = m.URL
			}
			to := strings.TrimPrefix(strings.TrimPrefix(key, "url:"), "cmd:")
			srv.Agent.Key, srv.Agent.Template, srv.Agent.Names, srv.Agent.From, srv.Agent.Present = key, m.Template, m.names, m.from, true
			srv.Agent.GoneAt = time.Time{}
			srv.State, srv.Tools, srv.CheckedAt = StateUnreachable, nil, time.Time{}
			rep.New = append(rep.New, srv.ID) // checked again at its new address
			rep.Outcomes = append(rep.Outcomes, SyncOutcome{Name: m.Name, From: from, ID: srv.ID, Outcome: SyncMoved,
				Why: "the agent's environment expands " + m.Template + " to " + to + " (was " + was + ")" + defaultedNote(m.Found) + mergeNote(m)})
			continue
		}
		if i := slices.IndexFunc(f.Servers, func(x Server) bool { return x.Owner == "" && sameServer(x, m.Found) }); i >= 0 {
			srv := &f.Servers[i]
			srv.Agent = &AgentOrigin{Key: key, Names: m.names, From: m.from, Present: true, SeenAt: now, Adopted: true}
			rep.Outcomes = append(rep.Outcomes, SyncOutcome{Name: m.Name, From: from, ID: srv.ID, Outcome: SyncAlready, Why: "the person had connected it: only marked"})
			continue
		}
		srv, c, out := s.fromAgent(m, f.Servers, now)
		f.Servers = append(f.Servers, srv)
		if c.Header != "" || len(c.Env) > 0 {
			creds = append(creds, credWrite{srv.ID, c})
		}
		for _, mk := range srv.Agent.Missing {
			rep.Ask = append(rep.Ask, AskKey{Server: srv.Name, MissingKey: mk})
		}
		out.From = from
		rep.Outcomes = append(rep.Outcomes, out)
		rep.New = append(rep.New, srv.ID)
	}
	for i := range f.Servers {
		srv := &f.Servers[i]
		if srv.Agent == nil || seen[srv.Agent.Key] || srv.Agent.Template != "" && seenTpl[srv.Agent.Template] || !srv.Agent.Present {
			continue
		}
		srv.Agent.Present, srv.Agent.GoneAt = false, now
		rep.Outcomes = append(rep.Outcomes, SyncOutcome{Name: srv.Name, From: strings.Join(srv.Agent.From, ", "), ID: srv.ID,
			Outcome: SyncGone, Why: "no longer in the agent's config: kept, marked"})
	}
	// A sync that changed nothing writes nothing: the agents and the page are not told of a change that is not one.
	if after, _ := json.Marshal(f); string(after) != string(before) {
		err = s.writeAll(f)
	}
	s.mu.Unlock()
	if err != nil {
		return rep, err
	}
	for _, w := range creds {
		if err := s.writeCred(w.id, w.c); err != nil {
			return rep, err
		}
	}
	rep.Outcomes = append(rep.Outcomes, s.fillMissing()...)
	// Keys still missing (asked before, not given yet) are asked again only by the runtime's choice: it gets them all.
	return rep, nil
}

// sameServer says whether a connection the person made is the server found: the same address or command line.
func sameServer(x Server, f Found) bool {
	if f.Transport == Stdio {
		return x.Transport == Stdio && slices.Equal(x.Command, f.Command)
	}
	return x.Transport != Stdio && x.Transport != API && strings.TrimRight(x.URL, "/") == strings.TrimRight(f.URL, "/")
}

// fromAgent makes the connection of a found server, the credential its literal keys make, and the outcome.
func (s *Service) fromAgent(m merged, list []Server, now time.Time) (Server, cred, SyncOutcome) {
	srv := Server{ID: newID(), Name: oneLine(m.Name), Transport: m.Transport, Auth: AuthNone, Enabled: true,
		State: StateUnreachable, CreatedAt: now,
		Agent: &AgentOrigin{Key: agentKey(m.Found), Names: m.names, From: m.from, Present: true, SeenAt: now}}
	if e, ok := catalogMatch(m.Found); ok {
		srv.CatalogID = e.ID
		if e.Name != "" {
			srv.Name = e.Name
		}
		if srv.Transport == Stdio {
			srv.FixedEnv = e.FixedEnv
		}
	} else {
		s.ownIdentity(&srv)
		srv.CatalogID = uniqueCustomID(srv.CatalogID, list, srv.ID)
	}
	srv.Slug = uniqueSlug(slugBase(srv), list, srv.ID)
	out := SyncOutcome{Name: m.Name, ID: srv.ID, Outcome: SyncTaken}
	if srv.CatalogID != "" && !strings.HasPrefix(srv.CatalogID, CustomPrefix) {
		out.Why = "matched the catalog: " + srv.CatalogID
	} else {
		out.Why = "a service of one's own: " + srv.CatalogID
	}
	var c cred
	if srv.Transport == Stdio {
		srv.URL = "stdio:" + srv.CatalogID
		srv.Command = slices.Clone(m.Command)
		names := sortedKeys(m.Env)
		for _, k := range names {
			v := m.Env[k]
			srv.Env = append(srv.Env, EnvField{Name: k, Label: k, Secret: true})
			if HasVarRef(v) {
				srv.Agent.Missing = append(srv.Agent.Missing, MissingKey{Key: k, Template: v, Secret: askName(srv.Slug, k)})
				continue
			}
			c.Env = withKV(c.Env, k, v)
		}
		if len(names) > 0 {
			srv.Auth = AuthEnv
		}
	} else {
		srv.URL = m.URL
		names := sortedKeys(m.Headers)
		sort.SliceStable(names, func(i, j int) bool {
			return strings.EqualFold(names[i], "Authorization") && !strings.EqualFold(names[j], "Authorization")
		})
		for i, k := range names {
			if i > 0 {
				srv.Agent.Note = strings.TrimPrefix(srv.Agent.Note+", "+k, ", ")
				continue
			}
			v := m.Headers[k]
			if HasVarRef(v) {
				hn, _, err := headerOf(k, "x")
				if err != nil {
					srv.Agent.Note = k
					continue
				}
				srv.Auth, srv.HeaderName = AuthHeader, hn
				srv.Agent.Missing = append(srv.Agent.Missing, MissingKey{Key: hn, Header: true, Template: v, Secret: askName(srv.Slug, hn)})
				continue
			}
			hn, value, err := headerOf(k, v)
			if err != nil {
				srv.Agent.Note = k
				continue
			}
			srv.Auth, srv.HeaderName, c.Header = AuthHeader, hn, value
		}
		if srv.Agent.Note != "" {
			srv.Agent.Note = "headers not taken: " + srv.Agent.Note
		}
	}
	srv.Agent.Template = m.Template
	out.Why += defaultedNote(m.Found) + mergeNote(m)
	if len(srv.Agent.Missing) > 0 {
		srv.State = StateSignIn
		out.Outcome = SyncNeedsKey
		var keys []string
		for _, mk := range srv.Agent.Missing {
			keys = append(keys, mk.Key+" → safe "+mk.Secret)
		}
		out.Why += "; waits for " + strings.Join(keys, ", ")
	}
	return srv, c, out
}

// fillMissing takes the keys the person has put in the safe for the connections that wait for them.
func (s *Service) fillMissing() []SyncOutcome {
	list, err := s.List()
	if err != nil {
		return nil
	}
	var out []SyncOutcome
	for _, srv := range list {
		if srv.Agent == nil || len(srv.Agent.Missing) == 0 {
			continue
		}
		c, _ := s.readCred(srv.ID)
		var left []MissingKey
		var filled []string
		for _, mk := range srv.Agent.Missing {
			v, err := s.secrets.Reveal(mk.Secret)
			if err != nil || strings.TrimSpace(v) == "" {
				left = append(left, mk)
				continue
			}
			value := strings.TrimSpace(v)
			if refs := VarRefs(mk.Template); len(refs) == 1 {
				value = varRe.ReplaceAllLiteralString(mk.Template, value)
			}
			if mk.Header {
				_, hv, err := headerOf(mk.Key, value)
				if err != nil {
					left = append(left, mk)
					continue
				}
				c.Header = hv
			} else {
				c.Env = withKV(c.Env, mk.Key, value)
			}
			filled = append(filled, mk.Key)
		}
		if len(filled) == 0 {
			continue
		}
		if err := s.writeCred(srv.ID, c); err != nil {
			continue
		}
		_, _ = s.update(srv.ID, func(x *Server) error {
			if x.Agent != nil {
				x.Agent.Missing = left
			}
			return nil
		})
		out = append(out, SyncOutcome{Name: srv.Name, From: strings.Join(srv.Agent.From, ", "), ID: srv.ID, Outcome: SyncKeyFilled,
			Why: fmt.Sprintf("took %s from the safe", strings.Join(filled, ", "))})
	}
	return out
}

// defaultedNote tells the trace that an address was expanded by its default: the agent's environment has no value.
func defaultedNote(f Found) string {
	if len(f.Defaulted) == 0 {
		return ""
	}
	return "; expanded by default: $" + strings.Join(f.Defaulted, ", $") + " is not in the agent's environment"
}

// sameTemplate says whether a connection the sync made is the server found, expanded to another value now: the same
// template, or — a connection made before templates were kept — the same agent's name and place, the same transport
// and, for an address, the same path on the same loopback host.
func sameTemplate(x Server, m merged) bool {
	if m.Template == "" || x.Agent == nil {
		return false
	}
	if x.Agent.Template != "" {
		return x.Agent.Template == m.Template
	}
	if x.Transport != m.Transport || !overlap(x.Agent.Names, m.names) || !overlap(x.Agent.From, m.from) {
		return false
	}
	if m.Transport == Stdio {
		return len(x.Command) > 0 && len(m.Command) > 0 && x.Command[0] == m.Command[0]
	}
	a, errA := url.Parse(x.URL)
	b, errB := url.Parse(m.URL)
	return errA == nil && errB == nil && a.Hostname() == b.Hostname() && a.Path == b.Path && loopback(x.URL)
}

func overlap(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// mergeNote tells the trace what another agent wrote for the same local server and was not taken.
func mergeNote(m merged) string {
	if len(m.notes) == 0 {
		return ""
	}
	return "; " + strings.Join(m.notes, "; ")
}
