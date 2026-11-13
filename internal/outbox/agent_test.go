package outbox

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// A message one agent sends another goes into the running turn of the agent it is for instead of waiting for
// that turn to end. These are the states it can find the agent in, and in each of them the message is typed at most
// once and never lost.

// sendAgent queues another agent's message, steered: read at the agent's next step (soft) or at once.
func (r *rig) sendAgent(id, text string, soft bool) wire.RefuseCode {
	r.t.Helper()
	ch, code := r.box.Send(Draft{ClientID: id, Text: text, Mode: wire.ModeSteer, Kind: wire.KindPrompt, Agent: true, Soft: soft},
		r.agent.State, r.clock.Now())
	r.record(ch)
	return code
}

// went counts the times a text went into the pane, typed as a line or pasted.
func (r *rig) went(text string) int {
	n := 0
	for _, c := range r.typed() {
		if strings.HasPrefix(c, "line:") || strings.HasPrefix(c, "paste:") {
			n += strings.Count(c, text)
		}
	}
	return n
}

func (r *rig) to(k wire.AgentStateKind, screenText string) {
	r.agent.State.K = k
	r.pane.screen = screenText
}

// The turn runs: the message is typed into it at once, waits in the agent's own queue for its next step, and is in the
// turn when the transcript shows it — the line never held it for the turn's end.
func TestAnAgentMessageGoesIntoTheRunningTurn(t *testing.T) {
	r := newRig(t, tui(t, "claude-busy-spinner.ansi.txt"), wire.StateWorking)
	r.leaves()
	r.sendAgent("peer-1", "<peer-message chat=\"Координатор\">rebase first</peer-message>", true)
	r.pump()
	if st := r.state("peer-1"); st.K != wire.MsgInInput || !r.box.Steered("peer-1") {
		t.Fatalf("not typed into the turn: %s", r.trail("peer-1"))
	}
	if n := len(r.box.Queue().Items); n != 0 {
		t.Fatalf("the queue still counts a message the agent has: %d", n)
	}
	_, ch := r.box.UserEntry("c:bb#0", 10, turn("c:aa#0"), r.clock.Now())
	r.record(ch)
	if st := r.state("peer-1"); st.K != wire.MsgInTurn || st.TurnID != "c:aa#0" {
		t.Fatalf("not in the running turn: %+v", st)
	}
	r.advance(time.Hour)
	if r.went("rebase first") != 1 {
		t.Fatalf("typed %d times: %v", r.went("rebase first"), r.typed())
	}
}

// The agent is free: the message starts a turn as any message does, and is not marked as steered.
func TestAnAgentMessageToAFreeAgentStartsATurn(t *testing.T) {
	r := hookRig(t, tui(t, "claude-idle-placeholder.ansi.txt"))
	r.leaves()
	r.sendAgent("peer-1", "run the suite", true)
	r.pump()
	if st := r.state("peer-1"); st.K != wire.MsgInInput || r.box.Steered("peer-1") {
		t.Fatalf("%s steered %v", r.trail("peer-1"), r.box.Steered("peer-1"))
	}
	r.hook("run the suite")
	if st := r.state("peer-1"); st.K != wire.MsgAccepted {
		t.Fatalf("%+v", st)
	}
	r.advance(time.Hour)
	if r.went("run the suite") != 1 {
		t.Fatalf("typed %d times", r.went("run the suite"))
	}
}

// The agent asks for a permission: nothing is typed into the menu, the message stays in the line, and it goes into the
// same turn once the person has answered — it does not wait for the turn's end.
func TestAnAgentMessageWaitsOutAPermissionMenu(t *testing.T) {
	r := newRig(t, permissionScreen, wire.StateWaitingPermission)
	r.leaves()
	r.sendAgent("peer-1", "stop and report", true)
	r.pump()
	r.advance(time.Minute)
	if st := r.state("peer-1"); st.K != wire.MsgQueued || st.Reason != wire.QueuedBlockedMenu || len(r.typed()) != 0 {
		t.Fatalf("%+v typed %v", st, r.typed())
	}
	if got := r.box.Queue().Items; !reflect.DeepEqual(got, []string{"peer-1"}) {
		t.Fatalf("queue %v", got)
	}
	// The screen may still show the menu for a moment after the state says the agent works: still nothing is typed.
	r.agent.State.K = wire.StateWorking
	r.pump()
	if len(r.typed()) != 0 {
		t.Fatalf("typed into a menu: %v", r.typed())
	}
	r.to(wire.StateWorking, tui(t, "claude-busy-spinner.ansi.txt"))
	r.advance(blockedMax)
	if st := r.state("peer-1"); st.K != wire.MsgInInput || !r.box.Steered("peer-1") || r.went("stop and report") != 1 {
		t.Fatalf("after the answer: %s, typed %v", r.trail("peer-1"), r.typed())
	}
}

// The agent asks the person a question. The person's own message answers it in words; another agent's message never
// does — it waits for the dialog to close.
func TestAnAgentMessageNeverAnswersThePersonsQuestion(t *testing.T) {
	question := tui(t, "claude-2.1.322-question.ansi.txt")
	r := newRig(t, question, wire.StateWaitingQuestion)
	r.sendAgent("peer-1", "the build is green", true)
	r.pump()
	r.advance(time.Minute)
	if st := r.state("peer-1"); st.K != wire.MsgQueued || st.Reason != wire.QueuedBlockedMenu || len(r.typed()) != 0 {
		t.Fatalf("%+v typed %v", st, r.typed())
	}
	// The state lags behind the screen: the dialog alone holds the message too.
	r.agent.State.K = wire.StateWorking
	r.advance(time.Minute)
	if len(r.typed()) != 0 {
		t.Fatalf("typed into the question: %v", r.typed())
	}
	r.leaves()
	r.to(wire.StateWorking, tui(t, "claude-busy-spinner.ansi.txt"))
	r.advance(blockedMax)
	if r.state("peer-1").K != wire.MsgInInput || r.went("the build is green") != 1 {
		t.Fatalf("after the answer: %s, typed %v", r.trail("peer-1"), r.typed())
	}
}

// A compaction reads nothing: the message waits it out in the line and goes into the turn when it is over.
func TestAnAgentMessageWaitsOutACompaction(t *testing.T) {
	r := newRig(t, tui(t, "claude-busy-spinner.ansi.txt"), wire.StateWorking)
	r.leaves()
	r.agent.State.Compacting = true
	r.sendAgent("peer-1", "use the new branch", true)
	r.pump()
	r.advance(time.Minute)
	if st := r.state("peer-1"); st.K != wire.MsgQueued || len(r.typed()) != 0 {
		t.Fatalf("typed into a compaction: %+v %v", st, r.typed())
	}
	r.agent.State.Compacting = false
	r.pump()
	if r.state("peer-1").K != wire.MsgInInput || !r.box.Steered("peer-1") || r.went("use the new branch") != 1 {
		t.Fatalf("after the compaction: %s, typed %v", r.trail("peer-1"), r.typed())
	}
}

// The typing fails (the terminal did not take the keys): the message is back in the line, not lost and not failed, and
// a later try types it once.
func TestAnAgentMessageWhoseTypingFailedStaysInTheLine(t *testing.T) {
	r := newRig(t, tui(t, "claude-busy-spinner.ansi.txt"), wire.StateWorking)
	r.leaves()
	r.pane.fail = "line:"
	r.sendAgent("peer-1", "check the logs", true)
	r.pump()
	if st := r.state("peer-1"); st.K != wire.MsgQueued {
		t.Fatalf("after the failed try: %s", r.trail("peer-1"))
	}
	if got := r.box.Queue().Items; !reflect.DeepEqual(got, []string{"peer-1"}) {
		t.Fatalf("queue %v", got)
	}
	r.pane.fail = ""
	r.advance(retryMax)
	if r.state("peer-1").K != wire.MsgInInput || r.went("check the logs") != 1 {
		t.Fatalf("after the retry: %s, typed %v", r.trail("peer-1"), r.typed())
	}
	r.advance(time.Minute)
	if r.went("check the logs") != 1 {
		t.Fatalf("typed twice: %v", r.typed())
	}
}

// The same message sent again (its answer was lost on the way) is the message already there: one record, typed once.
func TestAnAgentMessageSentTwiceIsTypedOnce(t *testing.T) {
	r := newRig(t, tui(t, "claude-busy-spinner.ansi.txt"), wire.StateWorking)
	r.leaves()
	r.sendAgent("peer-1", "merge it", true)
	r.pump()
	if code := r.sendAgent("peer-1", "merge it", true); code != "" {
		t.Fatalf("the second send: %s", code)
	}
	r.pump()
	r.advance(time.Minute)
	if r.went("merge it") != 1 || len(r.box.Queue().Items) != 0 {
		t.Fatalf("typed %d times, queue %v", r.went("merge it"), r.box.Queue().Items)
	}
}

// Several agents' messages that wait together reach the agent as one message, in the order they came; the person's
// queued words are not pulled into the running turn with them, and keep their place for the turn's end.
func TestAgentMessagesThatWaitTogetherGoInAsOne(t *testing.T) {
	r := newRig(t, permissionScreen, wire.StateWaitingPermission)
	r.leaves()
	r.send("m0", "the person's next task", wire.ModeQueue)
	r.sendAgent("peer-1", "<peer-message chat=\"A\">first</peer-message>", true)
	r.sendAgent("peer-2", "<peer-message chat=\"B\">second</peer-message>", true)
	r.sendAgent("peer-3", "<peer-message chat=\"A\">third</peer-message>", true)
	if got := r.box.Queue().Items; !reflect.DeepEqual(got, []string{"peer-3", "m0"}) {
		t.Fatalf("queue %v", got)
	}
	for _, id := range []string{"peer-1", "peer-2"} {
		if st := r.state(id); st.K != wire.MsgCancelled {
			t.Fatalf("%s was not taken into the one message: %+v", id, st)
		}
	}
	m, _ := r.box.Message("peer-3")
	want := "<peer-message chat=\"A\">first</peer-message>\n\n<peer-message chat=\"B\">second</peer-message>\n\n<peer-message chat=\"A\">third</peer-message>"
	if m.Text != want {
		t.Fatalf("text %q", m.Text)
	}
	r.to(wire.StateWorking, tui(t, "claude-busy-spinner.ansi.txt"))
	r.advance(blockedMax)
	if r.state("peer-3").K != wire.MsgInInput || r.jobs != 1 {
		t.Fatalf("%s, %d tries", r.trail("peer-3"), r.jobs)
	}
	for _, word := range []string{"first", "second", "third"} {
		if r.went(">"+word+"<") != 1 {
			t.Fatalf("%q went in %d times: %v", word, r.went(">"+word+"<"), r.typed())
		}
	}
	if st := r.state("m0"); st.K != wire.MsgQueued || r.went("the person's next task") != 0 {
		t.Fatalf("the person's queued message went into the turn: %+v", st)
	}
}

// A message already with the agent is not taken into a later one: it would be read twice.
func TestAnAgentMessageAlreadyTypedIsNotGatheredAgain(t *testing.T) {
	r := newRig(t, tui(t, "claude-busy-spinner.ansi.txt"), wire.StateWorking)
	r.leaves()
	r.sendAgent("peer-1", "first", true)
	r.pump()
	r.sendAgent("peer-2", "second", true)
	r.pump()
	if m, _ := r.box.Message("peer-2"); m.Text != "second" || r.state("peer-1").K != wire.MsgInInput {
		t.Fatalf("text %q, first %s", m.Text, r.trail("peer-1"))
	}
	if r.went("first") != 1 || r.went("second") != 1 {
		t.Fatalf("typed %v", r.typed())
	}
}

// Read at the next step by default: the tool the agent runs is handed to the background only for a message its sender
// marked as urgent, and one urgent message among those gathered makes the whole urgent.
func TestOnlyAnUrgentAgentMessageHandsTheRunningToolOver(t *testing.T) {
	job := func(soft ...bool) *Job {
		r := newRig(t, tui(t, "claude-busy-spinner.ansi.txt"), wire.StateWorking)
		r.agent.HandOver = true
		// Held by a compaction, so they wait together and go in as one.
		r.agent.State.Compacting = true
		for i, s := range soft {
			r.sendAgent("peer-"+string(rune('a'+i)), "word", s)
		}
		r.agent.State.Compacting = false
		return r.try()
	}
	if j := job(true); !j.Message.Steer || j.Message.HandOver || !j.Message.Agent {
		t.Fatalf("a message for the next step: %+v", j.Message)
	}
	if j := job(false); !j.Message.HandOver {
		t.Fatalf("an urgent message: %+v", j.Message)
	}
	if j := job(false, true); !j.Message.HandOver {
		t.Fatalf("an urgent message gathered into a later one: %+v", j.Message)
	}
}
