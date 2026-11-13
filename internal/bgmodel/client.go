// Package bgmodel is the model behind the background features: the small jobs harnessly does beside a chat (the
// summary of a notification, «what is going on now», the next-step hints, the background memory). A person picks a
// provider and a model for each (Settings → Background tasks), or leaves it «as now», where the chat's own agent
// decides and nothing here is called.
//
// The client is small on purpose: one chat completion, in the OpenAI-compatible or the Anthropic-compatible form, with
// a timeout and a cap on the answer's tokens. A feature that cannot get its answer falls back to what it did before.
package bgmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/extproviders"
)

// Bounds of one call.
const (
	DefaultTimeout   = 12 * time.Second
	MaxTimeout       = 60 * time.Second
	DefaultMaxTokens = 256
	MaxMaxTokens     = 4096
	// MaxPrompt is the most text a call sends (bytes); a longer prompt keeps its end, where a turn's outcome is.
	MaxPrompt = 24 << 10
)

// Reasons a call failed.
const (
	ReasonUnauthorized = "unauthorized" // the provider refused the key
	ReasonUnreachable  = "unreachable"  // the address did not answer
	ReasonTimeout      = "timeout"      // no answer in time
	ReasonRateLimited  = "rate-limited" // 429
	ReasonRejected     = "rejected"     // another error of the provider
	ReasonEmpty        = "empty"        // an answer with no text
	ReasonNoProvider   = "no-provider"  // the provider picked for the feature is gone
)

// Error is a failed call: why, and the provider's status when it answered. It holds neither the key nor the body.
type Error struct {
	Reason string
	Status int
}

func (e *Error) Error() string { return "bgmodel: " + e.Reason }

// ReasonOf is the reason of any error of a call.
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ReasonRejected
}

// Request is one completion.
type Request struct {
	BaseURL string
	API     extproviders.API
	Key     string
	Model   string
	// System is the instruction, Prompt the text it is about.
	System string
	Prompt string
	// MaxTokens caps the answer (DefaultMaxTokens when 0), Timeout the whole call (DefaultTimeout when 0).
	MaxTokens int
	Timeout   time.Duration
}

// Client makes the calls.
type Client struct {
	// HTTP sends them; nil is a client that follows no redirect (a redirect would carry the key to the host it points
	// at).
	HTTP *http.Client
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[len(s)-max:]
	// Not from the middle of a character.
	for len(s) > 0 && s[0]&0xC0 == 0x80 {
		s = s[1:]
	}
	return s
}

// Complete asks the model once and gives the text of its answer.
func (c *Client) Complete(ctx context.Context, r Request) (string, error) {
	maxTokens := r.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	maxTokens = min(maxTokens, MaxMaxTokens)
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, min(timeout, MaxTimeout))
	defer cancel()
	prompt := tail(r.Prompt, MaxPrompt)

	var body map[string]any
	path := "chat/completions"
	if r.API == extproviders.APIAnthropic {
		path = "messages"
		body = map[string]any{"model": r.Model, "max_tokens": maxTokens, "messages": []map[string]string{{"role": "user", "content": prompt}}}
		if r.System != "" {
			body["system"] = r.System
		}
	} else {
		messages := []map[string]string{}
		if r.System != "" {
			messages = append(messages, map[string]string{"role": "system", "content": r.System})
		}
		messages = append(messages, map[string]string{"role": "user", "content": prompt})
		body = map[string]any{"model": r.Model, "max_tokens": maxTokens, "messages": messages}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, extproviders.Endpoint(r.BaseURL, r.API, path), bytes.NewReader(raw))
	if err != nil {
		return "", &Error{Reason: ReasonUnreachable}
	}
	req.Header = extproviders.Headers(r.API, r.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http().Do(req)
	if err != nil {
		if extproviders.IsTimeout(err) || ctx.Err() != nil {
			return "", &Error{Reason: ReasonTimeout}
		}
		return "", &Error{Reason: ReasonUnreachable}
	}
	defer res.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", &Error{Reason: ReasonTimeout, Status: res.StatusCode}
	}
	switch status := res.StatusCode; {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "", &Error{Reason: ReasonUnauthorized, Status: status}
	case status == http.StatusTooManyRequests:
		return "", &Error{Reason: ReasonRateLimited, Status: status}
	case status < 200 || status > 299:
		return "", &Error{Reason: ReasonRejected, Status: status}
	}
	text := strings.TrimSpace(textOf(r.API, answer))
	if text == "" {
		return "", &Error{Reason: ReasonEmpty, Status: res.StatusCode}
	}
	return text, nil
}

// textOf is the text of an answer: choices[0].message.content of the OpenAI form (a string, or parts with text), the
// text blocks of the Anthropic one.
func textOf(api extproviders.API, raw []byte) string {
	type part struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	join := func(parts []part) string {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "" || p.Type == "text" || p.Type == "output_text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	if api == extproviders.APIAnthropic {
		var doc struct {
			Content []part `json:"content"`
		}
		if json.Unmarshal(raw, &doc) != nil {
			return ""
		}
		return join(doc.Content)
	}
	var doc struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &doc) != nil || len(doc.Choices) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(doc.Choices[0].Message.Content, &s) == nil {
		return s
	}
	var parts []part
	if json.Unmarshal(doc.Choices[0].Message.Content, &parts) == nil {
		return join(parts)
	}
	return ""
}
