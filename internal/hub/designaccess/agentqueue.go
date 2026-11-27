package designaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/safego"
)

// HRN-1799 (stage 7). A comment on a site that calls the agent (@harnessly) is handed to a chat on the
// machine (POST <platform>/design/site/comments/agent). With the machine off the hub keeps it in design_agent_comments,
// answers 202 {queued: true} — the page says it waits for the machine — and hands it on as the person once the machine's
// link is back. GET on the same route lists the comments of a project still waiting.

var agentCommentPath = regexp.MustCompile(`^/api/[A-Za-z0-9_-]+/design/site/comments/agent$`)

var (
	commentIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
	agentChatRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
)

const (
	// agentQueueMax caps the comments one machine has waiting.
	agentQueueMax = 200
	// agentQueueKeep is how long a comment waits for its machine before it is dropped.
	agentQueueKeep = 7 * 24 * time.Hour
	// agentQueueClaim is how long a replica holds a comment it is handing on before another may try it.
	agentQueueClaim = time.Minute
	// agentQueueRounds and agentQueueGap: a machine that has just come up gets its comments in rounds, since the project's
	// newest revision (with the comment in it) may reach its folder a moment after the link.
	agentQueueRounds = 20
	agentQueueGap    = 15 * time.Second
	agentCallTimeout = 30 * time.Second
)

type agentQueueState struct {
	mu      sync.Mutex
	running map[string]bool
}

// UseForward gives the queue the proxy's way to a machine as a person. It is set once, before the hub serves.
func (s *Service) UseForward(f func(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine)) {
	s.forward = f
}

type agentComment struct {
	Artifact string `json:"artifact"`
	ID       string `json:"id"`
	Chat     string `json:"chat"`
}

func (s *Service) serveAgentQueue(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine) bool {
	rest := strings.TrimPrefix(r.URL.EscapedPath(), "/m/"+m.ID)
	if !agentCommentPath.MatchString(rest) || (r.Method != http.MethodPost && r.Method != http.MethodGet) {
		return false
	}
	ctx := r.Context()
	if s.forward == nil || !s.machineOff(ctx, m) {
		return false
	}
	if r.Method == http.MethodPost && s.machineUp(m.ID) {
		return false
	}
	var c agentComment
	if r.Method == http.MethodPost {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
		if err != nil || len(raw) > 64<<10 || json.Unmarshal(raw, &c) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Нужны artifact, id и chat", "code": "bad_request"})
			return true
		}
	} else {
		c.Artifact = r.URL.Query().Get("artifact")
	}
	if !projectIDRe.MatchString(c.Artifact) || (r.Method == http.MethodPost && (!commentIDRe.MatchString(c.ID) || !agentChatRe.MatchString(c.Chat))) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Нужны artifact, id и chat", "code": "bad_request"})
		return true
	}
	need := roleComment
	if r.Method == http.MethodGet {
		need = roleView
	}
	t, err := s.read(ctx, m)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	p, stored, err := t.effective(c.Artifact)
	if err != nil || !stored {
		// A project the hub has no row of is still the machine's: the machine answers, or the proxy says it is off.
		return false
	}
	f, err := t.fold(p, u.Email)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	if f.Role == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Макета больше нет"})
		return true
	}
	if rank(f.Role) < rank(need) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Нет доступа к проекту", "code": "no_access", "role": f.Role})
		return true
	}
	if r.Method == http.MethodGet {
		ids, err := s.waitingComments(ctx, m.ID, c.Artifact)
		if err != nil {
			s.writeError(w, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"waiting": ids})
		return true
	}
	if err := s.enqueueAgent(ctx, m, u, c, rest); err != nil {
		s.writeError(w, err)
		return true
	}
	// The link may have come back while the comment was written: it goes on at once then.
	if s.machineUp(m.ID) {
		s.drainAgentQueue(m.ID)
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
	return true
}

func (s *Service) enqueueAgent(ctx context.Context, m store.Machine, u store.User, c agentComment, path string) error {
	var n int
	if err := s.db.S().Q().QueryRow(ctx, `SELECT COUNT(*) FROM design_agent_comments WHERE machine_id = ?`, m.ID).Scan(&n); err != nil {
		return err
	}
	if n >= agentQueueMax {
		return fail("rate_limited", "Агенту на этой машине уже ждут много комментариев: включите машину, и они дойдут")
	}
	_, err := s.db.Exec(ctx, `INSERT INTO design_agent_comments (id, machine_id, user_id, project_id, comment_id, chat, path, created_ms, tries)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0) ON CONFLICT DO NOTHING`, newUUID(), m.ID, u.ID, c.Artifact, c.ID, c.Chat, path, ms(s.now()))
	return err
}

func (s *Service) waitingComments(ctx context.Context, machineID, project string) ([]string, error) {
	rows, err := s.db.S().Q().Query(ctx, `SELECT comment_id FROM design_agent_comments WHERE machine_id = ? AND project_id = ? ORDER BY created_ms`, machineID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// drainAgentQueue hands on what waits for the machine, in rounds while it is up and something is left; one run per
// machine on this replica at a time.
func (s *Service) drainAgentQueue(machineID string) {
	if s.forward == nil {
		return
	}
	s.agentQ.mu.Lock()
	if s.agentQ.running == nil {
		s.agentQ.running = map[string]bool{}
	}
	if s.agentQ.running[machineID] {
		s.agentQ.mu.Unlock()
		return
	}
	s.agentQ.running[machineID] = true
	s.agentQ.mu.Unlock()
	safego.Go("design agent queue", func() {
		defer func() {
			s.agentQ.mu.Lock()
			delete(s.agentQ.running, machineID)
			s.agentQ.mu.Unlock()
		}()
		for round := 0; round < agentQueueRounds; round++ {
			if round > 0 {
				time.Sleep(agentQueueGap)
			}
			if !s.machineUp(machineID) {
				return
			}
			left, err := s.handOnAgent(context.Background(), machineID)
			if err != nil {
				s.log.Warn("design agent queue: not handed on", "machine", machineID, "error", err)
			}
			if left == 0 && err == nil {
				return
			}
		}
	})
}

type queuedComment struct {
	id, userID, project, comment, chat, path string
	created                                  int64
}

// handOnAgent hands each waiting comment of the machine to it once; left is how many still wait.
func (s *Service) handOnAgent(ctx context.Context, machineID string) (int, error) {
	now := ms(s.now())
	if _, err := s.db.Exec(ctx, `DELETE FROM design_agent_comments WHERE machine_id = ? AND created_ms < ?`, machineID, now-agentQueueKeep.Milliseconds()); err != nil {
		return 0, err
	}
	rows, err := s.db.S().Q().Query(ctx, `SELECT id, user_id, project_id, comment_id, chat, path, created_ms FROM design_agent_comments
		WHERE machine_id = ? AND (claimed_ms IS NULL OR claimed_ms < ?) ORDER BY created_ms`, machineID, now-agentQueueClaim.Milliseconds())
	if err != nil {
		return 0, err
	}
	var queued []queuedComment
	for rows.Next() {
		var q queuedComment
		if err := rows.Scan(&q.id, &q.userID, &q.project, &q.comment, &q.chat, &q.path, &q.created); err != nil {
			rows.Close()
			return 0, err
		}
		queued = append(queued, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(queued) == 0 {
		return 0, nil
	}
	m, err := s.db.S().MachineByID(ctx, machineID)
	if err != nil {
		return len(queued), err
	}
	left := 0
	for _, q := range queued {
		res, err := s.db.Exec(ctx, `UPDATE design_agent_comments SET claimed_ms = ? WHERE id = ? AND (claimed_ms IS NULL OR claimed_ms < ?)`, now, q.id, now-agentQueueClaim.Milliseconds())
		if err != nil {
			return left + 1, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // another replica has it
		}
		status := s.tellAgent(ctx, m, q)
		switch {
		case status >= 200 && status < 300:
			_, err = s.db.Exec(ctx, `DELETE FROM design_agent_comments WHERE id = ?`, q.id)
		case status == 0 || status == http.StatusNotFound || status >= 500:
			// Not there yet: the comment's revision has not reached the machine's folder, or the machine went away.
			left++
			_, err = s.db.Exec(ctx, `UPDATE design_agent_comments SET claimed_ms = NULL, tries = tries + 1 WHERE id = ?`, q.id)
		default:
			// The machine said no for good (the chat is gone, the person lost the right): nobody would ever take it.
			s.log.Warn("design agent queue: the machine refused a comment", "machine", machineID, "project", q.project, "status", status)
			_, err = s.db.Exec(ctx, `DELETE FROM design_agent_comments WHERE id = ?`, q.id)
		}
		if err != nil {
			return left, err
		}
	}
	return left, nil
}

// tellAgent sends one comment to the machine as the person who wrote it; the status it answered, 0 when none.
func (s *Service) tellAgent(ctx context.Context, m store.Machine, q queuedComment) int {
	u, err := s.db.S().UserByID(ctx, q.userID)
	if err != nil || u.DisabledAt != nil {
		return http.StatusForbidden
	}
	body, _ := json.Marshal(agentComment{Artifact: q.project, ID: q.comment, Chat: q.chat})
	cctx, cancel := context.WithTimeout(ctx, agentCallTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, q.path, bytes.NewReader(body))
	if err != nil {
		return http.StatusBadRequest
	}
	if s.hubURL != "" {
		if pu, err := url.Parse(s.hubURL); err == nil {
			req.Host = pu.Host
		}
	}
	req.RemoteAddr = "127.0.0.1:0"
	req.Header.Set("Content-Type", "application/json")
	rec := &statusRecorder{header: http.Header{}}
	s.forward(rec, req, u, m)
	if cctx.Err() != nil {
		return 0
	}
	return rec.status
}

// statusRecorder keeps the status of an answer and drops its body.
type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header { return r.header }
func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return len(b), nil
}
