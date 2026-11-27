// Package viewonly is what a person who may only watch a machine reaches there (docs/hub/own-work.md, «Levels»). The
// hub applies it to every request of such a person, which also covers machines that run an older runtime; the runtime
// applies it again to a session the hub asked for as a watcher's.
//
// Both lists are allow-lists of what the watching page asks for: a route that is not here is closed. Routes that hand
// over files of the machine (downloads, previews, the file pane, notebooks, the folder browser), terminals, secrets,
// settings of agents and the preview of the owner's dev servers are left out on purpose.
package viewonly

import (
	"context"
	"encoding/json"
	"strings"
)

// Level is the value of a session's "level" claim for a watcher.
const Level = "view"

type key struct{}

// With marks a request as a watcher's.
func With(ctx context.Context) context.Context { return context.WithValue(ctx, key{}, true) }

// Watching says the request is a watcher's.
func Watching(ctx context.Context) bool { v, _ := ctx.Value(key{}).(bool); return v }

// sockets are the runtime's sockets a watcher may open, with the messages of the page each one passes on. The
// heartbeat ({"type":"ping"}) is one of them everywhere.
var sockets = map[string]map[string]bool{
	// The conversation: what a chat says, as it comes. timeline:resume starts the agent again and is a task.
	"/api/timeline": {"timeline:subscribe": true, "timeline:unsubscribe": true},
	// Tab states and the queue: asking for them again; dismissing a tab or a notice changes the owner's page.
	"/api/status": {"status:request-sync": true, "session-history:request": true},
	// Layout and workspace changes: the page sends nothing there.
	"/api/sync": {},
}

// Socket says a watcher may open the socket at the runtime's path (without any mount of the runtime).
func Socket(path string) bool { _, ok := sockets[path]; return ok }

// Message is a message of a watcher's page on the socket at path, rewritten to what may go on to the runtime; false
// drops it. The type is read from the key "type" exactly, as the runtime reads it, and the message is written anew
// without any other spelling of that key, so both sides read the same one.
func Message(path string, data []byte) ([]byte, bool) {
	var msg map[string]json.RawMessage
	if json.Unmarshal(data, &msg) != nil {
		return nil, false
	}
	var typ string
	if json.Unmarshal(msg["type"], &typ) != nil || !MessageType(path, typ) {
		return nil, false
	}
	for k := range msg {
		if k != "type" && strings.EqualFold(k, "type") {
			delete(msg, k)
		}
	}
	out, err := json.Marshal(msg)
	return out, err == nil
}

// MessageType says a message of this type from a watcher's page may be acted on.
func MessageType(path, typ string) bool {
	allow, ok := sockets[path]
	return ok && (typ == "ping" || allow[typ])
}

// api are the reads of the API under /api: "*" is one segment, a final "**" any number of them.
var api = []string{
	"auth/preflight", "auth/setup", "capabilities", "check-agent", "check-claude", "config", "health", "keybindings",
	"layout", "layout/cwd", "manifest", "message-history", "preflight/runtime", "push/vapid-key", "quick-prompts",
	"sidebar-items", "stats/**", "timeline/entries", "timeline/message-counts", "timeline/sessions", "uploads/**",
	"voice/assets/**", "workspace",

	"v2/apps", "v2/designly/**", "v2/hello", "v2/machine/stats", "v2/machines", "v2/me", "v2/me/looks/**",
	"v2/me/settings", "v2/runtime-settings", "v2/search", "v2/sessions", "v2/uploads/*",
	"v2/conversations/*/context", "v2/conversations/*/digest", "v2/conversations/*/entries/*/output",
	"v2/conversations/*/glance", "v2/conversations/*/legacy", "v2/conversations/*/made",
	"v2/conversations/*/messages", "v2/conversations/*/open", "v2/conversations/*/open-refs",
	"v2/conversations/*/rewind", "v2/conversations/*/rewinds/*/turns", "v2/conversations/*/shot",
	"v2/conversations/*/tasks", "v2/conversations/*/turns", "v2/conversations/*/turns/*/entries",
}

// platform are the reads under the platform's own prefix, which a deployment names (/api/<name>/…).
var platform = []string{
	"apps/kinds", "apps/kinds/docs", "apps/widgets", "bootstrap", "commands", "docs/people", "economics",
	"favicon", "gateway-ping", "images", "installed-apps", "launch-options", "link-meta", "link-meta/avatar",
	"link-meta/projects", "machines", "notices", "notices/stream", "pane-intents", "release-notice", "search",
	"sources", "stats", "subagents", "subagents/*", "work",
	// Sinopia Design: the boards are results; who else sees them the hub's design access decides.
	"access/**", "design/access/**", "design/deck/**", "design/engine", "design/home", "design/kit/canvas",
	"design/kit/canvases", "design/kit/file/**", "design/kit/fonts", "design/kit/layouts", "design/kit/link",
	"design/kit/project", "design/kit/usage", "design/kits/**", "design/library", "design/library/boards",
	"design/library/history", "design/library/locate", "design/library/releases", "design/library/shot",
	"design/library/thumb", "design/project/**", "design/published", "design/scene3d/**", "design/site/**",
	"design/store/**", "design/store-cover", "design/store-library/**", "design/store-records-check",
	"design/store-work-machine", "design/video/**",
}

// notPlatform are first segments under /api that are never a deployment's name for the platform.
var notPlatform = map[string]bool{"cli": true, "fs": true, "v2": true, "layout": true, "editor": true, "internal": true}

// Readable says a watcher may read (GET, HEAD, OPTIONS) the runtime's path: path is the decoded path without the
// runtime's mount, escaped the same request's path as it was written. Outside /api lie the page and its files.
func Readable(path, escaped string) bool {
	if low := strings.ToLower(escaped); strings.Contains(low, "%2f") || strings.Contains(low, "%5c") {
		return false
	}
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\\\x00") {
		return false
	}
	var segs []string
	if trimmed := strings.TrimSuffix(path[1:], "/"); trimmed != "" {
		segs = strings.Split(trimmed, "/")
	}
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return false
		}
	}
	if len(segs) == 0 || segs[0] != "api" {
		for _, s := range segs {
			// The API under another mount, and a preview of the owner's dev server (by path).
			if s == "api" || s == "__pv" {
				return false
			}
		}
		return len(segs) == 0 || segs[0] != "internal"
	}
	rest := segs[1:]
	for _, p := range api {
		if match(p, rest) {
			return true
		}
	}
	if len(rest) > 1 && !notPlatform[rest[0]] {
		for _, p := range platform {
			if match(p, rest[1:]) {
				return true
			}
		}
	}
	return false
}

func match(pattern string, segs []string) bool {
	want := strings.Split(pattern, "/")
	for i, w := range want {
		if w == "**" {
			return true
		}
		if i >= len(segs) || w != "*" && w != segs[i] {
			return false
		}
	}
	return len(segs) == len(want)
}
