package files

import (
	"path/filepath"
	"strings"
)

// homeCredentials are the places under a home folder that hold credentials, which the name rule of Secret does not
// see: harnessly's own data (the machine's keys, the cli-token, the safe's key), the agents' and the tools' sign-ins,
// and the shells' histories.
var homeCredentials = []string{
	".harnessly", ".config/harnessly", ".local/state/harnessly", ".codex", ".claude.json",
	".config/gh", ".config/glab-cli", ".config/gcloud", ".config/hub", ".config/rclone", ".config/op", ".config/doctl",
	".config/tbot", ".git-credentials", ".kube", ".docker", ".azure", ".tsh", ".vault-token", ".bao-token",
	".password-store", ".local/share/keyrings", ".terraform.d", ".npmrc", ".pypirc", ".pgpass", ".my.cnf",
	".bash_history", ".zsh_history", ".psql_history", ".mysql_history", ".python_history", ".node_repl_history",
	".local/share/fish/fish_history",
}

// dataSecrets are what stays closed in a data folder that cannot be closed as a whole.
var dataSecrets = []string{
	"machine.json", "hub/machine.json", "agent.json", "cli-token", "config.json", "connections.json", "secrets", "mcp",
	"codex", "providers", "connectors", "agent-env", "chatgpt",
}

// HomeCredentials are the credential places of the home folder home.
func HomeCredentials(home string) []string {
	if home == "" {
		return nil
	}
	out := make([]string, 0, len(homeCredentials))
	for _, p := range homeCredentials {
		out = append(out, filepath.Join(home, p))
	}
	return out
}

// Deny closes the places (files or folders, by their own path and by the real one behind links) to every root, except
// the folders of open that lie inside them. A place that is or holds one of keep (the home folder, the workspace) cannot
// be closed whole without closing that too: its known secret files are closed instead.
func (a *Access) Deny(places, open, keep []string) {
	both := func(p string) []string {
		p = resolveAbs(p)
		if real, err := Realpath(p); err == nil && real != p {
			return []string{p, real}
		}
		return []string{p}
	}
	var kept []string
	for _, k := range keep {
		if k != "" {
			kept = append(kept, both(k)...)
		}
	}
	for _, place := range places {
		if place == "" {
			continue
		}
		for _, p := range both(place) {
			holds := false
			for _, k := range kept {
				holds = holds || underDir(k, p)
			}
			if !holds {
				a.denied = append(a.denied, p)
				continue
			}
			for _, name := range dataSecrets {
				a.denied = append(a.denied, filepath.Join(p, name))
			}
		}
	}
	for _, o := range open {
		if o != "" {
			a.open = append(a.open, both(o)...)
		}
	}
}

// underDir: p is dir or lies under it; letter case does not tell them apart (a file system may not).
func underDir(p, dir string) bool {
	dir = strings.TrimSuffix(dir, "/")
	return len(p) >= len(dir) && strings.EqualFold(p[:len(dir)], dir) && (len(p) == len(dir) || p[len(dir)] == '/')
}

func (a *Access) closed(p string) bool {
	if !strings.HasPrefix(p, "/") {
		return false
	}
	p = filepath.Clean(p)
	for _, d := range a.denied {
		if !underDir(p, d) {
			continue
		}
		opened := false
		for _, o := range a.open {
			// An opening counts only inside the closed place: a root above it opens nothing.
			opened = opened || len(o) > len(d) && underDir(o, d) && underDir(p, o)
		}
		if !opened {
			return true
		}
	}
	return false
}
