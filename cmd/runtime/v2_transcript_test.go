package main

import (
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/wire"
)

// A chat that reads the transcript of the session a hook names is left alone; a new session (after /clear, a resume
// that forked) or a chat without a transcript yet is attached.
func TestFollowedTranscript(t *testing.T) {
	const a, b = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	claude := wire.Conversation{AgentSessionID: a, TranscriptPath: "/h/.claude/projects/p/" + a + ".jsonl"}
	codex := wire.Conversation{AgentSessionID: a, TranscriptPath: "/h/.codex/sessions/2026/11/10/rollout-2026-11-10T10-00-00-" + a + ".jsonl"}
	for _, tc := range []struct {
		name           string
		rec            wire.Conversation
		session, named string
		want           bool
	}{
		{"the same Claude session", claude, a, "", true},
		{"the same Codex session, by its id", codex, a, "", true},
		{"the same Codex session, by its file", codex, a, codex.TranscriptPath, true},
		{"a new session", claude, b, "", false},
		{"the session known, its transcript not yet", wire.Conversation{AgentSessionID: a}, a, "", false},
		{"the id attached before the transcript of the new session", wire.Conversation{AgentSessionID: b, TranscriptPath: claude.TranscriptPath}, b, "", false},
		{"another file for the same session", codex, a, "/elsewhere/" + a + ".jsonl", false},
	} {
		if got := followed(tc.rec, tc.session, tc.named); got != tc.want {
			t.Errorf("%s: followed %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Claude writes a new session's transcript after its prompt-submit hook: the look goes on until the file is there,
// and gives up after its pauses.
func TestLookForATranscriptNotWrittenYet(t *testing.T) {
	pauses := []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}
	var slept []time.Duration
	sleep := func(d time.Duration) { slept = append(slept, d) }
	looks := 0
	appearsAt := func(n int) func() (string, bool) {
		return func() (string, bool) {
			looks++
			return "/t.jsonl", looks >= n
		}
	}
	if path, ok := lookFor(appearsAt(3), pauses, sleep); !ok || path != "/t.jsonl" || looks != 3 || len(slept) != 2 {
		t.Fatalf("found %q %v after %d looks, slept %v", path, ok, looks, slept)
	}
	looks, slept = 0, nil
	if _, ok := lookFor(appearsAt(10), pauses, sleep); ok || looks != 4 || len(slept) != 3 {
		t.Fatalf("never written: %v after %d looks, slept %v", ok, looks, slept)
	}
}
