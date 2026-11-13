package outbox

import (
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// The first prompt a launch carried, failed by its term before the launch was taken for failed, is first in the line
// again once the launch fails, queued: the next launch carries it. One that failed before this launch
// began stays the person's to retry.
func TestAFirstPromptLostToItsTermGoesBackWhenTheLaunchFails(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	b := New("tab-1", wire.DefaultLimits, nil)
	for _, id := range []string{"first", "m2"} {
		if _, code := b.Send(Draft{ClientID: id, Text: "do the task " + id, Mode: wire.ModeQueue}, wire.AgentState{K: wire.StateStarting}, now); code != "" {
			t.Fatalf("send %s: %s", id, code)
		}
	}
	if _, ok := b.Carry("first", now); !ok {
		t.Fatal("not carried")
	}
	launched := ms(now)
	// Nothing says the launch is under way (the runtime was held up past the term): the carried message fails.
	failedAt := now.Add(CarryDeadline + time.Second)
	b.Tick(failedAt)
	if m, _ := b.Message("first"); m.State.K != wire.MsgFailed || m.State.Code != wire.MsgFailedUnconfirmed {
		t.Fatalf("first after its term: %+v", m.State)
	}
	later := failedAt.Add(time.Minute)
	b.Pause(wire.PausedLaunchFailed, later)
	if ch := b.ReturnLostFirst(ms(failedAt)+1, later); len(ch.Events) != 0 || b.Head() != "m2" {
		t.Fatalf("a first prompt that failed before the launch began came back: head %q", b.Head())
	}
	b.ReturnLostFirst(launched, later)
	m, _ := b.Message("first")
	if b.Head() != "first" || m.State.K != wire.MsgQueued || m.State.Position != 1 || m.State.Reason != wire.QueuedPaused {
		t.Fatalf("first after the launch failed: head %q, %+v", b.Head(), m.State)
	}
	if q := b.Queue(); len(q.Items) != 2 || q.Items[0] != "first" || q.Items[1] != "m2" {
		t.Fatalf("the line: %+v", q)
	}
	// The next launch carries it again.
	if _, ok := b.Resume(later); !ok {
		t.Fatal("not resumed")
	}
	if _, ok := b.Carry("first", later); !ok {
		t.Fatal("the next launch does not carry it")
	}
	// A queue that already stands paused for a failed launch still takes back what a launch carries.
	b.paused = &wire.QueuePause{Reason: wire.PausedLaunchFailed, At: ms(later)}
	b.Pause(wire.PausedLaunchFailed, later)
	if m, _ := b.Message("first"); m.State.K != wire.MsgQueued || b.Head() != "first" {
		t.Fatalf("carried under a standing pause: %+v, head %q", m.State, b.Head())
	}
}
