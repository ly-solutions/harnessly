package store

import "context"

const designUnrecorded = "design_unrecorded"

// DesignUnrecordedClosed tells whether this installation keeps projects without a row closed when HUB_DESIGN_UNRECORDED
// says nothing. The first call decides for good: closed when nobody has an account yet, open for an installation that
// had people before the choice (its projects without a row are in use).
func (s Store) DesignUnrecordedClosed(ctx context.Context) (bool, error) {
	users, err := s.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	value := "closed"
	if users > 0 {
		value = "open"
	}
	if _, err := s.q.Exec(ctx, `INSERT INTO instance_defaults (name, value) VALUES (?, ?) ON CONFLICT (name) DO NOTHING`, designUnrecorded, value); err != nil {
		return false, err
	}
	err = s.q.QueryRow(ctx, `SELECT value FROM instance_defaults WHERE name = ?`, designUnrecorded).Scan(&value)
	return value == "closed", err
}
