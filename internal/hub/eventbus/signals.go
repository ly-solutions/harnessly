package eventbus

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// The hub's own signals: what changed in it that other replicas and workers act on, journaled like any event
// so every replica hears them and a worker subscribes to them. They are the deployment's (owner ""), carry no space of
// the journal (a person's view of their spaces' team events never lists them: the facts name people and projects) and
// are kept for SignalRetention only.
const (
	// TypeRevisionCommitted: the store took a write. Facts: space, kind (the collection), project (a design project, when
	// the path is in one).
	TypeRevisionCommitted = "revision.committed"
	// TypeAccessChanged: who may see or edit what of a space changed (a grant, a team, a member, an expiry). Facts:
	// space, and project or person when one is known.
	TypeAccessChanged = "access.changed"
	// SignalSource is the source of the hub's own signals.
	SignalSource = "harnessly"
)

// SignalTypes are the types the journal keeps for SignalRetention only.
var SignalTypes = []string{TypeRevisionCommitted, TypeAccessChanged}

// SignalRetention is how long a signal stays in the journal: a follower or a worker behind by more has nothing to catch
// up on that its next read of the state would not give.
const SignalRetention = 24 * time.Hour

// FollowEvery is how often a replica's follower looks for what another replica journaled; what this replica journals
// it hears at once.
var FollowEvery = time.Second

// signalFlushDelay gathers a burst of the same signal into one event.
var signalFlushDelay = 200 * time.Millisecond

type signals struct {
	mu      sync.Mutex
	pending map[string]map[string]string // type \x00 key -> facts
	order   []string
	wake    chan struct{}
	started bool

	fmu     sync.RWMutex
	follows []follow
	grew    chan struct{} // closed when this replica journals a signal
}

type follow struct {
	types []string
	fn    func(Event)
}

// Signal journals one of the hub's own signals off the caller's path: it returns at once, and a burst with the same type
// and key is one event with the last facts. A nil bus does nothing.
func (b *Bus) Signal(typ, key string, facts map[string]string) {
	if b == nil {
		return
	}
	s := &b.sig
	k := typ + "\x00" + key
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]map[string]string{}
		s.wake = make(chan struct{}, 1)
	}
	if _, ok := s.pending[k]; !ok {
		s.order = append(s.order, k)
	}
	s.pending[k] = facts
	if !s.started {
		s.started = true
		go b.flushLoop()
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (b *Bus) flushLoop() {
	for range b.sig.wake {
		time.Sleep(signalFlushDelay)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		b.FlushSignals(ctx)
		cancel()
	}
}

// FlushSignals journals the signals gathered so far.
func (b *Bus) FlushSignals(ctx context.Context) {
	s := &b.sig
	s.mu.Lock()
	order, pending := s.order, s.pending
	s.order, s.pending = nil, map[string]map[string]string{}
	s.mu.Unlock()
	for _, k := range order {
		typ := k[:indexNul(k)]
		e := Event{Owner: "", Type: typ, Source: SignalSource, Ext: typ + ":" + store.NewID(), Facts: pending[k], Via: []string{ViaHarnessly}}
		if _, _, err := b.Append(ctx, e); err != nil {
			b.log.Warn("event bus: a signal not journaled", "type", typ, "error", err)
		}
	}
}

func indexNul(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return i
		}
	}
	return len(s)
}

// Follow makes fn hear, on this replica, every signal of these types journaled after Tail starts: from any replica,
// at least once and in the order of the journal. fn must not block. A nil bus does nothing. Call it before Tail.
func (b *Bus) Follow(types []string, fn func(Event)) {
	if b == nil || fn == nil {
		return
	}
	b.sig.fmu.Lock()
	b.sig.follows = append(b.sig.follows, follow{types: types, fn: fn})
	b.sig.fmu.Unlock()
}

// grown says that this replica journaled something: its followers read at once.
func (b *Bus) grown() {
	s := &b.sig
	s.fmu.Lock()
	if s.grew != nil {
		close(s.grew)
	}
	s.grew = make(chan struct{})
	s.fmu.Unlock()
}

func (b *Bus) grewCh() <-chan struct{} {
	s := &b.sig
	s.fmu.Lock()
	defer s.fmu.Unlock()
	if s.grew == nil {
		s.grew = make(chan struct{})
	}
	return s.grew
}

// Tail runs this replica's followers until ctx ends (every replica runs it, not only the leader). It starts at the end
// of the journal: what was journaled before is not heard.
func (b *Bus) Tail(ctx context.Context) {
	b.sig.fmu.RLock()
	n := len(b.sig.follows)
	b.sig.fmu.RUnlock()
	if n == 0 {
		return
	}
	var after int64
	for ctx.Err() == nil {
		h, err := b.Head(ctx)
		if err == nil {
			after = h
			break
		}
		b.log.Warn("event bus: the head of the journal not read", "error", err)
		sleepCtx(ctx, FollowEvery)
	}
	tick := time.NewTicker(FollowEvery)
	defer tick.Stop()
	for ctx.Err() == nil {
		grew := b.grewCh()
		next, err := b.TailOnce(ctx, after)
		if err != nil {
			b.log.Warn("event bus: the signals not read", "error", err)
		}
		if next > after {
			after = next
			continue // there may be more
		}
		select {
		case <-ctx.Done():
		case <-grew:
		case <-tick.C:
		}
	}
}

// TailOnce hands the followers the signals after seq, a page of them, and says the last seq it read (after when none).
func (b *Bus) TailOnce(ctx context.Context, after int64) (int64, error) {
	b.sig.fmu.RLock()
	follows := slices.Clone(b.sig.follows)
	b.sig.fmu.RUnlock()
	var types []string
	for _, f := range follows {
		for _, t := range f.types {
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
	}
	if len(types) == 0 {
		return after, nil
	}
	args := []any{after}
	in := ""
	for i, t := range types {
		if i > 0 {
			in += ", "
		}
		in += "?"
		args = append(args, t)
	}
	rows, err := b.db.Query(ctx, `SELECT `+eventSelect+` FROM bus_events WHERE owner_id = '' AND seq > ? AND type IN (`+in+`) ORDER BY seq LIMIT 500`, args...)
	if err != nil {
		return after, err
	}
	var got []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			rows.Close()
			return after, err
		}
		got = append(got, e)
	}
	err = rows.Err()
	rows.Close()
	for _, e := range got {
		after = e.Seq
		for _, f := range follows {
			if slices.Contains(f.types, e.Type) {
				f.fn(e)
			}
		}
	}
	return after, err
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// sweepSignals drops the signals older than SignalRetention.
func (b *Bus) sweepSignals(ctx context.Context) (int64, error) {
	before := store.Ms(b.now().Add(-SignalRetention))
	var total int64
	for {
		res, err := b.db.Exec(ctx, `DELETE FROM bus_events WHERE seq IN (SELECT seq FROM bus_events WHERE owner_id = '' AND type IN (?, ?) AND received_at < ? ORDER BY seq LIMIT 1000)`,
			TypeRevisionCommitted, TypeAccessChanged, before)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < 1000 {
			return total, nil
		}
	}
}
