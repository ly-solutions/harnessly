package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/convo"
	"github.com/ly-solutions/harnessly/internal/outbox"
	"github.com/ly-solutions/harnessly/internal/tmux"
)

// A dead agent tab with nothing in it is not made a chat: its launch would make another tab, which the next start of
// the runtime found left behind in its turn (13.11, a score of empty chats per machine update). A tab with anything to
// take over still is. The tmux server is a private one that never starts, so no session of it runs.
func TestImportSkipsDeadEmptyTabs(t *testing.T) {
	tm := tmux.New(tmux.Names{Socket: "hs-import-test-" + strconv.FormatInt(time.Now().UnixNano(), 36), Prefix: "hs-"})
	v := &v2Live{c: &core{cfg: coreConfig{Tmux: tm}}}
	queues := map[string][]outbox.LegacyItem{"tab-q": {{ID: "m1", Text: "hi"}}}
	for _, c := range []struct {
		tab  convo.ImportTab
		skip bool
	}{
		{convo.ImportTab{ID: "tab-dead", Session: "hs-ws-1-pane-1-tab-dead"}, true},
		{convo.ImportTab{ID: "tab-none"}, true},
		{convo.ImportTab{ID: "tab-s", Session: "hs-ws-1-pane-1-tab-s", AgentSessionID: "a1"}, false},
		{convo.ImportTab{ID: "tab-t", Session: "hs-ws-1-pane-1-tab-t", TranscriptPath: "/t.jsonl"}, false},
		{convo.ImportTab{ID: "tab-q", Session: "hs-ws-1-pane-1-tab-q"}, false},
	} {
		if got := v.nothingToTakeOver(c.tab, queues); got != c.skip {
			t.Errorf("%s: skipped %v, want %v", c.tab.ID, got, c.skip)
		}
	}
}
