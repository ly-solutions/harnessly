package storehub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/eventbus"
)

type fakeBus struct {
	mu      sync.Mutex
	signals []map[string]string
	follows []func(eventbus.Event)
}

func (b *fakeBus) Signal(typ, _ string, facts map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f := map[string]string{"type": typ}
	for k, v := range facts {
		f[k] = v
	}
	b.signals = append(b.signals, f)
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

// a write is said on the bus as revision.committed, and access.changed of the space wakes a machine's held
// changes poll at once with its new scope (before, it waited for the view cache and the end of the poll).
func TestAGrantWakesTheHeldPollWithTheNewScope(t *testing.T) {
	ctx := context.Background()
	machines := fakeMachines{
		"owner": {ID: "owner", Name: "vm-owner", SpaceID: "sp1"},
		"mate":  {ID: "mate", Name: "vm-mate", SpaceID: "sp1"},
	}
	design := &fakeDesign{view: map[string]map[string]Grant{
		"owner": {"secret": {See: true, Edit: true, Own: true}},
		"mate":  {"secret": {See: false}},
	}}
	st, client := designStore(t, machines, design)
	bus := &fakeBus{}
	st.UseBus(bus)
	owner, mate := client("owner"), client("mate")
	if _, err := owner.Put(ctx, "secret/project/Main.dc.html", []byte("v1"), "t"); err != nil {
		t.Fatal(err)
	}
	bus.mu.Lock()
	var said bool
	for _, s := range bus.signals {
		if s["type"] == eventbus.TypeRevisionCommitted && s["space"] == "sp1" && s["kind"] == "design" && s["project"] == "secret" {
			said = true
		}
	}
	bus.mu.Unlock()
	if !said {
		t.Fatalf("the write was not said on the bus: %v", bus.signals)
	}

	ch, err := mate.Changes(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	type answer struct {
		scope string
		err   error
		at    time.Duration
	}
	got := make(chan answer, 1)
	start := time.Now()
	go func() {
		out, err := mate.Changes(ctx, ch.Rev, 25*time.Second)
		got <- answer{out.Scope, err, time.Since(start)}
	}()
	time.Sleep(300 * time.Millisecond)

	// The mate is given the project; the hub says so on the bus.
	design.mu.Lock()
	design.view["mate"] = map[string]Grant{"secret": {See: true}}
	design.mu.Unlock()
	bus.say(eventbus.Event{Type: eventbus.TypeAccessChanged, Facts: map[string]string{"space": "sp1"}})

	select {
	case a := <-got:
		if a.err != nil {
			t.Fatal(a.err)
		}
		if a.scope == ch.Scope {
			t.Fatalf("the poll answered the old scope %q", a.scope)
		}
		if a.at > 5*time.Second {
			t.Fatalf("answered after %s", a.at)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the held poll did not wake on access.changed")
	}
}
