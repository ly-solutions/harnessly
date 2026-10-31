package wire

import (
	"context"
	"errors"
	"net/http"

	"github.com/ly-solutions/harnessly/internal/jsonlog"
)

// LegacyTab is the answer of GET /api/v2/conversations/{id}/legacy: the Legacy tab a conversation runs in, which
// the right pane's tools of the current interface read (its timeline, diff, merge requests, subagents, spend).
type LegacyTab struct {
	TabID          string  `json:"tabId"`
	WorkspaceID    string  `json:"workspaceId"`
	SessionName    string  `json:"sessionName"`
	JsonlPath      *string `json:"jsonlPath"`
	AgentSessionID *string `json:"agentSessionId"`
	Cwd            string  `json:"cwd"`
}

func (l LegacyTab) Validate() error {
	return errors.Join(need("tabId", l.TabID), need("sessionName", l.SessionName), needPtr("jsonlPath", l.JsonlPath),
		needPtr("agentSessionId", l.AgentSessionID))
}

// ErrNoTab is what Server.Legacy answers for a conversation that has no tmux session yet: it is there, its tab is not.
var ErrNoTab = errors.New("no legacy tab yet")

// legacy serves GET /api/v2/conversations/{id}/legacy; a conversation without a tmux session yet
// answers 200 {"tab":null}, an unknown one is not found.
func (s *Server) legacy(ctx context.Context, w http.ResponseWriter, r *http.Request, conv string) {
	if s.Legacy == nil {
		refuse(w, RefuseNotFound)
		return
	}
	tab, err := s.Legacy(ctx, conv)
	switch {
	case errors.Is(err, ErrNoTab):
		writeJSON(w, http.StatusOK, map[string]any{"tab": nil})
	case err != nil && s.NotFound != nil && s.NotFound(err):
		refuse(w, RefuseNotFound)
	case err != nil:
		jsonlog.Log("warn", "v2: legacy tab failed", jsonlog.F{K: "path", V: r.URL.Path}, jsonlog.F{K: "error", V: err.Error()})
		refuse(w, RefuseInternal)
	default:
		s.writeChecked(w, r, tab)
	}
}
