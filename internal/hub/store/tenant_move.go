package store

import (
	"context"
	"time"
)

// MoveMachine takes a live machine to another space, keeping its row (so its id, key and links): it lands in teamID
// ("" for none). Its access list is the caller's to clear.
func (s Store) MoveMachine(ctx context.Context, machineID, spaceID, teamID string) error {
	_, err := s.q.Exec(ctx, `UPDATE machines SET space_id = ?, team_id = ? WHERE id = ? AND revoked_at IS NULL`, spaceID, nullStr(teamID), machineID)
	return err
}

// designTables are the tables that key a design project's rows by its machine.
var designTables = []string{"design_projects", "design_members", "design_team_grants", "design_requests", "design_invites", "design_share_links", "design_access_log"}

// LeaveDesigns keeps the design projects recorded on a machine in the space it moves out of: their files
// are in that space's library, so their rows stay there, under a revoked stand-in of the machine with its name, maker
// and access list. The moved machine takes a copy of each project's row and of the people named in it, for what it
// syncs to its new space, unless that space has a record of the project already. m is the machine as it was before the
// move; call it after MoveMachine, before the access list is cleared, in the same transaction.
func (s Store) LeaveDesigns(ctx context.Context, m Machine, to string, at time.Time) error {
	var n int
	if err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM design_projects WHERE machine_id = ?`, m.ID).Scan(&n); err != nil || n == 0 {
		return err
	}
	access, err := s.MachineAccess(ctx, []string{m.ID})
	if err != nil {
		return err
	}
	left := m
	left.ID = NewID()
	left.KeyPub = "moved:" + left.ID
	left.RevokedAt = &at
	if err := s.CreateMachine(ctx, &left); err != nil {
		return err
	}
	if err := s.SetMachineAccess(ctx, left.ID, access, at); err != nil {
		return err
	}
	// A private snapshot's pages refer to it by the machine: it is copied, its pages moved, the old row dropped.
	if _, err := s.q.Exec(ctx, `INSERT INTO design_private_snapshots (machine_id, project_id, rev, created_by, created_at, expires_at)
		SELECT ?, project_id, rev, created_by, created_at, expires_at FROM design_private_snapshots WHERE machine_id = ?`, left.ID, m.ID); err != nil {
		return err
	}
	for _, table := range append([]string{"design_private_snapshot_pages"}, designTables...) {
		if _, err := s.q.Exec(ctx, `UPDATE `+table+` SET machine_id = ? WHERE machine_id = ?`, left.ID, m.ID); err != nil {
			return err
		}
	}
	if _, err := s.q.Exec(ctx, `DELETE FROM design_private_snapshots WHERE machine_id = ?`, m.ID); err != nil {
		return err
	}
	// The registry of guest links names a design as <machine>/<project>.
	if _, err := s.q.Exec(ctx, `UPDATE resource_guest_links SET space_id = ?, id = (SELECT l.machine_id || '/' || l.project_id FROM design_share_links l WHERE l.id = resource_guest_links.link_id)
		WHERE kind = 'design' AND link_id IN (SELECT id FROM design_share_links WHERE machine_id = ?)`, m.SpaceID, left.ID); err != nil {
		return err
	}
	const free = `machine_id = ? AND project_id NOT IN (SELECT o.project_id FROM design_projects o JOIN machines om ON om.id = o.machine_id WHERE om.space_id = ?)`
	// The people first: once a project's copy is in the new space, it no longer counts as free there.
	if _, err := s.q.Exec(ctx, `INSERT INTO design_members (machine_id, project_id, person_email, role, granted_by, granted_at, expires_at, via_invite)
		SELECT ?, project_id, person_email, role, granted_by, granted_at, expires_at, via_invite FROM design_members WHERE via_invite IS NULL AND `+free,
		m.ID, left.ID, to); err != nil {
		return err
	}
	const cols = `project_id, owner_email, owner_name, home_team, team_level, editors_can_share, legacy, title, subkind, created_at, updated_at, kind, preview, kit_id, work_machine_id, meta_by, meta_at`
	_, err = s.q.Exec(ctx, `INSERT INTO design_projects (machine_id, `+cols+`) SELECT ?, `+cols+` FROM design_projects WHERE `+free, m.ID, left.ID, to)
	return err
}

// CountRevokedMachines is how many revoked machines a space still holds on its row.
func (s Store) CountRevokedMachines(ctx context.Context, spaceID string) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM machines WHERE space_id = ? AND revoked_at IS NOT NULL`, spaceID).Scan(&n)
	return n, err
}

// DetachRevokedMachines takes the revoked machines off a space, so that deleting it does not delete their rows (their
// history is keyed by the machine).
func (s Store) DetachRevokedMachines(ctx context.Context, spaceID string) error {
	_, err := s.q.Exec(ctx, `UPDATE machines SET space_id = NULL, team_id = NULL WHERE space_id = ? AND revoked_at IS NOT NULL`, spaceID)
	return err
}

// CountSitesOfSpace is how many sites carry the space's slug in their address.
func (s Store) CountSitesOfSpace(ctx context.Context, slug string) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM sites WHERE space = ?`, slug).Scan(&n)
	return n, err
}

// CountEnvEntriesOfTenant is how many environment entries are set at the tenant's layer.
func (s Store) CountEnvEntriesOfTenant(ctx context.Context, spaceID string) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM env_entries WHERE layer = ? AND scope_id = ?`, EnvLayerTenant, spaceID).Scan(&n)
	return n, err
}

// CountLimitRulesOfTenant is how many limit rules the tenant and its teams carry.
func (s Store) CountLimitRulesOfTenant(ctx context.Context, spaceID string) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM limit_rules WHERE (scope_kind = ? AND scope_id = ?) OR org_key = ?
		OR (scope_kind = ? AND scope_id IN (SELECT id FROM teams WHERE space_id = ?))`,
		ScopeTenant, spaceID, "space:"+spaceID, ScopeTeam, spaceID).Scan(&n)
	return n, err
}

// CountActiveInvitesOfSpace counts the invites of a space that still let someone in at nowMs: the ones ActiveInvites
// lists, which the invitations screen shows and can revoke. A used-up invite opens nothing and keeps no tenant.
func (s Store) CountActiveInvitesOfSpace(ctx context.Context, spaceID string, nowMs int64) (int, error) {
	var n int
	err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM invites WHERE space_id = ? AND `+activeInvite, spaceID, nowMs).Scan(&n)
	return n, err
}

// DeleteSpace removes a space row. What hangs on it by a foreign key (members, invites, codes, the ssh key) goes with
// it; the caller has checked the rest is empty.
func (s Store) DeleteSpace(ctx context.Context, id string) error {
	_, err := s.q.Exec(ctx, `DELETE FROM spaces WHERE id = ?`, id)
	return err
}
