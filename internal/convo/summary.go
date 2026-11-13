package convo

import (
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/attach"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// summarize asks the cheap model, once a turn, for a few lines on what the agent did in the conversation's last turn
// The answer is accepted at once; the lines come later on the record as its recap, as Claude's own «while you
// were away» does, and stand like it until the person writes again. A recap Claude wrote after the turn ended is kept:
// the model is not asked for what is there.
func (c *conv) summarize(turnID string, now time.Time) wire.CmdResult {
	c.mark, c.markConvs = 0, 0
	if c.deletedAt != 0 {
		return wire.Refused(wire.RefuseNotFound)
	}
	if c.d.Summarizer == nil || c.summaryPick().Off() {
		return wire.Refused(wire.RefuseUnsupported)
	}
	t := c.rec.LastTurn
	if t == nil || t.ID != turnID || t.Status != wire.TurnDone || t.EndedAt == nil {
		return wire.Refused(wire.RefuseBadRequest) // only the last turn, and only one that is done
	}
	if c.summarized == turnID {
		return c.recResult(now)
	}
	if r := c.rec.Recap; r != nil && r.At >= *t.EndedAt && c.recapStands(r) {
		c.summarized = turnID
		return c.recResult(now)
	}
	message, replies := c.turnTexts(turnID)
	if len(replies) == 0 {
		return wire.Refused(wire.RefuseBadRequest) // the turn said nothing to sum up
	}
	c.summarized = turnID
	summarizer, provider, model := c.d.Summarizer, c.rec.Provider, ""
	if pick := c.summaryPick(); !pick.Auto() {
		provider, model = wire.Provider(pick.Provider), pick.Model
	}
	c.spawn(func() any {
		text, err := summarizer(provider, model, message, replies)
		if err != nil {
			return err
		}
		return text
	}, func(v any) {
		text, ok := v.(string)
		if !ok {
			c.d.Log.Warn("turn summary not made", "conv", c.rec.ID, "err", v)
			return
		}
		c.applySummary(turnID, strings.TrimSpace(text))
	})
	return c.recResult(now)
}

func (c *conv) summaryPick() *wire.NoticeSummaryModel {
	if c.d.SummaryModel == nil {
		return nil
	}
	return c.d.SummaryModel()
}

// summarizeEnded sums up the turn that has just ended, without the page asking: the work runs in the
// background and the record gets the recap when it is made. A delegated chat is not summed up — nobody reads its
// «готово» — and nor is a turn that did not end done; the page's own ask stays as the fallback and finds the turn
// asked already.
func (c *conv) summarizeEnded(now time.Time) {
	t := c.rec.LastTurn
	if t == nil || t.Status != wire.TurnDone || c.rec.ParentID != nil || c.d.Summarizer == nil {
		return
	}
	// summarize starts as a command does (it clears the marks of what was published); the turn may end inside another
	// command, whose answer needs its marks.
	mark, markConvs := c.mark, c.markConvs
	c.summarize(t.ID, now)
	c.mark, c.markConvs = mark, markConvs
}

// applySummary puts the turn's summary on the record, unless the chat moved on or Claude wrote a recap of its own after
// the turn meanwhile.
func (c *conv) applySummary(turnID, text string) {
	t := c.rec.LastTurn
	if text == "" || t == nil || t.ID != turnID || t.EndedAt == nil {
		return
	}
	now := c.d.Clock.Now()
	r := &wire.Recap{Text: text, At: ms(now)}
	c.summary = r
	if cur := c.rec.Recap; cur != nil && cur.At >= *t.EndedAt && c.recapStands(cur) {
		return
	}
	c.rec.Recap = r
	c.touch()
	c.flush(now, true)
}

// turnTexts are the person's message that opened a turn and what the agent said in it, in order: its replies only,
// never what the tools were given or gave back.
func (c *conv) turnTexts(turnID string) (message string, replies []string) {
	_, entries, _ := c.fd.Snapshot(1 << 20)
	for _, e := range entries {
		if e.ID != turnID && (e.TurnID == nil || *e.TurnID != turnID) {
			continue
		}
		switch e.T {
		case wire.EntryUser:
			if message == "" {
				message = strings.TrimSpace(attach.Strip(e.Text))
			}
		case wire.EntrySay:
			if md := strings.TrimSpace(e.MD); md != "" {
				replies = append(replies, md)
			}
		}
	}
	return message, replies
}

// summaryOver is the recap the record shows when the transcript has recap: the turn's own summary while it is the newer
// and still stands, else the transcript's.
func (c *conv) summaryOver(recap *wire.Recap) *wire.Recap {
	if s := c.summary; s != nil && c.recapStands(s) && (recap == nil || s.At > recap.At) {
		return s
	}
	return recap
}
