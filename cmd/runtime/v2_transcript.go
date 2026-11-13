package main

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/safego"
	"github.com/ly-solutions/harnessly/internal/wire"
)

// A chat finds its agent's transcript itself: every hook of its agent names the agent's session, and the session's
// file is found by that id (a Codex hook names the file too). The tab keeper's view (syncOnce) is the way only while
// the chats' sessions are left to the tabs' keepers (LegacyLife).

// findRetries are the pauses between the looks for a session's transcript that is not written yet (about 20 s in all).
var findRetries = []time.Duration{300 * time.Millisecond, 700 * time.Millisecond, time.Second, 2 * time.Second, 3 * time.Second,
	4 * time.Second, 4 * time.Second, 5 * time.Second}

// followSession is follow for a hook that names its tmux session only.
func (v *v2Live) followSession(session, agentSession, transcript string) {
	if !v.ready.Load() {
		return
	}
	if id := v.rt.ConvOfSession(session); id != "" {
		v.follow(id, session, agentSession, transcript)
	}
}

// follow attaches the conversation to the agent session a hook of its own agent named, when that is not the one it
// reads already: with the transcript the hook gave, or the one found by the session's id. The search reads a
// directory, so it runs beside the hook. Claude writes a new session's file after its prompt-submit hook has fired, so
// a file not there yet is looked for again for a while (findRetries) instead of waiting for the next hook: the chat
// shows the message and confirms it by its entry as soon as the file appears.
func (v *v2Live) follow(conv, session, agentSession, transcript string) {
	if agentSession == "" || !v.rt.OwnsSession(session) {
		return
	}
	rec, ok := v.rt.Record(conv)
	if !ok || followed(rec, agentSession, transcript) {
		return
	}
	if transcript != "" {
		_ = v.rt.Attach(conv, session, transcript, agentSession)
		return
	}
	if v.ses == nil {
		return
	}
	if _, busy := v.finding.LoadOrStore(conv, struct{}{}); busy {
		return
	}
	safego.Go("chat transcript", func() {
		defer v.finding.Delete(conv)
		find := func() (string, bool) {
			found, ok := v.ses.Find(rec.Provider, agentSession)
			return found.TranscriptPath, ok && found.TranscriptPath != ""
		}
		if path, ok := lookFor(find, findRetries, time.Sleep); ok {
			_ = v.rt.Attach(conv, session, path, agentSession)
		}
	})
}

// lookFor asks find until it finds, sleeping the pauses between the looks; false when the last look found nothing.
func lookFor(find func() (string, bool), pauses []time.Duration, sleep func(time.Duration)) (string, bool) {
	for i := 0; ; i++ {
		if path, ok := find(); ok {
			return path, true
		}
		if i >= len(pauses) {
			return "", false
		}
		sleep(pauses[i])
	}
}

// followed says the conversation already reads the transcript of that agent session: the file the hook named, or,
// when it named none, a file called by the session's id (both agents name their transcripts so).
func followed(rec wire.Conversation, agentSession, transcript string) bool {
	if rec.AgentSessionID != agentSession || rec.TranscriptPath == "" {
		return false
	}
	if transcript != "" {
		return rec.TranscriptPath == transcript
	}
	return strings.Contains(filepath.Base(rec.TranscriptPath), agentSession)
}
