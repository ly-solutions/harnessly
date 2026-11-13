// Package leader picks, of the replicas of a hub (docs/hub/replicas.md), the one that does the background jobs that
// must run once: the passes that hand something over to machines or to the outside world, or that would race each
// other over the same rows. It is one lease of the table job_leases, "hub-leader", taken and renewed by every replica
// every few seconds: whoever holds it does the led jobs, the others wait and take over within a lease once the holder
// stops renewing (it was stopped, killed, or cut from the database). A clean stop gives the lease up at once.
//
// The jobs that serve only this replica (its links, its streams, its caches) do not go through it.
package leader

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/safego"
)

// Lease is the name of the lease in job_leases.
const Lease = "hub-leader"

const (
	// TTL is how long a taken lease lasts; Every is how often it is renewed. A holder that could not renew for
	// TTL-Every takes itself for a follower, before the lease is anybody's, so two replicas never lead at once
	// (clocks of the replicas agreeing within a couple of seconds).
	TTL   = 15 * time.Second
	Every = 5 * time.Second
)

// Leader is this replica's part in the election. A nil *Leader leads always: a hub of one process does every job.
type Leader struct {
	db    *store.DB
	id    string
	log   *slog.Logger
	now   func() time.Time
	ttl   time.Duration
	every time.Duration
	mu    sync.Mutex
	until time.Time // this replica may lead until then; zero when it does not
	wake  chan struct{}
}

// New makes the part of replica id. Nothing is taken until Run.
func New(db *store.DB, id string, log *slog.Logger, now func() time.Time) *Leader {
	if log == nil {
		log = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	return &Leader{db: db, id: id, log: log, now: now, ttl: TTL, every: Every, wake: make(chan struct{})}
}

// Held says whether this replica leads now.
func (l *Leader) Held() bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.now().Before(l.until)
}

// Run takes and renews the lease until ctx ends, then gives it up.
func (l *Leader) Run(ctx context.Context) {
	l.try(ctx)
	t := time.NewTicker(l.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			l.set(time.Time{})
			rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := l.db.S().ReleaseLease(rctx, Lease, l.id); err != nil {
				l.log.Warn("leader: lease not given up, the next replica takes it when it runs out", "error", err)
			}
			cancel()
			return
		case <-t.C:
			l.try(ctx)
		}
	}
}

func (l *Leader) try(ctx context.Context) {
	start := l.now()
	ok, err := l.db.S().TakeLease(ctx, Lease, l.id, start, l.ttl)
	switch {
	case err != nil:
		if ctx.Err() == nil {
			l.log.Warn("leader: lease not renewed", "error", err)
		}
		// keep what was held: it runs out by itself (until) before the lease is anybody's
	case ok:
		l.set(start.Add(l.ttl - l.every))
	default:
		l.set(time.Time{})
	}
}

func (l *Leader) set(until time.Time) {
	l.mu.Lock()
	was := l.now().Before(l.until)
	l.until = until
	is := l.now().Before(until)
	var wake chan struct{}
	if was != is {
		wake = l.wake
		l.wake = make(chan struct{})
	}
	l.mu.Unlock()
	if wake != nil {
		close(wake)
		if is {
			l.log.Info("leader: this replica leads the background jobs", "replica", l.id)
		} else {
			l.log.Info("leader: this replica no longer leads the background jobs", "replica", l.id)
		}
	}
}

// changed is closed at the next change of Held (a change found by a renewal; a lease that ran out between two is
// found by the poll in Lead).
func (l *Leader) changed() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.wake
}

// Lead runs job while this replica leads, until ctx ends: job gets a context that ends when the lead is lost, and is
// started again when it comes back. With a nil Leader job simply runs with ctx.
func (l *Leader) Lead(ctx context.Context, name string, job func(context.Context)) {
	if l == nil {
		job(ctx)
		return
	}
	for ctx.Err() == nil {
		if !l.Held() {
			select {
			case <-ctx.Done():
				return
			case <-l.changed():
			case <-time.After(time.Second):
			}
			continue
		}
		jctx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		safego.Go("led "+name, func() {
			defer close(done)
			job(jctx)
		})
		l.log.Info("leader: job started", "job", name, "replica", l.id)
		finished := false
		for !finished && l.Held() && ctx.Err() == nil {
			select {
			case <-ctx.Done():
			case <-done:
				finished = true
			case <-l.changed():
			case <-time.After(time.Second):
			}
		}
		cancel()
		<-done
		if ctx.Err() != nil {
			return
		}
		if finished {
			// a job that ended by itself (it has nothing to do here) is started again only after the lead moves away
			// and comes back
			for l.Held() && ctx.Err() == nil {
				select {
				case <-ctx.Done():
				case <-l.changed():
				case <-time.After(time.Second):
				}
			}
			continue
		}
		l.log.Info("leader: job stopped, the lead moved", "job", name, "replica", l.id)
	}
}
