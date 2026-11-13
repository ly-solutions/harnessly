package designaccess

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ly-solutions/harnessly/internal/safego"
)

// How much of the designs lives in the hub: of the projects the hub has a row for, how many the store has a
// revision of. A project the store has nothing of opens only from a machine, so the number of them is what is still
// to be brought over; the hub counts it now and then, says it in its metrics and names the projects in its log.

const (
	coverageEvery = 30 * time.Minute
	coverageTime  = 2 * time.Minute
	// coverageNamed is how many of the missing projects one line of the log names.
	coverageNamed = 60
)

// Coverage is the last count of the recorded projects by whether the store has them; At is zero before the first.
type Coverage struct {
	Stored, Missing int
	At              time.Time
}

type coverage struct {
	last    atomic.Pointer[Coverage]
	running atomic.Bool
}

// Coverage is the last count; its At is zero when the hub has not counted yet (no store, or just started).
func (s *Service) Coverage() Coverage {
	if c := s.cover.last.Load(); c != nil {
		return *c
	}
	return Coverage{}
}

// coverageDue starts a count when the last one is old enough and none is running.
func (s *Service) coverageDue() {
	if s.view == nil {
		return
	}
	if c := s.cover.last.Load(); c != nil && s.now().Sub(c.At) < coverageEvery {
		return
	}
	if !s.cover.running.CompareAndSwap(false, true) {
		return
	}
	safego.Go("design coverage", func() {
		defer s.cover.running.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), coverageTime)
		defer cancel()
		c, missing, err := s.countCoverage(ctx)
		if err != nil {
			s.log.Warn("design: the projects in the store were not counted", "error", err)
			return
		}
		s.cover.last.Store(&c)
		named := missing
		if len(named) > coverageNamed {
			named = named[:coverageNamed]
		}
		s.log.Info("design: recorded projects by whether the store has a revision of them", "stored", c.Stored, "missing", c.Missing,
			"missingProjects", strings.Join(named, " "))
	})
}

// countCoverage asks the store for the newest revision of every recorded project of every space; missing names the
// ones it has nothing of, as <space>/<project>.
func (s *Service) countCoverage(ctx context.Context) (Coverage, []string, error) {
	rows, err := s.db.S().Q().Query(ctx, `SELECT DISTINCT m.space_id, p.project_id FROM design_projects p
		JOIN machines m ON m.id = p.machine_id WHERE m.revoked_at IS NULL AND m.space_id IS NOT NULL AND m.space_id <> ''`)
	if err != nil {
		return Coverage{}, nil, err
	}
	type key struct{ space, project string }
	var all []key
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.space, &k.project); err != nil {
			rows.Close()
			return Coverage{}, nil, err
		}
		all = append(all, k)
	}
	if err := rows.Close(); err != nil {
		return Coverage{}, nil, err
	}
	c := Coverage{At: s.now()}
	var missing []string
	for _, k := range all {
		head, err := s.view.Head(ctx, k.space, k.project)
		if err != nil {
			return Coverage{}, nil, err
		}
		if head > 0 {
			c.Stored++
			continue
		}
		c.Missing++
		missing = append(missing, k.space+"/"+k.project)
	}
	sort.Strings(missing)
	return c, missing, nil
}
