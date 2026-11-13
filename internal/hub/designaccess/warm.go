package designaccess

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/ly-solutions/harnessly/internal/safego"
)

// A hub that has just started (a rollout, a moved pod) keeps nothing of the store on its disk: the first
// guest of a link waited seconds while the whole project was read. The projects whose guest links were opened lately
// are laid out once, in the background, right after the start, so the first guest reads a warm hub.

const (
	// warmOpened is how lately a link must have been opened for its project to be laid out ahead.
	warmOpened = 14 * 24 * time.Hour
	// warmProjects is how many projects are laid out, the most recently opened first.
	warmProjects = 24
	warmTime     = 5 * time.Minute
)

// warmer is a view that can lay a project out ahead of a read (designview.View).
type warmer interface {
	Warm(ctx context.Context, spaceID, project string, at int64) error
}

type warmState struct{ started atomic.Bool }

// warmOnce lays the lately opened projects out, once in the hub's life.
func (s *Service) warmOnce() {
	w, ok := s.view.(warmer)
	if !ok || !s.warm.started.CompareAndSwap(false, true) {
		return
	}
	safego.Go("design warm", func() {
		ctx, cancel := context.WithTimeout(context.Background(), warmTime)
		defer cancel()
		n, err := s.warmOpened(ctx, w)
		if err != nil {
			s.log.Warn("design: the lately opened projects were not laid out ahead", "done", n, "error", err)
			return
		}
		s.log.Info("design: the lately opened projects are laid out ahead of their guests", "projects", n)
	})
}

// warmOpened lays out the projects of the live guest links opened lately, one after another (each reads its own
// contents several at a time), and says how many it did.
func (s *Service) warmOpened(ctx context.Context, w warmer) (int, error) {
	now := s.now()
	rows, err := s.db.S().Q().Query(ctx, `SELECT m.space_id, l.project_id, MAX(l.last_opened_at) AS opened FROM design_share_links l
		JOIN machines m ON m.id = l.machine_id
		WHERE l.revoked_at IS NULL AND (l.expires_at IS NULL OR l.expires_at > ?) AND l.last_opened_at > ?
			AND m.space_id IS NOT NULL AND m.space_id <> ''
		GROUP BY m.space_id, l.project_id ORDER BY opened DESC LIMIT ?`, ms(now), ms(now.Add(-warmOpened)), warmProjects)
	if err != nil {
		return 0, err
	}
	type target struct{ space, project string }
	var all []target
	for rows.Next() {
		var t target
		var opened int64
		if err := rows.Scan(&t.space, &t.project, &opened); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	done := 0
	for _, t := range all {
		if ctx.Err() != nil {
			return done, ctx.Err()
		}
		head, err := s.view.Head(ctx, t.space, t.project)
		if err != nil || head == 0 {
			continue
		}
		if err := w.Warm(ctx, t.space, t.project, head); err != nil {
			s.log.Warn("design: a project not laid out ahead", "space", t.space, "project", t.project, "error", err)
			continue
		}
		done++
	}
	return done, nil
}
