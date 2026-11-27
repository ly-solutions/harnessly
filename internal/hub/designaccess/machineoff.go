package designaccess

import (
	"context"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// HRN-1799 (stages 4, 6 and 7). The machine is only for the agent's edits; with it off the hub answers the
// rest of Sinopia: the kits' reads from the store, the library's statuses from the facts the machine left, a site
// published from the store, and a comment that calls the agent kept in a queue until the machine is back.

// machineOff: the hub answers for the machine when the store is there. There is no switch: it is how Sinopia works.
func (s *Service) machineOff(_ context.Context, m store.Machine) bool { return s.storeServes(m) }

// machineUp: the machine's link is up now.
func (s *Service) machineUp(id string) bool { return s.reg != nil && s.reg.Get(id) != nil }
