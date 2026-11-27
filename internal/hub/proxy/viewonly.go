package proxy

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/ly-solutions/harnessly/internal/hub/machaccess"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/viewonly"
)

// A person the machine's owner opened it to only to watch (machaccess.LevelView, docs/hub/own-work.md, «Levels») reads
// what the watching page shows and writes nothing. The line is drawn here, for machines of any version: only the reads
// and sockets of internal/viewonly pass, a socket through a relay that drops every message of the page but the ones that
// only ask to be shown something. The session the hub asks the runtime for says it is a watcher's, so a runtime that
// knows of watchers draws the same line again.

// viewReadLimit is how big one message of the runtime may be on the relay: a first status sync is ~590 KB raw.
const (
	viewReadLimit = 32 << 20
	viewPageLimit = 64 << 10
)

// runtimePath is the request's path as the runtime answers it: without /m/<id> and the runtime's own /agents.
func runtimePath(path, id string) string {
	rest := strings.TrimPrefix(path, "/m/"+id)
	if r, ok := strings.CutPrefix(rest, "/agents"); ok && (r == "" || strings.HasPrefix(r, "/")) {
		rest = r
	}
	return rest
}

// readMethod says the method only reads.
func readMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// serveViewer answers a watcher's request that is not a read of the watching page: a socket it may open is relayed,
// anything else is refused. False leaves an allowed read to the usual way.
func (s *Service) serveViewer(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine) bool {
	upgrade := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
	path := runtimePath(r.URL.Path, m.ID)
	if !upgrade && readMethod(r.Method) && viewonly.Readable(path, r.URL.EscapedPath()) {
		return false
	}
	if upgrade && r.Method == http.MethodGet && viewonly.Socket(path) {
		s.relayView(w, r.WithContext(viewonly.With(r.Context())), u, m, path)
		return true
	}
	s.fail(w, r, machaccess.ErrViewOnly)
	return true
}

// relayView carries one of a watcher's sockets between the page and the runtime: everything the runtime says, and of the
// page only the messages viewonly.Message passes, as it rewrote them.
func (s *Service) relayView(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine, path string) {
	if s.reg.Await(r.Context(), m.ID) == nil {
		s.offline(w, r)
		return
	}
	leave, ok := s.enter(u.ID, m.ID)
	if !ok {
		s.upstreamFailed(w, r, errCrowded)
		return
	}
	defer leave()
	g, err := s.grant(r.Context(), u, m)
	if err != nil {
		s.upstreamFailed(w, r, err)
		return
	}
	target := *r.URL
	target.Scheme, target.Host = "http", "runtime"
	if q := target.Query(); q.Has("hl_ticket") {
		q.Del("hl_ticket")
		target.RawQuery = q.Encode()
	}
	h := http.Header{}
	h.Set("Cookie", runtimeCookie(g))
	if o := r.Header.Get("Origin"); o != "" {
		h.Set("Origin", o)
	}
	if ua := r.Header.Get("User-Agent"); ua != "" {
		h.Set("User-Agent", ua)
	}
	h.Set("X-Forwarded-Host", r.Host)
	h.Set("X-Forwarded-Proto", map[bool]string{true: "https", false: "http"}[s.cfg.Secure])
	h.Set("X-Forwarded-For", s.auth.ClientIP(r))
	var protocols []string
	for _, v := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				protocols = append(protocols, p)
			}
		}
	}
	dctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	up, res, err := websocket.Dial(dctx, target.String(), &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: s.transport(m.ID)}, HTTPHeader: h, Host: r.Host, Subprotocols: protocols,
	})
	cancel()
	if err != nil {
		if res != nil && res.StatusCode == http.StatusUnauthorized {
			s.dropGrant(u.ID, m.ID)
		}
		s.upstreamFailed(w, r, err)
		return
	}
	defer up.CloseNow()
	up.SetReadLimit(viewReadLimit)
	page, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: protocols})
	if err != nil {
		up.Close(websocket.StatusGoingAway, "")
		return
	}
	defer page.CloseNow()
	page.SetReadLimit(viewPageLimit)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	defer s.track(u.ID+"/"+m.SpaceID, stop)()
	done := make(chan struct{}, 2)
	go func() { // the runtime → the page, as it is
		defer func() { done <- struct{}{} }()
		for {
			typ, data, err := up.Read(ctx)
			if err != nil {
				closeLike(page, err)
				return
			}
			if page.Write(ctx, typ, data) != nil {
				return
			}
		}
	}()
	go func() { // the page → the runtime, only what allow lets
		defer func() { done <- struct{}{} }()
		for {
			typ, data, err := page.Read(ctx)
			if err != nil {
				closeLike(up, err)
				return
			}
			if typ != websocket.MessageText {
				continue
			}
			data, ok := viewonly.Message(path, data)
			if !ok {
				continue
			}
			if up.Write(ctx, typ, data) != nil {
				return
			}
		}
	}()
	<-done
}

// closeLike ends the other side of the relay as this one ended.
func closeLike(c *websocket.Conn, err error) {
	var ce websocket.CloseError
	if errors.As(err, &ce) {
		c.Close(ce.Code, ce.Reason)
		return
	}
	c.Close(websocket.StatusGoingAway, "")
}
