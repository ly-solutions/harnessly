package convo

import (
	"context"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// A message from another agent (internal/peers) enters a conversation here and only here: the same outbox as the
// person's messages — so it waits, is typed and is confirmed the same way — under a client id of its own, from a device
// of its own. Nothing in the outbox or the typist knows the difference; the feed tells it by the envelope in the text.
const (
	// PeerClientPrefix starts the client id of every message from another agent.
	PeerClientPrefix = "peer-"
	// PeerDevice is the device such a message is sent from.
	PeerDevice = "peer"
)

// IsPeerClientID says whether a message of the outbox came from another agent.
func IsPeerClientID(id string) bool { return strings.HasPrefix(id, PeerClientPrefix) }

// DeliverPeer puts a message from another agent into the queue of convID. With steer it goes into the running turn,
// read at the agent's next step (DeliverPeerAtOnce: at once), and, when no turn runs or
// the agent cannot take it now (it asks for a permission or asks the person a question, it compacts, its box holds
// someone's text), it waits at the head of the line and goes in as soon as the agent can take it. Without steer it
// waits at the head of the line for the turn to end. The answer is the send's own result; PeerOutcome says where the
// message went.
func (r *Runtime) DeliverPeer(ctx context.Context, convID, clientID, text string, steer bool) wire.CmdResult {
	if !IsPeerClientID(clientID) {
		return wire.Refused(wire.RefuseBadRequest)
	}
	// The mode does not depend on the agent's state at this moment: a steered message that found a permission menu is
	// still steered when the turn goes on, and one that finds the agent free is typed as any message is.
	mode := wire.ModeNow
	if steer {
		mode = wire.ModeSteer
	}
	return r.Do(ctx, PeerDevice, wire.SendCmd{OpID: clientID, ClientID: clientID, ConversationID: convID, Text: text,
		Mode: mode, Kind: wire.KindPrompt, AttachmentIDs: []string{}})
}

// DeliverPeerAtOnce is DeliverPeer steered and urgent: in a running turn the tool the agent runs is handed to the
// background, so the message is read now and not when that tool ends (outbox.Record.Soft). It is this machine's own
// delegation that may ask for it — a message from another machine never does: its id and text are its sender's, and
// neither can carry the mark.
func (r *Runtime) DeliverPeerAtOnce(ctx context.Context, convID, clientID, text string) wire.CmdResult {
	if !IsPeerClientID(clientID) {
		return wire.Refused(wire.RefuseBadRequest)
	}
	mark := func(on bool) error {
		_, err := live(r, ctx, convID, func(c *conv, _ time.Time) (struct{}, error) {
			if !on {
				delete(c.peerAtOnce, clientID)
				return struct{}{}, nil
			}
			if c.peerAtOnce == nil {
				c.peerAtOnce = map[string]struct{}{}
			}
			c.peerAtOnce[clientID] = struct{}{}
			return struct{}{}, nil
		})
		return err
	}
	if err := mark(true); err != nil {
		return wire.Refused(refusal(err))
	}
	res := r.DeliverPeer(ctx, convID, clientID, text, true)
	if res.R == wire.ResultRefused {
		_ = mark(false) // the send never came for it
	}
	return res
}

// PeerWhere is where a message from another agent stands.
type PeerWhere string

const (
	// PeerInTurn: the agent has it inside its running turn (typed in, read at its next step, or read already).
	PeerInTurn PeerWhere = "in-turn"
	// PeerStarted: the agent was free and took it as a new turn.
	PeerStarted PeerWhere = "started"
	// PeerQueued: it waits in the line; Reason says for what.
	PeerQueued PeerWhere = "queued"
	// PeerFailed: it will not be delivered (failed, cancelled, unknown).
	PeerFailed PeerWhere = "failed"
)

// PeerOutcome is what became of a message from another agent.
type PeerOutcome struct {
	Where  PeerWhere
	Reason wire.QueueReason
}

// peerOutcomePoll is how often PeerOutcome looks while the message is on its way to the agent.
const peerOutcomePoll = 100 * time.Millisecond

// PeerOutcome waits, at most wait, until the message clientID of convID is with the agent or is held in the line for a
// reason that will not pass by itself in a moment, and says which. A message still being typed when the wait ends is
// reported as queued behind a busy agent: it is on its way and is never typed twice.
func (r *Runtime) PeerOutcome(ctx context.Context, convID, clientID string, wait time.Duration) PeerOutcome {
	// The wall clock: the wait is the caller's patience, not the conversation's time.
	deadline := time.Now().Add(wait)
	for {
		out, settled := r.peerOutcome(ctx, convID, clientID)
		if settled || !time.Now().Before(deadline) {
			return out
		}
		select {
		case <-ctx.Done():
			return out
		case <-time.After(peerOutcomePoll):
		}
	}
}

func (r *Runtime) peerOutcome(ctx context.Context, convID, clientID string) (PeerOutcome, bool) {
	type look struct {
		st               wire.MessageStatus
		steered, working bool
	}
	got, err := live(r, ctx, convID, func(c *conv, _ time.Time) (look, error) {
		return look{c.box.Status([]string{clientID})[0], c.box.Steered(clientID), c.rec.State.K == wire.StateWorking}, nil
	})
	if err != nil || got.st.K != wire.StatusKnown || got.st.Message == nil {
		return PeerOutcome{Where: PeerFailed}, true
	}
	return peerStanding(*got.st.Message, got.steered, got.working)
}

// peerStanding reads a message's state for its sender: where it is, and whether that is settled or may change in a
// moment (a message just queued is typed at the next look of the screen; one typed is confirmed a moment later).
func peerStanding(m wire.Message, steered, working bool) (PeerOutcome, bool) {
	switch m.State.K {
	case wire.MsgQueued:
		switch m.State.Reason {
		case wire.QueuedBlockedMenu, wire.QueuedBlockedText, wire.QueuedPaused, wire.QueuedStalled, wire.QueuedLaunching, wire.QueuedResuming:
			// A menu, someone's text, a pause, a start: these do not pass in a moment, and the sender is told why.
			return PeerOutcome{Where: PeerQueued, Reason: m.State.Reason}, true
		}
		// Behind a working agent a message for the turn's end stays where it is; a steered one, and any message to a
		// free agent, is about to be typed.
		return PeerOutcome{Where: PeerQueued, Reason: wire.QueuedAgentBusy}, working && m.Mode != wire.ModeSteer
	case wire.MsgTyping:
		return PeerOutcome{Where: PeerQueued, Reason: wire.QueuedAgentBusy}, false
	case wire.MsgInInput, wire.MsgAccepted, wire.MsgInTurn, wire.MsgDone:
		if steered {
			return PeerOutcome{Where: PeerInTurn}, true
		}
		// In the box of a free agent, its Enter not confirmed yet: a moment more says it was taken.
		return PeerOutcome{Where: PeerStarted}, m.State.K != wire.MsgInInput
	}
	return PeerOutcome{Where: PeerFailed}, true
}
