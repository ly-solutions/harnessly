package designaccess

import (
	"context"

	"github.com/ly-solutions/harnessly/internal/hub/eventbus"
)

// A change of access is said on the hub's bus (access.changed): every replica's held page polls of the space
// wake on it (SpaceChanged), and a worker of the leader sends the new snapshot to the space's machines, whichever part
// of the hub changed the access (a grant here, a member or a team elsewhere, an expiry).

// Signaler is the part of the bus the service uses (eventbus.Bus).
type Signaler interface {
	Signal(typ, key string, facts map[string]string)
	Follow(types []string, fn func(eventbus.Event))
}

// UseBus makes the service say its changes of access on the bus and hear the bus's; nil leaves the polls as they were.
// Call it before the hub serves.
func (s *Service) UseBus(b Signaler) {
	if b == nil {
		return
	}
	s.sigMu.Lock()
	s.bus = b
	s.sigMu.Unlock()
	b.Follow([]string{eventbus.TypeAccessChanged}, func(e eventbus.Event) {
		if space := e.Facts["space"]; space != "" {
			s.SpaceChanged(space)
		}
	})
}

// AccessChanged says on the bus that who may see what of a space changed; project or person may be "".
func (s *Service) AccessChanged(spaceID, project, person string) {
	s.sigMu.Lock()
	b := s.bus
	s.sigMu.Unlock()
	if b == nil || spaceID == "" {
		return
	}
	facts := map[string]string{"space": spaceID}
	if project != "" {
		facts["project"] = project
	}
	if person != "" {
		facts["person"] = person
	}
	b.Signal(eventbus.TypeAccessChanged, spaceID+"\x00"+project+"\x00"+person, facts)
}

// onBus: the service says its changes of access on the bus.
func (s *Service) onBus() bool {
	s.sigMu.Lock()
	defer s.sigMu.Unlock()
	return s.bus != nil
}

// SignalConsumer is the leader's worker of access.changed on the bus.
const SignalConsumer = "sinopia"

// HandleSignal is the worker's handler (eventbus.Handler): the new snapshot goes to every machine of the space, which
// is how a member or a team changed elsewhere in the hub reaches the runtimes.
func (s *Service) HandleSignal(ctx context.Context, e eventbus.Event) error {
	if e.Type != eventbus.TypeAccessChanged {
		return nil
	}
	space := e.Facts["space"]
	if space == "" {
		return nil
	}
	for _, id := range s.spaceMachines(ctx, space) {
		s.kick(id)
	}
	return nil
}

// spaceOf is the space of a machine; "" when it is not known.
func (s *Service) spaceOf(ctx context.Context, machineID string) string {
	var space string
	if err := s.db.S().Q().QueryRow(ctx, `SELECT COALESCE(space_id, '') FROM machines WHERE id = ?`, machineID).Scan(&space); err != nil {
		return ""
	}
	return space
}
