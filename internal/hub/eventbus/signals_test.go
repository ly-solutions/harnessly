package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"
)

// a signal journaled by one replica reaches the followers of another, a burst of one key is one event, and
// the signals leave the journal after SignalRetention while the other events stay.
func TestSignalsReachTheOtherReplicaAndExpire(t *testing.T) {
	a, bdb := openDB(t)
	c := &clock{t: time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)}
	ba, bb := New(a, nil, c.now), New(bdb, nil, c.now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	prev := FollowEvery
	FollowEvery = 50 * time.Millisecond
	defer func() { FollowEvery = prev }()

	var mu sync.Mutex
	var heard []Event
	bb.Follow([]string{TypeAccessChanged}, func(e Event) {
		mu.Lock()
		heard = append(heard, e)
		mu.Unlock()
	})
	done := make(chan struct{})
	go func() { bb.Tail(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond) // Tail starts at the head: what came before is not heard

	ba.sig.mu.Lock()
	ba.sig.started = true // no background flusher: the test flushes
	ba.sig.mu.Unlock()
	ba.Signal(TypeAccessChanged, "s1", map[string]string{"space": "s1", "project": "p1"})
	ba.Signal(TypeAccessChanged, "s1", map[string]string{"space": "s1", "project": "p2"})
	ba.Signal(TypeAccessChanged, "s2", map[string]string{"space": "s2"})
	ba.Signal(TypeRevisionCommitted, "s1", map[string]string{"space": "s1", "kind": "design"})
	ba.FlushSignals(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(heard)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	got := append([]Event(nil), heard...)
	mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("heard %d signals, want 2 (one per key, only access.changed): %+v", len(got), got)
	}
	if got[0].Facts["space"] != "s1" || got[0].Facts["project"] != "p2" || got[1].Facts["space"] != "s2" {
		t.Fatalf("facts %v %v", got[0].Facts, got[1].Facts)
	}
	if got[0].Owner != "" || got[0].Space != "" || got[0].Source != SignalSource {
		t.Fatalf("a signal is the deployment's, with no space of the journal: %+v", got[0])
	}

	// A person's event stays for the retention; the signals go after a day.
	if _, _, err := ba.Append(ctx, Event{Owner: "u1", Type: "chat.message", Source: "telegram", Ext: "m1", Via: []string{ViaHarnessly}}); err != nil {
		t.Fatal(err)
	}
	c.add(SignalRetention + time.Hour)
	if _, err := ba.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	left, err := ba.List(ctx, Query{All: true, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Owner != "u1" {
		t.Fatalf("left after the sweep: %+v", left)
	}
	cancel()
	<-done
}
