package designaccess

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// The edit lease of a project (block «Аренда правки» of the target architecture): who edits it now, on which
// machine, until when. The page of an editor in the editing mode takes it and renews it while it is open; everyone else
// sees in the header who edits. It is advisory: two editors still cannot overwrite each other, every write carries its
// base revision and a stale one is 409.
//
//	GET    lease   the lease now, or {"lease": null} when nobody edits (any role)
//	POST   lease   take or renew it for the caller on this machine (edit); taken by another one, it stays theirs
//	DELETE lease   give it back, the caller's own only (edit)

// leaseTTL is how long a lease outlives its last renewal: the page renews it every 30 s.
const leaseTTL = 90 * time.Second

type editLease struct {
	Person      string `json:"person"`
	Name        string `json:"name"`
	Machine     string `json:"machine"`
	MachineName string `json:"machineName"`
	Until       int64  `json:"until"`
	// Mine: the caller holds it on this machine.
	Mine bool `json:"mine"`
}

func (s *Service) serveLease(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine, project string, f folded) error {
	ctx := r.Context()
	q := s.db.S().Q()
	now := s.now()
	who := normEmail(u.Email)
	current := func() (*editLease, error) {
		var l editLease
		err := q.QueryRow(ctx, `SELECT person, name, machine_id, machine_name, until_ms FROM design_edit_leases
			WHERE space_id = ? AND project_id = ? AND until_ms > ?`, m.SpaceID, project, now.UnixMilli()).
			Scan(&l.Person, &l.Name, &l.Machine, &l.MachineName, &l.Until)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		l.Mine = l.Person == who && l.Machine == m.ID
		return &l, nil
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		l, err := current()
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]any{"lease": l})
		return nil
	}
	if rank(f.Role) < rank(roleEdit) {
		return fail("forbidden", "Править проект может редактор")
	}
	switch r.Method {
	case http.MethodPost:
		name := strings.TrimSpace(u.Name)
		if name == "" {
			name = who
		}
		until := now.Add(leaseTTL).UnixMilli()
		// Taken when nobody holds it, or it ran out, or it is the caller's own on this machine.
		if _, err := q.Exec(ctx, `INSERT INTO design_edit_leases (space_id, project_id, person, name, machine_id, machine_name, until_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (space_id, project_id) DO UPDATE SET person = excluded.person, name = excluded.name,
				machine_id = excluded.machine_id, machine_name = excluded.machine_name, until_ms = excluded.until_ms
			WHERE design_edit_leases.until_ms <= ? OR (design_edit_leases.person = excluded.person AND design_edit_leases.machine_id = excluded.machine_id)`,
			m.SpaceID, project, who, name, m.ID, strings.TrimSpace(m.Name), until, now.UnixMilli()); err != nil {
			return err
		}
	case http.MethodDelete:
		if _, err := q.Exec(ctx, `DELETE FROM design_edit_leases WHERE space_id = ? AND project_id = ? AND person = ? AND machine_id = ?`,
			m.SpaceID, project, who, m.ID); err != nil {
			return err
		}
	default:
		return fail("bad_request", "Метод не поддерживается")
	}
	l, err := current()
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"lease": l})
	return nil
}
