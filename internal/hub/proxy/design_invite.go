package proxy

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"

	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// The link is a navigation only. The signed-in design access API consumes it.
func (s *Service) designInvite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	token := r.PathValue("token")
	if !store.ValidDesignInviteToken(token) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	p, err := s.auth.Authenticate(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if p == nil {
		http.Redirect(w, r, "/signin?next="+url.QueryEscape(r.URL.Path), http.StatusFound)
		return
	}
	var machine, space string
	err = s.db.QueryRow(r.Context(), `SELECT m.id,m.space_id FROM design_invites i JOIN machines m ON m.id=i.machine_id WHERE i.token_hash=? AND m.revoked_at IS NULL`, store.HashToken(token)).Scan(&machine, &space)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.spaces.Access(r.Context(), p.User.ID, space); err != nil {
		// a person of another space accepts the invitation in a space of their own; the project is then read
		// from the hub's store of the owner's space (designaccess/foreign.go), never through a machine there.
		machine, space = s.ownPlace(r, p.User.ID)
		if space == "" {
			httpx.WriteError(w, r, httpx.ErrNotFound)
			return
		}
	}
	sp, err := s.db.S().SpaceByID(r.Context(), space)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	target := workspaceBase + "/designly/invite/" + token
	if !s.Workspace() || !s.fleetOn(r.Context(), sp) {
		if machine == "" {
			httpx.WriteError(w, r, httpx.ErrNotFound)
			return
		}
		if _, _, err := s.admitView(r.Context(), p.User.ID, machine); err != nil {
			s.fail(w, r, err)
			return
		}
		target = "/m/" + machine + "/designly/invite/" + token + "?runtime=1"
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// ownPlace is a space of the person's own and a machine there they may open ("" when they have none), for an invitation
// of another space: the first space with a machine they may open, else the first space.
func (s *Service) ownPlace(r *http.Request, userID string) (machine, space string) {
	ctx := r.Context()
	spaces, err := s.db.S().SpacesByUser(ctx, userID)
	if err != nil {
		return "", ""
	}
	for _, sp := range spaces {
		ms, err := s.db.S().MachinesBySpace(ctx, sp.ID)
		if err != nil {
			continue
		}
		for _, m := range ms {
			if m.RevokedAt != nil || m.DisabledAt != nil {
				continue
			}
			if _, _, err := s.admitView(ctx, userID, m.ID); err == nil {
				return m.ID, sp.ID
			}
		}
	}
	if len(spaces) > 0 {
		return "", spaces[0].ID
	}
	return "", ""
}
