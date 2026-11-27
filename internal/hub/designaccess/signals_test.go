package designaccess

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/eventbus"
)

type fakeBus struct {
	mu      sync.Mutex
	said    []map[string]string
	follows []func(eventbus.Event)
}

func (b *fakeBus) Signal(typ, _ string, facts map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f := map[string]string{"type": typ}
	for k, v := range facts {
		f[k] = v
	}
	b.said = append(b.said, f)
}

func (b *fakeBus) Follow(_ []string, fn func(eventbus.Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.follows = append(b.follows, fn)
}

func (b *fakeBus) say(e eventbus.Event) {
	b.mu.Lock()
	fs := append([]func(eventbus.Event){}, b.follows...)
	b.mu.Unlock()
	for _, fn := range fs {
		fn(e)
	}
}

// a change of access in Sinopia is said on the bus; the bus wakes the space's held page polls (and only
// that space's); the leader's worker sends the snapshot to every machine of the
// space, whichever part of the hub changed it.
func TestAccessIsSaidOnTheBusAndWakesTheSpace(t *testing.T) {
	ctx := context.Background()
	r := newSpaceRig(t, true)
	bus := &fakeBus{}
	r.svc.UseBus(bus)

	r.svc.kickSpace(ctx, r.owner)
	bus.mu.Lock()
	if len(bus.said) != 1 || bus.said[0]["type"] != eventbus.TypeAccessChanged || bus.said[0]["space"] != r.owner.SpaceID {
		t.Fatalf("said %v", bus.said)
	}
	bus.mu.Unlock()

	wake := r.svc.spaces.on(r.owner.SpaceID)
	other := r.svc.spaces.on("another")
	bus.say(eventbus.Event{Type: eventbus.TypeAccessChanged, Facts: map[string]string{"space": r.owner.SpaceID}})
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("a change of access on the bus did not wake the space's polls")
	}
	select {
	case <-other:
		t.Fatal("another space's polls woke")
	default:
	}

	// The worker: every served machine of the space gets a push.
	r.svc.mu.Lock()
	for _, m := range []string{r.owner.ID, r.mate.ID} {
		r.svc.serving[m] = true
	}
	r.svc.mu.Unlock()
	if err := r.svc.HandleSignal(ctx, eventbus.Event{Type: eventbus.TypeAccessChanged, Facts: map[string]string{"space": r.owner.SpaceID}}); err != nil {
		t.Fatal(err)
	}
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	if r.svc.timers[r.owner.ID] == nil || r.svc.timers[r.mate.ID] == nil {
		t.Fatalf("the space's machines not pushed: %v", r.svc.timers)
	}
	for id, tm := range r.svc.timers {
		tm.Stop()
		delete(r.svc.timers, id)
	}
}
