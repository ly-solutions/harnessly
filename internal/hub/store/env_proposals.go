package store

import (
	"context"
	"database/sql"
	"time"
)

// The states of an EnvProposal.
const (
	EnvProposalWaiting  = "waiting"
	EnvProposalApplied  = "applied"
	EnvProposalDeclined = "declined"
)

// EnvProposal is a change of an organization's environment an agent proposed from a machine: a model provider found by
// its name, added, changed or (Remove) taken out. It changes nothing until an administrator of the organization applies
// it. Source names secrets and holds no value of one.
type EnvProposal struct {
	ID          string
	OrgID       string
	MachineID   string
	MachineName string
	// OwnerID is the account that connected the machine; "" when it is gone.
	OwnerID   string
	Conv      string
	ConvTitle string
	Name      string
	Hint      string
	Remove    bool
	Source    string
	State     string
	CreatedAt time.Time
	ExpiresAt time.Time
	DecidedAt *time.Time
	DecidedBy string
}

const envProposalCols = `id, org_id, machine_id, machine_name, owner_id, conv, conv_title, name, hint, remove, source, state, created_at, expires_at, decided_at, decided_by`

func scanEnvProposal(row interface{ Scan(...any) error }) (EnvProposal, error) {
	var p EnvProposal
	var remove int
	var created, expires int64
	var decided sql.NullInt64
	var owner, by sql.NullString
	if err := row.Scan(&p.ID, &p.OrgID, &p.MachineID, &p.MachineName, &owner, &p.Conv, &p.ConvTitle, &p.Name, &p.Hint, &remove, &p.Source, &p.State,
		&created, &expires, &decided, &by); err != nil {
		return EnvProposal{}, err
	}
	p.OwnerID, p.DecidedBy, p.Remove = owner.String, by.String, remove != 0
	p.CreatedAt, p.ExpiresAt = fromMs(created), fromMs(expires)
	if decided.Valid {
		at := fromMs(decided.Int64)
		p.DecidedAt = &at
	}
	return p, nil
}

// FileEnvProposal keeps a waiting proposal; p.ID is made when empty. It replaces the waiting one the same machine filed
// for the same name, and drops the organization's waiting ones whose term ended before now.
func (s Store) FileEnvProposal(ctx context.Context, p EnvProposal, now time.Time) (EnvProposal, error) {
	if p.ID == "" {
		p.ID = NewID()
	}
	p.State = EnvProposalWaiting
	if _, err := s.q.Exec(ctx, `DELETE FROM env_proposals WHERE org_id = ? AND state = ? AND (expires_at <= ? OR (machine_id = ? AND name = ?))`,
		p.OrgID, EnvProposalWaiting, Ms(now), p.MachineID, p.Name); err != nil {
		return EnvProposal{}, err
	}
	_, err := s.q.Exec(ctx, `INSERT INTO env_proposals (`+envProposalCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
		p.ID, p.OrgID, p.MachineID, p.MachineName, nullStr(p.OwnerID), p.Conv, p.ConvTitle, p.Name, p.Hint, envInt(p.Remove), p.Source, p.State,
		Ms(p.CreatedAt), Ms(p.ExpiresAt))
	return p, err
}

// EnvProposalByID is one proposal, or ErrNotFound.
func (s Store) EnvProposalByID(ctx context.Context, id string) (EnvProposal, error) {
	return scanEnvProposal(s.q.QueryRow(ctx, `SELECT `+envProposalCols+` FROM env_proposals WHERE id = ?`, id))
}

// WaitingEnvProposals are the proposals of an organization that wait and whose term has not ended, oldest first.
func (s Store) WaitingEnvProposals(ctx context.Context, orgID string, now time.Time) ([]EnvProposal, error) {
	rows, err := s.q.Query(ctx, `SELECT `+envProposalCols+` FROM env_proposals WHERE org_id = ? AND state = ? AND expires_at > ? ORDER BY created_at, id`,
		orgID, EnvProposalWaiting, Ms(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EnvProposal{}
	for rows.Next() {
		p, err := scanEnvProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DecideEnvProposal moves a waiting proposal whose term has not ended to state. False when it no longer waits: decided
// by someone else, or its term ended.
func (s Store) DecideEnvProposal(ctx context.Context, id, state, by string, now time.Time) (bool, error) {
	res, err := s.q.Exec(ctx, `UPDATE env_proposals SET state = ?, decided_at = ?, decided_by = ? WHERE id = ? AND state = ? AND expires_at > ?`,
		state, Ms(now), nullStr(by), id, EnvProposalWaiting, Ms(now))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReopenEnvProposal puts a proposal back among the waiting ones: the change it was taken out for was not made.
func (s Store) ReopenEnvProposal(ctx context.Context, id string) error {
	_, err := s.q.Exec(ctx, `UPDATE env_proposals SET state = ?, decided_at = NULL, decided_by = NULL WHERE id = ?`, EnvProposalWaiting, id)
	return err
}
