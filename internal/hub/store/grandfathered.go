package store

import "context"

// What an installation had when a stricter rule came, written down by the migration that brought the rule: it goes on as
// before, and the rule is asked only of what came later.

// AutomationOperator says whether the person owned an automation rule when operators' rights came.
func (s Store) AutomationOperator(ctx context.Context, userID string) (bool, error) {
	var n int
	err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM automation_operators WHERE user_id = ?`, userID).Scan(&n)
	return n > 0, err
}

// SafeRefTenant says of the tenants that were there when reference folders became per organization: listed,
// whether one of keys is among them; any, whether there were tenants at all (false on a new installation). A key is an
// organization's id, or "space:<id>" of a space that was outside any.
func (s Store) SafeRefTenant(ctx context.Context, keys ...string) (listed, any bool, err error) {
	var n int
	if err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM safe_ref_tenants`).Scan(&n); err != nil || n == 0 {
		return false, false, err
	}
	for _, k := range keys {
		var m int
		if err := s.q.QueryRow(ctx, `SELECT COUNT(*) FROM safe_ref_tenants WHERE tenant = ?`, k).Scan(&m); err != nil {
			return false, true, err
		}
		if m > 0 {
			return true, true, nil
		}
	}
	return false, true, nil
}
