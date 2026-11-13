package convo

import (
	"testing"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// A stopped chat, or one whose launch failed, is not looked at every ten seconds once a look found no tmux session of
// it; its clock keeps its pace, and a gone chat is still watched for its session coming back.
func TestAbsentSessionIsLookedAtSeldom(t *testing.T) {
	for _, k := range []wire.AgentStateKind{wire.StateStopped, wire.StateLaunchFailed} {
		c := &conv{}
		c.rec.State.K = k
		if got := c.lookEvery(); got != LookGone {
			t.Fatalf("%s before any look: %s", k, got)
		}
		c.looked = true
		if got := c.lookEvery(); got != LookAbsent {
			t.Fatalf("%s with no session: %s", k, got)
		}
		if got := c.clockEvery(); got != LookGone {
			t.Fatalf("%s clock: %s", k, got)
		}
		c.probe.Session = true
		if got := c.lookEvery(); got != LookGone {
			t.Fatalf("%s with a session: %s", k, got)
		}
	}
	c := &conv{looked: true}
	c.rec.State.K = wire.StateGone
	if got := c.lookEvery(); got != LookGone {
		t.Fatalf("gone: %s", got)
	}
}
