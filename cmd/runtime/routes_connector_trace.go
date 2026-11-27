package main

// The developer mode's trace of the connectors (docs/connectors.md «Developer mode»): the last lines each
// connection wrote while the mode was on, and a stream of new ones. The trace is kept in a ring on this machine; it can
// hold a service's raw answers, so only the person's page session reads it (never an agent's CLI token) and nothing of
// it goes to the hub's journal.
//
//	GET <platform>/connectors/trace[?conn=<id>&limit=N]   {on, entries, conns}
//	GET <platform>/connectors/trace/stream[?conn=<id>]    text/event-stream: data {entry}, ": ping" every 15 s

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/connectors"
	"github.com/ly-solutions/harnessly/internal/gate"
	"github.com/ly-solutions/harnessly/internal/sameorigin"
	"github.com/ly-solutions/harnessly/internal/settings"
)

const (
	traceLimitDefault = 200
	traceLimitMax     = 1000
	// devModeTTL is how long the settings file's developerMode is trusted before it is read again.
	devModeTTL = 2 * time.Second
	// syncTraceEvery: the last sync of the agents' MCP servers is written to the trace again at most this often.
	syncTraceEvery = time.Minute
)

// devModeReader answers whether the developer mode is on, reading the settings file at most every devModeTTL.
func devModeReader(store *settings.Store) func() bool {
	var mu sync.Mutex
	var on bool
	var at time.Time
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(at) < devModeTTL {
			return on
		}
		v, _ := store.ConfigObject().Get("developerMode")
		on, at = v == true, time.Now()
		return on
	}
}

// traceMCPSync writes the last synchronization of the agents' MCP servers to the trace again, at most once
// a minute: the developer mode may have been turned on after it ran.
func traceMCPSync(d deps, last *time.Time, mu *sync.Mutex) {
	if !connectors.Trace.On() {
		return
	}
	mu.Lock()
	if time.Since(*last) < syncTraceEvery {
		mu.Unlock()
		return
	}
	*last = time.Now()
	mu.Unlock()
	if s := agentSyncerOf(d); s != nil {
		s.Replay()
	}
}

func connectorTraceRoutes(d deps) []gate.Route {
	connectors.Trace.SetEnabled(devModeReader(&settings.Store{DataDir: d.DataDir, ConfigFile: d.ConfigFile}))
	base := platformAPI + "/connectors/trace"
	stream := base + "/stream"
	var syncMu sync.Mutex
	var syncAt time.Time
	ok := func(w http.ResponseWriter, r *http.Request) bool {
		if sameorigin.Refuse(w, r) {
			return false
		}
		if !d.SessionOK(r) { // the person's page alone: an agent's CLI token never reads the trace
			secretJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			return false
		}
		if r.Method != http.MethodGet {
			secretJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
			return false
		}
		return true
	}
	list := func(w http.ResponseWriter, r *http.Request) {
		if !ok(w, r) {
			return
		}
		traceMCPSync(d, &syncAt, &syncMu)
		limit := traceLimitDefault
		if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
			limit = min(n, traceLimitMax)
		}
		entries := connectors.Trace.Recent(r.URL.Query().Get("conn"), limit)
		if entries == nil {
			entries = []connectors.TraceEntry{}
		}
		secretJSON(w, http.StatusOK, map[string]any{"on": connectors.Trace.On(), "entries": entries, "conns": connectors.Trace.Conns()})
	}
	follow := func(w http.ResponseWriter, r *http.Request) {
		if !ok(w, r) {
			return
		}
		flusher, can := w.(http.Flusher)
		if !can {
			secretJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed"})
			return
		}
		traceMCPSync(d, &syncAt, &syncMu)
		entries, stop := connectors.Trace.Subscribe(r.URL.Query().Get("conn"))
		defer stop()
		h := w.Header()
		h.Set("Content-Type", "text/event-stream; charset=utf-8")
		h.Set("Cache-Control", "no-store, no-transform")
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(": ok\n\n"))
		flusher.Flush()
		beat := time.NewTicker(15 * time.Second)
		defer beat.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-entries:
				raw, _ := json.Marshal(e)
				_, _ = w.Write(append(append([]byte("data: "), raw...), '\n', '\n'))
				flusher.Flush()
			case <-beat.C:
				traceMCPSync(d, &syncAt, &syncMu)
				_, _ = w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			}
		}
	}
	is := func(r *http.Request, p string) bool { return slices.Contains([]string{p, "/agents" + p}, r.URL.Path) }
	return []gate.Route{
		{Match: func(r *http.Request) bool { return is(r, stream) }, Handler: http.HandlerFunc(follow)},
		{Match: func(r *http.Request) bool { return is(r, base) }, Handler: http.HandlerFunc(list)},
	}
}
