package convo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

func TestTheLastTurnIsSummedUpOnceAndTheSummaryIsTheRecap(t *testing.T) {
	r := newRig(t)
	var asked []string
	r.rt.d.Summarizer = func(_ wire.Provider, _ string, message string, replies []string) (string, error) {
		asked = append(asked, message+" | "+strings.Join(replies, " / "))
		return "Сделал форму оплаты.\nЖдёт проверки.", nil
	}
	r.named("first")
	r.settle(wire.StateReady)
	r.user("Сделай форму оплаты")
	r.say("Начинаю")
	r.say("Форма готова")
	r.turnEnd()
	r.tick(time.Second)
	turn := r.record().LastTurn
	if turn == nil || turn.Status != wire.TurnDone || turn.EndedAt == nil {
		t.Fatalf("last turn: %+v", turn)
	}
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm1", ID: r.id, TurnID: turn.ID}); res.R != wire.ResultAccepted {
		t.Fatalf("result: %+v", res)
	}
	if len(asked) != 1 || asked[0] != "Сделай форму оплаты | Начинаю / Форма готова" {
		t.Fatalf("asked: %v", asked)
	}
	rec := r.record()
	if rec.Recap == nil || rec.Recap.Text != "Сделал форму оплаты.\nЖдёт проверки." || rec.Recap.At < *turn.EndedAt {
		t.Fatalf("recap: %+v", rec.Recap)
	}
	// Another page asks too: the model is not asked again.
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm2", ID: r.id, TurnID: turn.ID}); res.R != wire.ResultAccepted || len(asked) != 1 {
		t.Fatalf("again: %+v %v", res, asked)
	}
	// The person writes again: the summary of the turn before is no longer what is happening.
	r.tick(time.Second)
	r.user("Теперь возвраты")
	r.tick(metaEvery)
	r.tick(metaEvery)
	if rec := r.record(); rec.Recap != nil {
		t.Fatalf("the recap stays after the person wrote: %+v", rec.Recap)
	}
}

func TestSummarizeRefusesWhatIsNotTheLastFinishedTurn(t *testing.T) {
	r := newRig(t)
	r.named("first")
	r.settle(wire.StateReady)
	r.user("ask")
	r.say("answer")
	r.turnEnd()
	r.tick(time.Second)
	turn := r.record().LastTurn
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm0", ID: r.id, TurnID: turn.ID}); res.R != wire.ResultRefused || res.Code != wire.RefuseUnsupported {
		t.Fatalf("no summarizer: %+v", res)
	}
	calls := 0
	r.rt.d.Summarizer = func(wire.Provider, string, string, []string) (string, error) {
		calls++
		return "", errors.New("no claude")
	}
	other := strings.TrimSuffix(turn.ID, "1") + "9"
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm1", ID: r.id, TurnID: other}); res.R != wire.ResultRefused || res.Code != wire.RefuseBadRequest {
		t.Fatalf("not the last turn: %+v", res)
	}
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm2", ID: r.id, TurnID: turn.ID}); res.R != wire.ResultAccepted || calls != 1 {
		t.Fatalf("a failed call is accepted: %+v %d", res, calls)
	}
	if rec := r.record(); rec.Recap != nil {
		t.Fatalf("no recap without a summary: %+v", rec.Recap)
	}
}

// finishedTurn runs one turn to its end in the rig, with the summarizer counting what it was asked.
func finishedTurn(r *rig, asked *[]string) {
	r.rt.d.Summarizer = func(p wire.Provider, model, message string, replies []string) (string, error) {
		*asked = append(*asked, string(p)+"/"+model)
		return "Готово.", nil
	}
	r.named("first")
	r.settle(wire.StateReady)
	r.user("Сделай")
	r.say("Сделал")
	r.turnEnd()
	r.tick(time.Second)
}

// the server sums a turn up when it ends, without the page asking; the page's ask after is a no-op.
func TestAFinishedTurnIsSummedUpWithoutThePageAsking(t *testing.T) {
	r := newRig(t)
	var asked []string
	finishedTurn(r, &asked)
	if len(asked) != 1 || asked[0] != "claude/" {
		t.Fatalf("asked: %v", asked)
	}
	if rec := r.record(); rec.Recap == nil || rec.Recap.Text != "Готово." {
		t.Fatalf("recap: %+v", rec.Recap)
	}
	turn := r.record().LastTurn
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm", ID: r.id, TurnID: turn.ID}); res.R != wire.ResultAccepted || len(asked) != 1 {
		t.Fatalf("the page's ask: %+v %v", res, asked)
	}
}

func TestSummaryModelIsTheSettingsPick(t *testing.T) {
	r := newRig(t)
	r.rt.d.SummaryModel = func() *wire.NoticeSummaryModel {
		return &wire.NoticeSummaryModel{Provider: "codex", Model: "gpt-5-mini"}
	}
	var asked []string
	finishedTurn(r, &asked)
	if len(asked) != 1 || asked[0] != "codex/gpt-5-mini" {
		t.Fatalf("asked: %v", asked)
	}
}

func TestSummaryOffAsksNobody(t *testing.T) {
	r := newRig(t)
	r.rt.d.SummaryModel = func() *wire.NoticeSummaryModel { return &wire.NoticeSummaryModel{Provider: wire.NoticeSummaryOff} }
	var asked []string
	finishedTurn(r, &asked)
	if len(asked) != 0 {
		t.Fatalf("asked: %v", asked)
	}
	turn := r.record().LastTurn
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm", ID: r.id, TurnID: turn.ID}); res.R != wire.ResultRefused || len(asked) != 0 {
		t.Fatalf("the page's ask when off: %+v %v", res, asked)
	}
}

func TestADelegatedChatIsNotSummedUp(t *testing.T) {
	r := newRig(t)
	r.named("first")
	if err := r.rt.call(context.Background(), r.id, func(c *conv, now time.Time) {
		parent := "tab-parent"
		c.rec.ParentID = &parent
	}); err != nil {
		t.Fatal(err)
	}
	var asked []string
	finishedTurn(r, &asked)
	if len(asked) != 0 {
		t.Fatalf("asked: %v", asked)
	}
}

// The turn is summed up once: not again after a restart, which finds the recap on the record.
func TestARestartDoesNotSumUpTheSameTurnAgain(t *testing.T) {
	r := newRig(t)
	var asked []string
	finishedTurn(r, &asked)
	if len(asked) != 1 {
		t.Fatalf("asked: %v", asked)
	}
	summarizer := r.rt.d.Summarizer
	r.rt.Stop()
	r.rt = r.runtime()
	r.rt.d.Summarizer = summarizer
	if err := r.rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.tick(time.Second)
	r.tick(metaEvery)
	turn := r.record().LastTurn
	if res := r.do(wire.ConversationSummarizeCmd{OpID: "sm", ID: r.id, TurnID: turn.ID}); res.R != wire.ResultAccepted || len(asked) != 1 {
		t.Fatalf("after the restart: %+v %v", res, asked)
	}
}
