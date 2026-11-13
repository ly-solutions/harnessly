package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/pluginreload"
	"github.com/ly-solutions/harnessly/internal/prompts"
	"github.com/ly-solutions/harnessly/internal/screen"
	"github.com/ly-solutions/harnessly/internal/status"
	"github.com/ly-solutions/harnessly/internal/tmux"
)

// The reload of plugins and skills in idle Claude tabs after the plugin reconciler changed them (internal/pluginreload).
// In front of the old runtime it is the old runtime's (it keeps the applied marks); alone, the core sweeps.

const reloadSweep = 5 * time.Second

type pluginReloader struct {
	c        *core
	markFile string
	applied  *pluginreload.Applied

	mu      sync.Mutex
	mark    pluginreload.Mark
	markMod time.Time

	// retry: the tabs told to wait by their screens; only the sweep's goroutine touches it.
	retry map[string]reloadRetry
}

// newPluginReloader watches the reconciler's mark (RUNTIME_PLUGIN_RELOAD_MARK); nil without one.
func newPluginReloader(c *core) *pluginReloader {
	mark := os.Getenv("RUNTIME_PLUGIN_RELOAD_MARK")
	if mark == "" {
		return nil
	}
	return &pluginReloader{c: c, markFile: mark, applied: &pluginreload.Applied{File: filepath.Join(c.cfg.DataDir, "claude-reload-applied.json")}}
}

// readMark is the mark, read again when the file changed; an unreadable one keeps the last (the reconciler replaces the
// file whole).
func (r *pluginReloader) readMark() pluginreload.Mark {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, err := os.Stat(r.markFile)
	if err != nil {
		return nil
	}
	if !st.ModTime().Equal(r.markMod) {
		if raw, err := os.ReadFile(r.markFile); err == nil {
			if m, ok := pluginreload.ParseMark(raw); ok {
				r.mark, r.markMod = m, st.ModTime()
			}
		}
	}
	return r.mark
}

// launched: a Claude the platform starts loads the plugins as they are now; nothing to reload in it.
func (r *pluginReloader) launched(tabID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	mark := r.mark
	r.mu.Unlock()
	if mark == nil {
		return
	}
	next := pluginreload.Apply(nil, mark, "", "settled")
	r.applied.Set(tabID, next)
	if err := r.applied.Save(); err != nil {
		agent.Log("warn", "core: applied reload marks not saved", agent.F{K: "err", V: err.Error()})
	}
}

func (r *pluginReloader) run(ctx context.Context, m *status.Manager, svc *prompts.Service) {
	t := time.NewTicker(reloadSweep)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.sweep(m, svc)
		}
	}
}

func (r *pluginReloader) sweep(m *status.Manager, svc *prompts.Service) {
	mark := r.readMark()
	if mark == nil {
		return
	}
	r.sweepTabs(mark, m.AgentSources(), time.Now(), reloadPass{
		queued: func(tabID string) bool { return len(svc.Queue().List(tabID)) > 0 },
		panes:  r.c.cfg.Tmux.ReadAllPanes,
		try:    func(session, command string) string { return r.reloadTab(svc, session, command) },
	})
}

// reloadPass is what a sweep asks of the rest of the runtime.
type reloadPass struct {
	queued func(tabID string) bool
	// panes is the shared listing of the panes, asked once a sweep and only when a tab has a reload to type.
	panes func() (map[string]tmux.PaneInfo, bool)
	// try reads the tab's screen and types the reload when the screen allows; a pluginreload outcome.
	try func(session, command string) string
}

// reloadRetry is how long a tab whose screen said «wait» is left alone before its screen is read again: twice as
// long each time, from reloadSweep to reloadRetryMax. Each read of the screen is a call of tmux, and 164 tabs asked
// every five seconds were most of the runtime's tmux processes at rest.
const reloadRetryMax = 5 * time.Minute

type reloadRetry struct {
	until time.Time
	pause time.Duration
	seq   int // the tab's EventSeq when it was told to wait: a hook since then (a turn ended) is a reason to look now
}

func (r *pluginReloader) sweepTabs(mark pluginreload.Mark, sources []status.AgentSource, now time.Time, p reloadPass) {
	live := map[string]bool{}
	for _, s := range sources {
		live[s.TabID] = true
	}
	changed := false
	for _, id := range r.applied.Tabs() {
		if len(sources) > 0 && !live[id] {
			r.applied.Delete(id)
			changed = true
		}
	}
	for id := range r.retry {
		if !live[id] {
			delete(r.retry, id)
		}
	}
	var panes map[string]tmux.PaneInfo
	listed := false
	// One tab at a time: a reload watches the screen for a moment, and the sweep must not crowd tmux.
	for _, s := range sources {
		applied := r.applied.Get(s.TabID)
		d := pluginreload.Decide(pluginreload.Tab{PanelType: s.PanelType, Provider: s.Provider, CliState: s.CliState, Queued: p.queued(s.TabID)}, mark, applied)
		if d.Action != "insert" {
			continue
		}
		if w, waits := r.retry[s.TabID]; waits && w.seq == s.EventSeq && now.Before(w.until) {
			continue
		}
		if !listed {
			var ok bool
			if panes, ok = p.panes(); !ok {
				return // tmux did not say what runs where: the next sweep asks again
			}
			listed = true
		}
		var outcome string
		if pane, has := panes[s.TmuxSession]; !has || tmux.SafeShells[pane.Command] {
			// No session, or a bare shell in it: no Claude to reload, and the next one reads the plugins as they are.
			// Its screen is not read for that.
			outcome = "settled"
		} else {
			command := pluginreload.Commands[d.Kind]
			outcome = p.try(s.TmuxSession, command)
			if outcome == "applied" {
				agent.Log("info", "core: plugins changed, reload typed into an idle Claude", agent.F{K: "tabId", V: s.TabID}, agent.F{K: "command", V: command})
			}
		}
		switch outcome {
		case "gone":
			outcome = "settled" // the session went while the reload was typed
		case "wait":
			w := r.retry[s.TabID]
			if w.seq != s.EventSeq || w.pause == 0 {
				w.pause = reloadSweep
			} else {
				w.pause = min(2*w.pause, reloadRetryMax)
			}
			w.until, w.seq = now.Add(w.pause), s.EventSeq
			if r.retry == nil {
				r.retry = map[string]reloadRetry{}
			}
			r.retry[s.TabID] = w
			continue
		}
		delete(r.retry, s.TabID)
		if next := pluginreload.Apply(applied, mark, d.Kind, outcome); outcome == "applied" || outcome == "settled" {
			r.applied.Set(s.TabID, next)
			changed = true
		}
	}
	if changed {
		if err := r.applied.Save(); err != nil {
			agent.Log("warn", "core: applied reload marks not saved", agent.F{K: "err", V: err.Error()})
		}
	}
}

func (r *pluginReloader) reloadTab(svc *prompts.Service, session, command string) string {
	tm := r.c.cfg.Tmux
	var view *pluginreload.Screen
	if styled, ok := tm.CaptureStyled(session); ok {
		st := screen.Read(styled)
		view = &pluginreload.Screen{Busy: st.Busy, Menu: st.Menu != nil}
		if st.InputLine {
			view.Input = &st.Input
		}
	}
	switch pluginreload.ScreenDecision(view) {
	case "wait":
		return "wait"
	case "probe":
		if p := tm.PaneCurrentCommand(session); p == "" || tmux.SafeShells[p] {
			return "settled"
		}
		return "wait"
	}
	res := svc.RunCommand(prompts.RunInput{Session: session, Provider: prompts.ProviderClaude, Line: command})
	code := ""
	if e, ok := res.Body.(*prompts.RunError); ok {
		code = e.Error
	}
	return pluginreload.Outcome(res.Status, code)
}
