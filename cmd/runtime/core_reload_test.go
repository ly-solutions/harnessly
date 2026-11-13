package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/pluginreload"
	"github.com/ly-solutions/harnessly/internal/status"
	"github.com/ly-solutions/harnessly/internal/tmux"
)

// The sweep of plugin reloads at rest: a tab with no session or a bare shell is settled without its screen
// being read, and a tab whose screen says «wait» is read again only after a pause that doubles up to five minutes —
// at once when a hook of the tab came in between.
func TestPluginReloadSweepDoesNotCrowdTmux(t *testing.T) {
	r := &pluginReloader{applied: &pluginreload.Applied{File: filepath.Join(t.TempDir(), "applied.json")}}
	mark := pluginreload.Mark{"plugins": 5}
	tab := func(id string, seq int) status.AgentSource {
		return status.AgentSource{TabID: id, TmuxSession: "s-" + id, PanelType: "claude-code", Provider: "claude", CliState: "idle", EventSeq: seq}
	}
	panes := map[string]tmux.PaneInfo{"s-shell": {Command: "bash"}, "s-busy": {Command: "claude"}, "s-ok": {Command: "claude"}}
	tried := map[string]int{}
	listings := 0
	pass := reloadPass{
		queued: func(string) bool { return false },
		panes:  func() (map[string]tmux.PaneInfo, bool) { listings++; return panes, true },
		try: func(session, command string) string {
			tried[session]++
			if command != "/reload-plugins" {
				t.Fatalf("typed %q", command)
			}
			if session == "s-ok" {
				return "applied"
			}
			return "wait"
		},
	}
	now := time.Unix(1_700_000_000, 0)
	sources := []status.AgentSource{tab("gone", 1), tab("shell", 1), tab("busy", 1), tab("ok", 1)}
	r.sweepTabs(mark, sources, now, pass)
	if tried["s-gone"] != 0 || tried["s-shell"] != 0 || tried["s-busy"] != 1 || tried["s-ok"] != 1 || listings != 1 {
		t.Fatalf("the first sweep: screens read %v, listings %d", tried, listings)
	}
	for _, id := range []string{"gone", "shell", "ok"} {
		if got := r.applied.Get(id); got["plugins"] != 5 {
			t.Fatalf("%s is not settled: %v", id, got)
		}
	}
	if got := r.applied.Get("busy"); got != nil {
		t.Fatalf("a waiting tab was marked: %v", got)
	}

	// An hour of sweeps five seconds apart: the waiting tab's screen is read after 5, 10, 20 … 300 s, not 720 times,
	// and nothing is asked of tmux for the settled tabs.
	listings = 0
	var readAt []time.Duration
	for at := reloadSweep; at <= time.Hour; at += reloadSweep {
		before := tried["s-busy"]
		r.sweepTabs(mark, sources, now.Add(at), pass)
		if tried["s-busy"] != before {
			readAt = append(readAt, at)
		}
	}
	if tried["s-gone"] != 0 || tried["s-shell"] != 0 || tried["s-ok"] != 1 {
		t.Fatalf("settled tabs were read again: %v", tried)
	}
	if len(readAt) < 12 || len(readAt) > 20 || listings != len(readAt) {
		t.Fatalf("the waiting tab was read %d times in an hour (at %v), listings %d", len(readAt), readAt, listings)
	}
	want := []time.Duration{5 * time.Second, 15 * time.Second, 35 * time.Second, 75 * time.Second, 155 * time.Second, 315 * time.Second, 615 * time.Second}
	for i, w := range want {
		if readAt[i] != w {
			t.Fatalf("the pauses: read at %v, want first %v", readAt, want)
		}
	}
	for i := len(want); i < len(readAt); i++ {
		if readAt[i]-readAt[i-1] != reloadRetryMax {
			t.Fatalf("the longest pause is not %s: %v", reloadRetryMax, readAt)
		}
	}

	// A hook of the tab (its turn ended) is a reason to read its screen at the next sweep, not after the pause.
	later := now.Add(time.Hour + reloadSweep)
	before := tried["s-busy"]
	moved := []status.AgentSource{tab("gone", 1), tab("shell", 1), tab("busy", 2), tab("ok", 1)}
	r.sweepTabs(mark, moved, later, pass)
	r.sweepTabs(mark, moved, later.Add(time.Second), pass)
	if tried["s-busy"] != before+1 {
		t.Fatalf("after a hook the screen was read %d times", tried["s-busy"]-before)
	}

	// tmux that does not answer settles nothing and reads no screen.
	fresh := &pluginReloader{applied: &pluginreload.Applied{File: filepath.Join(t.TempDir(), "applied.json")}}
	pass.panes = func() (map[string]tmux.PaneInfo, bool) { return nil, false }
	tried = map[string]int{}
	fresh.sweepTabs(mark, sources, now, pass)
	if len(tried) != 0 || fresh.applied.Get("gone") != nil {
		t.Fatalf("a silent tmux: read %v, settled %v", tried, fresh.applied.Get("gone"))
	}
}
