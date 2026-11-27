package storehub

import (
	"github.com/ly-solutions/harnessly/internal/hub/eventbus"
)

// The store says on the hub's bus what it took (revision.committed) and hears there who may now see what
// (access.changed): whichever part of the hub changed a space's roles (Sinopia, a member, a team), the space's machines
// look at their scope at once (AccessChanged), on every replica.

// Signaler is the part of the bus the store uses (eventbus.Bus).
type Signaler interface {
	Signal(typ, key string, facts map[string]string)
	Follow(types []string, fn func(eventbus.Event))
}

// UseBus makes the store journal its writes and follow the changes of access; nil leaves both off. Call it before the
// store serves.
func (s *Store) UseBus(b Signaler) {
	if b == nil {
		return
	}
	s.Observe(func(spaceID, kind, path string, _ bool) {
		facts := map[string]string{"space": spaceID, "kind": kind}
		key := spaceID + "\x00" + kind
		if kind == "design" {
			if id, ok := projectOf(path); ok {
				facts["project"] = id
				key += "\x00" + id
			}
		}
		b.Signal(eventbus.TypeRevisionCommitted, key, facts)
	})
	b.Follow([]string{eventbus.TypeAccessChanged}, func(e eventbus.Event) {
		if space := e.Facts["space"]; space != "" {
			s.AccessChanged(space)
		}
	})
}
