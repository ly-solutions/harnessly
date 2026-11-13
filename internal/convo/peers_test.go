package convo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/peers"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// A message from another agent goes through the conversation's outbox like the person's — typed, accepted, read in the
// turn — but it is the outbox's peer message (its client id, its device), and its user entry is the envelope the feed
// draws as another agent's card, never the person's own words.
func TestPeerMessageGoesThroughTheOutbox(t *testing.T) {
	r := newRig(t)
	r.open()
	r.settle(wire.StateReady)
	r.agentTakes()

	text := peers.Wrap(peers.Envelope{ID: "pm_abcdef", Person: "Иван", Machine: "dev-department",
		Chat: "Модель по умолчанию", Ref: "dev-department/tab-rsk", Via: "agent", Text: "Проверь модель по умолчанию."})
	res := r.rt.DeliverPeer(context.Background(), r.id, "peer-pm_abcdef", text, true)
	if res.R != wire.ResultQueued {
		t.Fatalf("deliver: %+v", res)
	}
	m := r.message("peer-pm_abcdef")
	// Steered whatever the agent does now: a free agent is typed into as for any message, and one that asks for a
	// permission gets it inside the turn that goes on after the answer.
	if !IsPeerClientID(m.ClientID) || m.Mode != wire.ModeSteer || m.Kind != wire.KindPrompt {
		t.Fatalf("an idle agent gets it at the head of the line as a prompt: %+v", m)
	}
	r.tick(time.Second)
	r.hook("prompt-submit")
	var user *wire.Entry
	for i, e := range r.snap().Entries {
		if e.T == wire.EntryUser {
			user = &r.snap().Entries[i]
		}
	}
	if user == nil {
		t.Fatal("no user entry for the peer message")
	}
	env, ok := peers.Parse(user.Text)
	if !ok || env.Person != "Иван" || env.Machine != "dev-department" || env.Ref != "dev-department/tab-rsk" ||
		!strings.Contains(env.Text, "модель по умолчанию") {
		t.Fatalf("the entry is not the envelope: %q", user.Text)
	}
	if user.ClientID == nil || *user.ClientID != "peer-pm_abcdef" {
		t.Fatalf("the entry does not carry the peer message: %+v", user.ClientID)
	}
}

// Delivery «at the next step»: a working agent gets the message steered into its turn; «after the turn» it waits.
func TestPeerMessageToAWorkingAgent(t *testing.T) {
	r := newRig(t)
	r.user("first")
	r.busy()
	r.open()
	r.settle(wire.StateWorking)

	if res := r.rt.DeliverPeer(context.Background(), r.id, "peer-pm_steer1", "<peer-message>x</peer-message>", true); res.R != wire.ResultQueued {
		t.Fatalf("steer: %+v", res)
	}
	if m := r.message("peer-pm_steer1"); m.Mode != wire.ModeSteer {
		t.Fatalf("at the next step means steered: %s", m.Mode)
	}
	if res := r.rt.DeliverPeer(context.Background(), r.id, "peer-pm_turn01", "<peer-message>y</peer-message>", false); res.R != wire.ResultQueued {
		t.Fatalf("turn: %+v", res)
	}
	if m := r.message("peer-pm_turn01"); m.Mode != wire.ModeNow {
		t.Fatalf("after the turn means the head of the line: %s", m.Mode)
	}
}

// The sender is told where its message went: into the running turn, or into the line and why.
func TestPeerOutcomeSaysWhereTheMessageWent(t *testing.T) {
	r := newRig(t)
	r.user("first")
	r.busy()
	r.open()
	r.settle(wire.StateWorking)
	ctx := context.Background()

	if res := r.rt.DeliverPeer(ctx, r.id, "peer-pm_turn02", "<peer-message>after</peer-message>", false); res.R != wire.ResultQueued {
		t.Fatalf("turn: %+v", res)
	}
	if o := r.rt.PeerOutcome(ctx, r.id, "peer-pm_turn02", 0); o.Where != PeerQueued || o.Reason != wire.QueuedAgentBusy {
		t.Fatalf("a message for the turn's end: %+v", o)
	}
	if res := r.rt.DeliverPeerAtOnce(ctx, r.id, "peer-pm_steer2", "<peer-message>now</peer-message>"); res.R != wire.ResultQueued {
		t.Fatalf("steer: %+v", res)
	}
	r.tick(time.Second)
	if o := r.rt.PeerOutcome(ctx, r.id, "peer-pm_steer2", 0); o.Where != PeerInTurn {
		t.Fatalf("a steered message, typed into the working agent: %+v (%+v)", o, r.message("peer-pm_steer2").State)
	}
	if m := r.message("peer-pm_turn02"); m.State.K != wire.MsgQueued {
		t.Fatalf("the message for the turn's end went into the turn: %+v", m.State)
	}
	if o := r.rt.PeerOutcome(ctx, r.id, "peer-pm_nosuch", 0); o.Where != PeerFailed {
		t.Fatalf("an unknown message: %+v", o)
	}
}

// Only this machine's delegation makes a message urgent (the running tool handed over for it). A message
// whose sender chose an id that looks like a mark — another machine's — is read at the next step like any other.
func TestOnlyTheRuntimeMarksAPeerMessageUrgent(t *testing.T) {
	r := newRig(t)
	r.user("first")
	r.busy()
	r.open()
	r.settle(wire.StateWorking)
	ctx := context.Background()
	soft := func(id string) bool {
		t.Helper()
		got, err := live(r.rt, ctx, r.id, func(c *conv, _ time.Time) (bool, error) { return c.box.Soft(id), nil })
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for _, id := range []string{"peer-now-abcdef", "peer-pm_plain1"} {
		if res := r.rt.DeliverPeer(ctx, r.id, id, "<peer-message>x</peer-message>", true); res.R != wire.ResultQueued {
			t.Fatalf("%s: %+v", id, res)
		}
		if !soft(id) {
			t.Fatalf("%s was taken for urgent by its id", id)
		}
	}
	if res := r.rt.DeliverPeerAtOnce(ctx, r.id, "peer-pm_urgent", "<peer-message>y</peer-message>"); res.R != wire.ResultQueued {
		t.Fatalf("at once: %+v", res)
	}
	if soft("peer-pm_urgent") {
		t.Fatal("the delegation's urgent message is not urgent")
	}
	if left, _ := live(r.rt, ctx, r.id, func(c *conv, _ time.Time) (int, error) { return len(c.peerAtOnce), nil }); left != 0 {
		t.Fatalf("marks left behind: %d", left)
	}
}

// What the sender is told, state by state: a message on its way is not called queued (it is typed a moment
// after it is sent), and one held by a menu is, with the reason.
func TestPeerStanding(t *testing.T) {
	msg := func(k wire.MessageStateKind, reason wire.QueueReason, mode wire.MessageMode) wire.Message {
		return wire.Message{Mode: mode, State: wire.MessageState{K: k, Reason: reason}}
	}
	for _, tc := range []struct {
		name             string
		m                wire.Message
		steered, working bool
		where            PeerWhere
		reason           wire.QueueReason
		settled          bool
	}{
		{"just queued for a working agent, steered", msg(wire.MsgQueued, wire.QueuedBehind, wire.ModeSteer), false, true, PeerQueued, wire.QueuedAgentBusy, false},
		{"just queued for a free agent", msg(wire.MsgQueued, wire.QueuedBehind, wire.ModeSteer), false, false, PeerQueued, wire.QueuedAgentBusy, false},
		{"being typed", msg(wire.MsgTyping, "", wire.ModeSteer), true, true, PeerQueued, wire.QueuedAgentBusy, false},
		{"typed into the running turn", msg(wire.MsgInInput, "", wire.ModeSteer), true, true, PeerInTurn, "", true},
		{"read in the running turn", msg(wire.MsgInTurn, "", wire.ModeSteer), true, true, PeerInTurn, "", true},
		{"in a free agent's box", msg(wire.MsgInInput, "", wire.ModeSteer), false, false, PeerStarted, "", false},
		{"taken by a free agent", msg(wire.MsgAccepted, "", wire.ModeSteer), false, true, PeerStarted, "", true},
		{"for the turn's end", msg(wire.MsgQueued, wire.QueuedAgentBusy, wire.ModeNow), false, true, PeerQueued, wire.QueuedAgentBusy, true},
		{"held by a permission menu", msg(wire.MsgQueued, wire.QueuedBlockedMenu, wire.ModeSteer), false, false, PeerQueued, wire.QueuedBlockedMenu, true},
		{"held by a menu, the turn goes on", msg(wire.MsgQueued, wire.QueuedBlockedMenu, wire.ModeSteer), false, true, PeerQueued, wire.QueuedBlockedMenu, true},
		{"held by someone's text", msg(wire.MsgQueued, wire.QueuedBlockedText, wire.ModeSteer), false, true, PeerQueued, wire.QueuedBlockedText, true},
		{"the agent starts", msg(wire.MsgQueued, wire.QueuedLaunching, wire.ModeSteer), false, false, PeerQueued, wire.QueuedLaunching, true},
		{"the queue is paused", msg(wire.MsgQueued, wire.QueuedPaused, wire.ModeSteer), false, false, PeerQueued, wire.QueuedPaused, true},
		{"failed", msg(wire.MsgFailed, "", wire.ModeSteer), false, false, PeerFailed, "", true},
		{"taken into a later message", msg(wire.MsgCancelled, "", wire.ModeSteer), false, false, PeerFailed, "", true},
	} {
		got, settled := peerStanding(tc.m, tc.steered, tc.working)
		if got.Where != tc.where || got.Reason != tc.reason || settled != tc.settled {
			t.Errorf("%s: %+v settled %v, want %s %q settled %v", tc.name, got, settled, tc.where, tc.reason, tc.settled)
		}
	}
}

func TestPeerMessageRefusals(t *testing.T) {
	r := newRig(t)
	r.open()
	if res := r.rt.DeliverPeer(context.Background(), "tab-nope", "peer-pm_000001", "x", true); res.Code != wire.RefuseNotFound {
		t.Fatalf("unknown conversation: %+v", res)
	}
	if res := r.rt.DeliverPeer(context.Background(), r.id, "m1", "x", true); res.Code != wire.RefuseBadRequest {
		t.Fatalf("a person's client id is not a peer message: %+v", res)
	}
}
