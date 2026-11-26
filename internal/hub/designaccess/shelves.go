package designaccess

import (
	"context"
	"errors"
	"sort"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// ShelfKeys names the layout files in .folders/ of the people the machine is for: whoever made it, and everyone its
// access names, by person or by team. A shelf says which projects a person has and how they call their folders, so
// being in the same space is not enough to receive it, and a machine open to the whole space is still only its maker's.
func (s *Service) ShelfKeys(ctx context.Context, m store.Machine) ([]string, error) {
	st := s.db.S()
	entries, err := st.MachineAccess(ctx, []string{m.ID})
	if err != nil {
		return nil, err
	}
	users := map[string]bool{}
	if m.CreatedBy != "" {
		users[m.CreatedBy] = true
	}
	for _, e := range entries {
		switch e.Kind {
		case store.AccessUser:
			users[e.RefID] = true
		case store.AccessTeam:
			members, err := st.TeamMembers(ctx, e.RefID)
			if err != nil {
				return nil, err
			}
			for _, tm := range members {
				users[tm.UserID] = true
			}
		}
	}
	keys := make([]string, 0, len(users))
	for id := range users {
		u, err := st.UserByID(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if email := normEmail(u.Email); email != "" {
			keys = append(keys, layoutKey(email))
		}
	}
	sort.Strings(keys)
	return keys, nil
}
