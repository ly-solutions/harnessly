package main

import (
	"context"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/storelink"
)

// HRN-1790, stage 2. The apps' checks that may keep a project from being shared run on this machine, and the hub asks
// them before it makes a link. With the machine off the hub used to make the link without them. Now the machine runs
// the checks itself a moment after a project changes, at the revision of the store its copy is at, and puts the
// verdict into the project's record; the hub reads it when the machine is
// off and believes it only while that revision is the project's newest.

// verdictStore is the part of the store a verdict is pinned to: the push of a project's folder, the newest revision
// the local copy holds of it and the newest the store holds.
type verdictStore interface {
	PushProject(ctx context.Context, project string) error
	LocalHead(project string) (int64, error)
	StoreHead(ctx context.Context, project string) (int64, error)
}

// shareVerdicts keeps the newest verdict of each project of the library.
type shareVerdicts struct {
	st verdictStore
	// blocked runs the apps' checks of the share trigger on the project's copy (design.API.ShareGate).
	blocked func(ctx context.Context, project string) bool

	mu   sync.Mutex
	done map[string]filestore.ShareVerdict
	// seen is the record's At a project was last settled at: an unchanged project is not checked again.
	seen map[string]int64
}

func newShareVerdicts(st verdictStore, blocked func(ctx context.Context, project string) bool) *shareVerdicts {
	return &shareVerdicts{st: st, blocked: blocked, done: map[string]filestore.ShareVerdict{}, seen: map[string]int64{}}
}

// attach puts each project's verdict into its record.
func (v *shareVerdicts) attach(recs []filestore.Record) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range recs {
		if s, ok := v.done[recs[i].ID]; ok {
			recs[i].Share = &s
		}
	}
}

// refresh settles the verdict of every record that changed since its last one and tells whether a verdict changed.
func (v *shareVerdicts) refresh(ctx context.Context, recs []filestore.Record) bool {
	changed := false
	for _, r := range recs {
		if ctx.Err() != nil {
			return changed
		}
		v.mu.Lock()
		at, had := v.seen[r.ID]
		v.mu.Unlock()
		if had && at == r.At {
			continue
		}
		s, ok := v.check(ctx, r.ID)
		if !ok {
			continue
		}
		v.mu.Lock()
		if v.done[r.ID] != s {
			v.done[r.ID] = s
			changed = true
		}
		v.seen[r.ID] = r.At
		v.mu.Unlock()
	}
	return changed
}

// check runs the checks on a copy that is the store's newest revision of the project and stays it while they run; a
// copy behind the store, ahead of it or changed meanwhile gets no verdict now (the next change asks again).
func (v *shareVerdicts) check(ctx context.Context, project string) (filestore.ShareVerdict, bool) {
	rev, ok := v.settled(ctx, project)
	if !ok {
		return filestore.ShareVerdict{}, false
	}
	v.mu.Lock()
	cur, had := v.done[project]
	v.mu.Unlock()
	if had && cur.Rev == rev {
		return cur, true
	}
	blocked := v.blocked(ctx, project)
	if after, ok := v.settled(ctx, project); !ok || after != rev {
		return filestore.ShareVerdict{}, false
	}
	return filestore.ShareVerdict{Rev: rev, Blocked: blocked}, true
}

// settled sends the project's local changes to the store and answers the revision the copy and the store agree on.
func (v *shareVerdicts) settled(ctx context.Context, project string) (int64, bool) {
	if err := v.st.PushProject(ctx, project); err != nil {
		return 0, false
	}
	local, err := v.st.LocalHead(project)
	if err != nil || local <= 0 {
		return 0, false
	}
	remote, err := v.st.StoreHead(ctx, project)
	if err != nil || remote != local {
		return 0, false
	}
	return local, true
}

// run settles the verdicts once now, then a while after each kick, and calls changed when one changed. It returns when
// ctx ends.
func (v *shareVerdicts) run(ctx context.Context, source func() []filestore.Record, kick <-chan struct{}, changed func()) {
	settle := func() {
		if v.refresh(ctx, source()) {
			changed()
		}
	}
	settle()
	// A project whose copy was not the store's when it changed (the sync had not started yet, or another machine's
	// edit was on its way) is asked again on this tick.
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			settle()
		case <-kick:
			// An editor writes many files in a row, and the sync sends them a moment later.
			select {
			case <-time.After(verdictSettle):
			case <-ctx.Done():
				return
			}
			select {
			case <-kick:
			default:
			}
			settle()
		}
	}
}

// verdictSettle is how long after a change the checks run.
var verdictSettle = 20 * time.Second

// linkVerdicts is the hub's store as the verdicts see it.
type linkVerdicts struct{ l *storelink.Link }

func (s linkVerdicts) PushProject(ctx context.Context, project string) error {
	return s.l.PushNow(ctx, filestore.KindDesign, project)
}

func (s linkVerdicts) LocalHead(project string) (int64, error) {
	return s.l.HeadUnder(filestore.KindDesign, project)
}

func (s linkVerdicts) StoreHead(ctx context.Context, project string) (int64, error) {
	es, err := s.l.Client(filestore.KindDesign).HistoryUnder(ctx, project+"/", 1)
	if err != nil || len(es) == 0 {
		return 0, err
	}
	return es[0].Rev, nil
}
