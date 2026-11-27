package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/connectors"
)

// The push of a proposal says whom, where, what and why.
func TestApprovalMessage(t *testing.T) {
	p := connectors.Proposal{ID: "ca_1", Name: "Telegram", Title: connectors.T("Send a message", "Отправить сообщение"),
		Args: []connectors.Pair{{Name: "chat", Value: "Миша Петров"}, {Name: "text", Value: "Привет! Созвон в 15:00"}},
		Why:  "попросил написать Мише", Conv: "tab-a"}
	msg := approvalMessage(p, "hl:", "Чат", time.Unix(0, 0))
	if msg.Kind != "approval" || msg.Tag != "hl:approval-ca_1" || msg.TabID != "tab-a" || msg.Proposal != "ca_1" {
		t.Fatalf("%+v", msg)
	}
	if !strings.Contains(msg.Title, "Telegram: Миша Петров?") {
		t.Fatalf("title %q", msg.Title)
	}
	if !strings.Contains(msg.Body, "«Привет! Созвон в 15:00»") || !strings.Contains(msg.Body, "попросил написать Мише") {
		t.Fatalf("body %q", msg.Body)
	}
	if msg.Token != "" || len(msg.Actions) != 0 {
		t.Fatal("the token and buttons are added by propose only")
	}
}

// The token of a proposal answers it once, only with the right value, only while it is kept.
func TestApprovalTokenIsSingleUse(t *testing.T) {
	now := time.Unix(1000, 0)
	a := &approvals{now: func() time.Time { return now }, sent: map[string]*approvalSent{
		"ca_1": {token: "secret", until: now.Add(time.Minute)},
		"ca_2": {token: "other", until: now.Add(-time.Second)},
	}}
	if a.take("ca_1", "wrong") || a.take("ca_2", "other") || a.take("", "") {
		t.Fatal("a wrong, expired or empty token answered")
	}
	if !a.take("ca_1", "secret") {
		t.Fatal("the right token refused")
	}
	if a.take("ca_1", "secret") {
		t.Fatal("the token answered twice")
	}
}

func TestPushLimiter(t *testing.T) {
	l := &pushLimiter{}
	now := time.Unix(0, 0)
	for i := 0; i < agentPushLimit; i++ {
		if !l.allow(now) {
			t.Fatal(i)
		}
	}
	if l.allow(now) {
		t.Fatal("over the limit")
	}
	if !l.allow(now.Add(agentPushWindow)) {
		t.Fatal("the window passed")
	}
}

// An agent's ask to turn notifications on stands in its chat until it is taken back or runs out.
func TestPushAskStandsUntilItRunsOut(t *testing.T) {
	now := time.Unix(1000, 0)
	a := &approvals{now: func() time.Time { return now }}
	a.ask("tab-a", "чтобы подтверждать отправки", true)
	if got := a.asking(); got["tab-a"].Why != "чтобы подтверждать отправки" {
		t.Fatalf("%+v", got)
	}
	now = now.Add(pushAskTTL + time.Second)
	if got := a.asking(); len(got) != 0 {
		t.Fatalf("ran out: %+v", got)
	}
	a.ask("tab-b", "", true)
	a.ask("tab-b", "", false)
	if got := a.asking(); len(got) != 0 {
		t.Fatalf("taken back: %+v", got)
	}
	if (&approvals{}).deviceCount() != -1 {
		t.Fatal("push off counts -1")
	}
}
