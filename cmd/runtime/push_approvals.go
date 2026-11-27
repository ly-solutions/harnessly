package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ly-solutions/harnessly/internal/agent"
	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/locale"
	"github.com/ly-solutions/harnessly/internal/push"
)

// An action that waits for the person's yes (connectors.Service.OnPropose) is a push to their phone at once, with the
// buttons «Send / Don't send». The buttons answer through /api/push/act with a token made for that one
// proposal: random, kept here only, single-use, gone with the proposal. No agent sees it: it is in no proposal, no CLI
// or MCP answer, only in the encrypted push to the person's own devices. When the proposal is answered anywhere (the
// card, Telegram, a button), the notification is replaced by a quiet one with the outcome.
//
//	POST /api/push/act                 {proposal, verb: approve|decline, token} → {state, result}   (no session: the token)
//	GET  /api/cli/push/devices         {devices}: the person's devices, the most recently used first
//	POST /api/cli/push/send            {device, title, body, url} → {sent}: an agent's push (kind "agent"), rate-limited
//	POST /api/cli/push/ask             {why, wait} → {state: subscribed|waiting, devices}: the agent's chat offers the
//	                                   person to turn notifications on (a tap in the chat), until a device subscribes

// approvalWait bounds how long a proposal is watched for its outcome: its time to live with room.
const approvalWait = 31 * time.Minute

// approvalTextMax bounds the preview of the text a send carries.
const approvalTextMax = 160

type approvalSent struct {
	token string
	subs  []push.Subscription
	until time.Time
	used  bool
}

// approvals are the pushes about proposals of one runtime.
type approvals struct {
	sender  *push.Sender
	svc     *connectors.Service
	titleOf func(conv string) string
	now     func() time.Time

	mu   sync.Mutex
	sent map[string]*approvalSent
	// asks are the chats whose agent asks the person to turn notifications on (harnessly push ask): why, and until.
	asks map[string]pushAsk
}

type pushAsk struct {
	Why   string    `json:"why"`
	Until time.Time `json:"-"`
}

// pushAskTTL is how long a chat offers to turn notifications on after its agent asked.
const pushAskTTL = 15 * time.Minute

// ask makes the chat conv offer to turn notifications on; done takes it back.
func (a *approvals) ask(conv, why string, on bool) {
	a.mu.Lock()
	if a.asks == nil {
		a.asks = map[string]pushAsk{}
	}
	if on {
		a.asks[conv] = pushAsk{Why: push.Clip(why, 200), Until: a.now().Add(pushAskTTL)}
	} else {
		delete(a.asks, conv)
	}
	a.mu.Unlock()
	if a.svc != nil {
		a.svc.Touch()
	}
}

// asking are the chats that offer to turn notifications on now, with why.
func (a *approvals) asking() map[string]pushAsk {
	out := map[string]pushAsk{}
	if a == nil {
		return out
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for conv, ask := range a.asks {
		if now.After(ask.Until) {
			delete(a.asks, conv)
			continue
		}
		out[conv] = ask
	}
	return out
}

// deviceCount is how many devices get this runtime's notifications; -1 when push is off here.
func (a *approvals) deviceCount() int {
	if a == nil || a.sender == nil {
		return -1
	}
	return len(a.sender.Store.Subscriptions())
}

func newApprovals(sender *push.Sender, svc *connectors.Service, titleOf func(string) string) *approvals {
	return &approvals{sender: sender, svc: svc, titleOf: titleOf, now: time.Now, sent: map[string]*approvalSent{}}
}

// pick is the person's language of a connector's text.
func pick(t connectors.Text) string {
	if t.Ru == "" {
		return t.En
	}
	return locale.Pick(t.En, t.Ru)
}

func newPushToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// recipientArgs name whom a send reaches, in the order the title takes them.
var recipientArgs = []string{"to", "chat", "attendees", "recipient", "channel", "user", "email", "phone"}

// textArgs are what a send says: its preview goes into the notification.
var textArgs = []string{"text", "body", "message", "content", "description", "note"}

func argOf(p connectors.Proposal, names []string) string {
	for _, n := range names {
		for _, a := range p.Args {
			if a.Name == n && strings.TrimSpace(a.Value) != "" {
				return a.Value
			}
		}
	}
	return ""
}

// approvalMessage is the push of a proposal: «Send a message · Telegram: Misha?», then the subject, a preview of the
// text and why the agent proposes it.
func approvalMessage(p connectors.Proposal, tagPrefix, convTitle string, at time.Time) push.Message {
	title := pick(p.Title)
	if p.Name != "" {
		title += " · " + p.Name
	}
	if to := argOf(p, recipientArgs); to != "" {
		title += ": " + push.Clip(to, 40)
	} else if len(p.Recipients) > 0 {
		title += ": " + push.Clip(p.Recipients[0], 40)
	}
	title = push.Clip(title, 78) + "?"
	var lines []string
	if subj := argOf(p, []string{"subject", "title"}); subj != "" {
		lines = append(lines, locale.Pick("Subject: ", "Тема: ")+push.Clip(subj, 80))
	}
	if text := argOf(p, textArgs); text != "" {
		lines = append(lines, "«"+push.Clip(text, approvalTextMax)+"»")
	} else if p.Connector == connectors.OwnService {
		for _, a := range p.Args {
			lines = append(lines, a.Name+": "+push.Clip(a.Value, 60))
			if len(lines) >= 3 {
				break
			}
		}
	}
	if p.Why != "" {
		lines = append(lines, locale.Pick("Why: ", "Зачем: ")+push.Clip(p.Why, 100))
	}
	body := strings.Join(lines, "\n")
	if utf8.RuneCountInString(body) > 400 {
		body = string([]rune(body)[:399]) + "…"
	}
	if convTitle == "" {
		convTitle = locale.Pick("Agent", "Агент")
	}
	msg := push.Message{Kind: "approval", Title: title, Body: body, ConversationTitle: push.Clip(convTitle, 80),
		Detail: body, Provider: "claude", TabID: p.Conv, Tag: tagPrefix + "approval-" + p.ID, At: at.UnixMilli(),
		Proposal: p.ID}
	return msg
}

// outcome is the words of a settled proposal: the notification that replaces the one with the buttons.
func outcome(p connectors.Proposal) (string, string) {
	what := pick(p.Title)
	if p.Name != "" {
		what += " · " + p.Name
	}
	switch p.State {
	case connectors.Done:
		return locale.Pick("Sent", "Отправлено"), what
	case connectors.Declined:
		return locale.Pick("Not sent", "Не отправлено"), what
	case connectors.Expired:
		return locale.Pick("Not sent: no answer in time", "Не отправлено: время вышло"), what
	case connectors.Failed:
		return locale.Pick("Did not go", "Не ушло"), push.Clip(p.Result, 160)
	}
	return "", ""
}

// propose is connectors.Service.OnPropose: the push of a new proposal, then the watch for its outcome.
func (a *approvals) propose(p connectors.Proposal) {
	if a == nil || a.sender == nil {
		return
	}
	var subs []push.Subscription
	switch p.Notify {
	case "none":
		return
	case "":
		subs = a.sender.Store.ApprovalTargets(p.Conv)
	default:
		var err error
		if subs, err = a.sender.Store.Pick(p.Notify); err != nil {
			subs = a.sender.Store.ApprovalTargets(p.Conv)
		}
	}
	if len(subs) == 0 {
		return
	}
	convTitle := ""
	if a.titleOf != nil && p.Conv != "" {
		convTitle = a.titleOf(p.Conv)
	}
	msg := approvalMessage(p, a.sender.TagPrefix(), convTitle, a.now())
	// A service of the person's own is confirmed on the card only: the notification just leads there.
	if p.Connector != connectors.OwnService {
		token := newPushToken()
		msg.Token = token
		msg.Actions = []push.Action{{Action: "approve", Title: locale.Pick("Send", "Отправить")},
			{Action: "decline", Title: locale.Pick("Don't send", "Не отправлять")}}
		msg.Results = map[string]string{
			"approve": locale.Pick("Sent", "Отправлено"), "decline": locale.Pick("Not sent", "Не отправлено"),
			"failed": locale.Pick("Did not go — open the chat", "Не получилось — откройте чат"),
		}
		a.mu.Lock()
		a.sweepLocked()
		a.sent[p.ID] = &approvalSent{token: token, subs: subs, until: p.CreatedAt.Add(approvalWait)}
		a.mu.Unlock()
	} else {
		a.mu.Lock()
		a.sent[p.ID] = &approvalSent{subs: subs, until: p.CreatedAt.Add(approvalWait), used: true}
		a.mu.Unlock()
	}
	n := a.sender.SendTo(subs, msg)
	agent.Log("info", "push: a proposal sent to the person's devices", agent.F{K: "proposal", V: p.ID}, agent.F{K: "devices", V: n})
	a.settleWhenAnswered(p.ID)
}

// settleWhenAnswered waits for the proposal's outcome and replaces its notification with a quiet one that says it.
func (a *approvals) settleWhenAnswered(id string) {
	p, err := a.svc.Wait(context.Background(), id, approvalWait)
	if err != nil {
		a.forget(id)
		return
	}
	if p.State == connectors.Waiting {
		// Never answered in its time: the service closes it as expired on its next look.
		_ = a.svc.Proposals()
		if got, err := a.svc.Wait(context.Background(), id, 0); err == nil {
			p = got
		}
	}
	a.mu.Lock()
	sent := a.sent[id]
	delete(a.sent, id)
	a.mu.Unlock()
	if sent == nil {
		return
	}
	title, body := outcome(p)
	if title == "" {
		return
	}
	a.sender.SendTo(sent.subs, push.Message{Kind: "settled", Title: title, Body: body, ConversationTitle: title,
		Detail: body, Provider: "claude", TabID: p.Conv, Tag: a.sender.TagPrefix() + "approval-" + id, At: a.now().UnixMilli()})
}

func (a *approvals) forget(id string) {
	a.mu.Lock()
	delete(a.sent, id)
	a.mu.Unlock()
}

func (a *approvals) sweepLocked() {
	now := a.now()
	for id, s := range a.sent {
		if now.After(s.until) {
			delete(a.sent, id)
		}
	}
}

// errBadToken: the token is not this proposal's, was used, or its proposal is gone.
var errBadToken = errors.New("bad-token")

// take spends the proposal's token: true once, for the right token while the proposal is kept.
func (a *approvals) take(id, token string) bool {
	if a == nil || id == "" || token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sent[id]
	if s == nil || s.used || s.token == "" || a.now().After(s.until) {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(s.token), []byte(token)) != 1 {
		return false
	}
	s.used = true
	return true
}

// act answers a proposal from a notification's button.
func (a *approvals) act(ctx context.Context, id, verb, token string) (connectors.Proposal, error) {
	if verb != "approve" && verb != "decline" {
		return connectors.Proposal{}, errBadToken
	}
	if !a.take(id, token) {
		return connectors.Proposal{}, errBadToken
	}
	return a.svc.AnswerVia(ctx, id, verb == "approve", "push")
}

// agentPushLimit is how many pushes the agents of this runtime may send in agentPushWindow.
const (
	agentPushLimit  = 6
	agentPushWindow = 10 * time.Minute
)

type pushLimiter struct {
	mu   sync.Mutex
	sent []time.Time
}

func (l *pushLimiter) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.sent[:0]
	for _, t := range l.sent {
		if now.Sub(t) < agentPushWindow {
			kept = append(kept, t)
		}
	}
	l.sent = kept
	if len(l.sent) >= agentPushLimit {
		return false
	}
	l.sent = append(l.sent, now)
	return true
}

// pushRoutes are the button route of the notifications and the agent's devices and pushes.
func pushRoutes(d deps, a *approvals, convOf func(*http.Request) string) []gate.Route {
	limiter := &pushLimiter{}
	jsonOut := func(w http.ResponseWriter, status int, v any) { secretJSON(w, status, v) }
	readBody := func(r *http.Request, out any) bool {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
		return err == nil && json.Unmarshal(raw, out) == nil
	}
	act := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonOut(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
			return
		}
		var b struct {
			Proposal string `json:"proposal"`
			Verb     string `json:"verb"`
			Token    string `json:"token"`
		}
		if !readBody(r, &b) {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "bad-request"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), connectorCallMax)
		defer cancel()
		p, err := a.act(ctx, b.Proposal, b.Verb, b.Token)
		if errors.Is(err, errBadToken) {
			jsonOut(w, http.StatusForbidden, map[string]string{"error": "bad-token"})
			return
		}
		if err != nil {
			connectorRefusal(w, err)
			return
		}
		agent.Log("info", "push: the person answered an action from a notification", agent.F{K: "proposal", V: p.ID}, agent.F{K: "state", V: p.State})
		jsonOut(w, http.StatusOK, map[string]string{"state": p.State, "result": push.Clip(p.Result, 160)})
	}
	cli := func(w http.ResponseWriter, r *http.Request) {
		if d.CLI == nil || !d.CLI.OK(r) {
			jsonOut(w, http.StatusForbidden, map[string]string{"error": "Forbidden"})
			return
		}
		if a == nil || a.sender == nil {
			jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "push-off",
				"message": "web push is off on this machine (RUNTIME_PUSH_SUBJECT / RUNTIME_PUSH_TAG_PREFIX unset)"})
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/devices") && r.Method == http.MethodGet:
			jsonOut(w, http.StatusOK, map[string]any{"devices": a.sender.Store.Devices()})
		case strings.HasSuffix(r.URL.Path, "/send") && r.Method == http.MethodPost:
			var b struct {
				Device string `json:"device"`
				Title  string `json:"title"`
				Body   string `json:"body"`
				URL    string `json:"url"`
			}
			if !readBody(r, &b) || strings.TrimSpace(b.Title) == "" {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "bad-args", "message": "title is required"})
				return
			}
			subs, err := a.sender.Store.Pick(b.Device)
			if err != nil {
				jsonOut(w, http.StatusNotFound, map[string]string{"error": "no-device", "message": "no such device (harnessly devices lists them)"})
				return
			}
			if !limiter.allow(time.Now()) {
				jsonOut(w, http.StatusTooManyRequests, map[string]string{"error": "rate-limited", "message": "at most 6 pushes in 10 minutes"})
				return
			}
			url := strings.TrimSpace(b.URL)
			if !strings.HasPrefix(url, "/") || strings.HasPrefix(url, "//") || strings.Contains(url, "..") {
				url = ""
			}
			conv := convOf(r)
			msg := push.Message{Kind: "agent", Title: push.Clip(b.Title, 80), Body: push.Clip(b.Body, 240),
				ConversationTitle: push.Clip(b.Title, 80), Detail: push.Clip(b.Body, 240), Provider: "claude", TabID: conv,
				Tag: a.sender.TagPrefix() + "agent-" + newPushToken()[:8], At: time.Now().UnixMilli(), URL: url}
			n := a.sender.SendTo(subs, msg)
			agent.Log("info", "push: an agent sent a push", agent.F{K: "conv", V: conv}, agent.F{K: "devices", V: n})
			jsonOut(w, http.StatusOK, map[string]int{"sent": n})
		case strings.HasSuffix(r.URL.Path, "/ask") && r.Method == http.MethodPost:
			var b struct {
				Why  string  `json:"why"`
				Wait float64 `json:"wait"`
			}
			_ = readBody(r, &b)
			before := len(a.sender.Store.Subscriptions())
			if before > 0 {
				jsonOut(w, http.StatusOK, map[string]any{"state": "subscribed", "devices": a.sender.Store.Devices()})
				return
			}
			conv := convOf(r)
			if conv == "" {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "no-chat", "message": "run it from a chat's agent: the card shows in that chat"})
				return
			}
			a.ask(conv, b.Why, true)
			wait := max(min(time.Duration(b.Wait*float64(time.Second)), 45*time.Second), 0)
			deadline := time.Now().Add(wait)
			for time.Now().Before(deadline) && len(a.sender.Store.Subscriptions()) == 0 {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Second):
				}
			}
			if len(a.sender.Store.Subscriptions()) > 0 {
				a.ask(conv, "", false)
				jsonOut(w, http.StatusOK, map[string]any{"state": "subscribed", "devices": a.sender.Store.Devices()})
				return
			}
			jsonOut(w, http.StatusOK, map[string]any{"state": "waiting", "devices": []push.Device{}})
		default:
			jsonOut(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		}
	}
	return []gate.Route{
		{Match: func(r *http.Request) bool { return pathIs(r, "/api/push/act") }, Handler: http.HandlerFunc(act)},
		{Match: func(r *http.Request) bool {
			return pathIs(r, "/api/cli/push/devices") || pathIs(r, "/api/cli/push/send") || pathIs(r, "/api/cli/push/ask")
		}, Handler: http.HandlerFunc(cli)},
	}
}
