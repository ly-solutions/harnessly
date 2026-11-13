package link

import (
	"context"
	"testing"
	"time"
)

// A machine whose replica stopped and sent it to dial another one is waited for: the request reaches its next link
// instead of answering «offline».
func TestAwaitWaitsForTheNextLinkOfAMachineHandedOver(t *testing.T) {
	h := newTestHub(t)
	ctx := context.Background()
	s := h.db.S()
	id := h.machine.ID
	fence, err := s.AcquireLink(ctx, id, "a", "http://a", time.Now().UTC(), DefaultLeaseTTL)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseLink(ctx, id, "a", fence, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	reg.SetCluster(&Cluster{DB: h.db, ID: "c", Addr: "http://c"})
	if reg.Get(id) != nil {
		t.Fatal("a released link is still found")
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		if _, err := s.AcquireLink(ctx, id, "b", "http://b", time.Now().UTC(), DefaultLeaseTTL); err != nil {
			t.Error(err)
		}
	}()
	start := time.Now()
	l := reg.Await(ctx, id)
	if l == nil || l.Owner() != "b" {
		t.Fatalf("Await = %v, want the link replica b took", l)
	}
	if d := time.Since(start); d < 250*time.Millisecond || d > handoverWait {
		t.Fatalf("waited %v", d)
	}
}

// A machine that has been away for a while is answered at once.
func TestAwaitDoesNotWaitForAMachineLongAway(t *testing.T) {
	h := newTestHub(t)
	ctx := context.Background()
	s := h.db.S()
	id := h.machine.ID
	past := time.Now().UTC().Add(-time.Minute)
	fence, err := s.AcquireLink(ctx, id, "a", "http://a", past, DefaultLeaseTTL)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseLink(ctx, id, "a", fence, past); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	reg.SetCluster(&Cluster{DB: h.db, ID: "c", Addr: "http://c"})
	start := time.Now()
	if l := reg.Await(ctx, id); l != nil {
		t.Fatalf("Await = %v, want nil", l)
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("an away machine waited %v", d)
	}
	// Nor one that never linked.
	if l := reg.Await(ctx, "nosuchmachine"); l != nil {
		t.Fatal("a machine never linked was found")
	}
}

// A lease that ran out a moment ago (its replica was killed) counts as a handover too.
func TestHandingOverAfterALeaseRanOut(t *testing.T) {
	h := newTestHub(t)
	ctx := context.Background()
	id := h.machine.ID
	if _, err := h.db.S().AcquireLink(ctx, id, "a", "http://a", time.Now().UTC().Add(-2*time.Second), time.Second); err != nil {
		t.Fatal(err)
	}
	c := &Cluster{DB: h.db, ID: "c", Addr: "http://c"}
	if !c.handingOver(id) {
		t.Fatal("a lease that ran out a second ago is not a handover")
	}
	if c.handingOver("nosuchmachine") {
		t.Fatal("a machine never linked is a handover")
	}
}
