package designaccess

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// a project shared by email with a person of another space (tenant). The person is no member of the owner's
// space and has no machine there, so their library is read under a machine of their own space; the project is a card of
// its own there, named by its card key (cardkey.go), and it is read from the hub's store of the owner's space only:
// neither the person's machine nor the owner's takes part, and nothing of the owner's space but that project is open.
//
// Only a personal grant crosses spaces: the person's own email among the project's members, or an invitation they
// accepted. A team grant, the team level and the space level stay inside their space. Without a file store (Librarian)
// no project crosses spaces, since its files would have to be read from the owner's machine.
//
// A project from another space opens read-only: comments and edits go through the owner's space and stay there.

// foreignOn tells whether projects may be read across spaces: the files are the hub's store's.
func (s *Service) foreignOn() bool {
	_, ok := s.view.(Librarian)
	return ok
}

// foreignCandidates are the projects of spaces the person is no member of that name them personally: (project, holder).
func (s *Service) foreignCandidates(ctx context.Context, u store.User) ([][2]string, error) {
	if !s.foreignOn() || u.DisabledAt != nil {
		return nil, nil
	}
	email := normEmail(u.Email)
	verified, err := s.db.S().UserIDByVerifiedEmail(ctx, email)
	if err != nil || verified == "" || verified != u.ID {
		return nil, err
	}
	rows, err := s.db.S().Q().Query(ctx, `SELECT p.project_id, p.machine_id FROM design_projects p JOIN machines m ON m.id=p.machine_id
		WHERE m.space_id <> '' AND m.disabled_at IS NULL AND m.revoked_at IS NULL
		AND m.space_id NOT IN (SELECT space_id FROM memberships WHERE user_id=?)
		AND p.owner_email <> ? AND (
			EXISTS (SELECT 1 FROM design_members g WHERE g.machine_id=p.machine_id AND g.project_id=p.project_id AND g.person_email=?)
			OR EXISTS (SELECT 1 FROM design_invites i JOIN design_invite_acceptances a ON a.invite_id=i.id WHERE i.machine_id=p.machine_id AND i.project_id=p.project_id AND a.person_email=?))
		ORDER BY p.updated_at DESC, p.project_id, p.machine_id`, u.ID, email, email, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var c [2]string
		if err := rows.Scan(&c[0], &c[1]); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// foreignHolder is the machine of another space whose project the card names, when the person holds a personal grant
// in it now; false otherwise (a card of the person's own space is resolved there, as before).
func (s *Service) foreignHolder(ctx context.Context, anchor store.Machine, u store.User, project, card string) (store.Machine, bool, error) {
	if card == "" || !cardShape(card) || !projectIDRe.MatchString(project) {
		return store.Machine{}, false, nil
	}
	cands, err := s.foreignCandidates(ctx, u)
	if err != nil {
		return store.Machine{}, false, err
	}
	for _, c := range cands {
		if c[0] != project || cardKey(c[1], project) != card {
			continue
		}
		m, err := s.db.S().MachineByID(ctx, c[1])
		if err != nil {
			return store.Machine{}, false, err
		}
		if m.SpaceID == "" || m.SpaceID == anchor.SpaceID {
			return store.Machine{}, false, nil
		}
		t, err := s.read(ctx, m)
		if err != nil {
			return store.Machine{}, false, err
		}
		p, stored, err := t.effective(project)
		if err != nil || !stored {
			return store.Machine{}, false, err
		}
		f, err := t.fold(p, normEmail(u.Email))
		if err != nil {
			return store.Machine{}, false, err
		}
		if f.Role == "" || (f.Source != "person" && f.Source != "link") {
			return store.Machine{}, false, nil
		}
		return m, true, nil
	}
	return store.Machine{}, false, nil
}

// foreignLibrary are the cards of the projects other spaces shared with the person, read from the store.
func (s *Service) foreignLibrary(ctx context.Context, u store.User, anchor store.Machine) ([]storeArtifact, error) {
	lister, ok := s.view.(Librarian)
	if !ok {
		return nil, nil
	}
	cands, err := s.foreignCandidates(ctx, u)
	if err != nil || len(cands) == 0 {
		return nil, err
	}
	email := normEmail(u.Email)
	out := []storeArtifact{}
	listed := map[string]bool{}
	for _, c := range cands {
		id, holder := c[0], c[1]
		key := cardKey(holder, id)
		if listed[key] {
			continue
		}
		m, err := s.db.S().MachineByID(ctx, holder)
		if err != nil {
			return nil, err
		}
		if m.SpaceID == "" || m.SpaceID == anchor.SpaceID {
			continue
		}
		t, err := s.read(ctx, m)
		if err != nil {
			return nil, err
		}
		p, stored, err := t.effective(id)
		if err != nil {
			return nil, err
		}
		if !stored {
			continue
		}
		f, err := t.fold(p, email)
		if err != nil {
			return nil, err
		}
		if f.Role == "" || (f.Source != "person" && f.Source != "link") {
			continue
		}
		items, err := lister.Library(ctx, m.SpaceID, []string{id})
		if err != nil {
			return nil, err
		}
		var it *LibraryItem
		for i := range items {
			if items[i].ID == id {
				it = &items[i]
			}
		}
		if it == nil {
			// Not in the store yet: nothing to read without the owner's machine.
			continue
		}
		title := it.Title
		if title == "" {
			title = id
		}
		a := storeArtifact{ID: id, CardKey: key, Title: title, UpdatedAt: it.UpdatedAt, Kind: it.Kind, Boards: it.Boards, Convs: []string{},
			Role: foreignRole(f.Role), AccessSource: f.Source, AccessUntil: f.Until}
		if a.Boards == nil {
			a.Boards = []LibraryBoard{}
		}
		if p.Owner != "" {
			name := p.OwnerName
			if name == "" {
				name, _, _ = strings.Cut(p.Owner, "@")
			}
			a.Owner = &storeOwner{Name: name}
		}
		listed[key] = true
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// foreignRole is what a person of another space may do there: read, never comment, edit or manage (those go through
// the owner's space).
func foreignRole(role string) string {
	if rank(role) > rank(roleView) {
		return roleView
	}
	return role
}

// foreignCard is the card a board's key carries: a project of another space is found again only by its card.
func foreignCard(foreign bool, card string) string {
	if foreign {
		return card
	}
	return ""
}

// foreignInvite is the machine of another space an invitation names: the person accepts it from a machine of their own
// space. Only with a store, and only for a verified account; the invitation's own rules (its email, its term) are
// checked when it is accepted.
func (s *Service) foreignInvite(ctx context.Context, anchor store.Machine, u store.User, invite string) (store.Machine, bool, error) {
	if invite == "" || !s.foreignOn() || anchor.SpaceID == "" || !store.ValidDesignInviteToken(invite) {
		return store.Machine{}, false, nil
	}
	var id, space string
	err := s.db.S().Q().QueryRow(ctx, `SELECT i.machine_id, m.space_id FROM design_invites i JOIN machines m ON m.id=i.machine_id
		WHERE i.token_hash=? AND m.revoked_at IS NULL AND m.disabled_at IS NULL`, store.HashToken(invite)).Scan(&id, &space)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Machine{}, false, nil
	}
	if err != nil {
		return store.Machine{}, false, err
	}
	if space == "" || space == anchor.SpaceID {
		return store.Machine{}, false, nil
	}
	verified, err := s.db.S().UserIDByVerifiedEmail(ctx, normEmail(u.Email))
	if err != nil {
		return store.Machine{}, false, err
	}
	if u.DisabledAt != nil || verified != u.ID {
		return store.Machine{}, false, fail("forbidden", "Нужен подтверждённый аккаунт")
	}
	m, err := s.db.S().MachineByID(ctx, id)
	if err != nil {
		return store.Machine{}, false, err
	}
	return m, true, nil
}
