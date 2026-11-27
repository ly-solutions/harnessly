package designaccess

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// A private snapshot is what a project's owner leaves for readers who already hold a grant while the owner machine is
// offline: bounded raster pages of the boards the owner chose, captured the way a guest snapshot is (static_guest.go).
// It carries no HTML and no link of its own. Publishing creates no grant and removing it takes none away: every read
// goes through the same project reader as a live one and is checked against the grant, the project and the snapshot's
// own term at that moment.

const (
	defaultSnapshotTerm = 7 * 24 * time.Hour
	maxSnapshotTerm     = 30 * 24 * time.Hour
)

func snapshotTerm(b map[string]any, now time.Time) (time.Time, error) {
	v, ok := b["ttl"]
	if !ok || v == nil {
		return now.Add(defaultSnapshotTerm), nil
	}
	term, err := ttlOf(v, ttlMessage)
	if err != nil {
		return time.Time{}, err
	}
	end, err := expiry(term, now)
	if err != nil || end == nil {
		return time.Time{}, fail("bad_request", ttlMessage)
	}
	if end.Sub(now) > maxSnapshotTerm {
		return time.Time{}, fail("bad_request", "Снимок живёт не дольше 30 дней: выберите срок короче")
	}
	return *end, nil
}

func (s *Service) snapshotMeta(ctx context.Context, m store.Machine, project string) (map[string]any, error) {
	var rev, by string
	var created, expires int64
	err := s.db.QueryRow(ctx, `SELECT rev, created_by, created_at, expires_at FROM design_private_snapshots WHERE machine_id=? AND project_id=? AND expires_at>?`,
		m.ID, project, ms(s.now())).Scan(&rev, &by, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT position, title FROM design_private_snapshot_pages WHERE machine_id=? AND project_id=? ORDER BY position`, m.ID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	boards := []map[string]any{}
	for rows.Next() {
		var n int
		var title string
		if err := rows.Scan(&n, &title); err != nil {
			return nil, err
		}
		if title == "" {
			title = strconv.Itoa(n + 1)
		}
		boards = append(boards, map[string]any{"n": n, "title": title})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"capturedAt": isoOf(time.UnixMilli(created)), "expiresAt": isoOf(time.UnixMilli(expires)), "boards": boards}, nil
}

// snapshotRoute is the owner's side: GET shows what is published, PUT publishes (replacing the earlier one), DELETE removes.
func (s *Service) snapshotRoute(r *http.Request, m store.Machine, email, project string) (any, int, error) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		meta, err := s.snapshotMeta(ctx, m, project)
		return map[string]any{"snapshot": meta}, 200, err
	case http.MethodDelete:
		if err := s.write(ctx, m, func(t *txn) error {
			_, err := t.q.Exec(t.ctx, `DELETE FROM design_private_snapshots WHERE machine_id=? AND project_id=?`, m.ID, project)
			if err == nil {
				err = t.writeLog(project, email, "snapshot.remove", "", nil)
			}
			return err
		}); err != nil {
			return nil, 0, err
		}
		return map[string]any{"snapshot": nil}, 200, nil
	case http.MethodPut:
	default:
		return nil, 0, fail("not_found", "Маршрут не найден")
	}
	ctx, cancel := context.WithTimeout(ctx, staticPublicationTimeout)
	defer cancel()
	b, err := readBody(r)
	if err != nil {
		return nil, 0, err
	}
	pages, err := checkPages(b["pages"])
	if err != nil {
		return nil, 0, err
	}
	end, err := snapshotTerm(b, s.now())
	if err != nil {
		return nil, 0, err
	}
	rev, shots, err := s.capture(ctx, m, project, shareInput{Pages: pages})
	if err != nil {
		return nil, 0, err
	}
	if err := s.write(ctx, m, func(t *txn) error {
		if _, err := t.lockProject(project); err != nil {
			return err
		}
		if _, err := t.q.Exec(t.ctx, `DELETE FROM design_private_snapshots WHERE machine_id=? AND project_id=?`, m.ID, project); err != nil {
			return err
		}
		if _, err := t.q.Exec(t.ctx, `INSERT INTO design_private_snapshots(machine_id,project_id,rev,created_by,created_at,expires_at) VALUES(?,?,?,?,?,?)`,
			m.ID, project, rev, normEmail(email), ms(t.now), ms(end)); err != nil {
			return err
		}
		for i, p := range shots {
			if _, err := t.q.Exec(t.ctx, `INSERT INTO design_private_snapshot_pages(machine_id,project_id,position,board,title,jpeg) VALUES(?,?,?,?,?,?)`,
				m.ID, project, i, p.Path, p.Title, p.Image); err != nil {
				return err
			}
		}
		return t.writeLog(project, email, "snapshot.publish", "", map[string]any{"boards": len(shots), "until": isoOf(end)})
	}); err != nil {
		return nil, 0, err
	}
	meta, err := s.snapshotMeta(ctx, m, project)
	return map[string]any{"snapshot": meta}, 200, err
}
