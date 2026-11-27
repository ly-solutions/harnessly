package resaccess

import (
	"fmt"
	"testing"
	"time"
)

// TestRoleActionMatrix is the table «role × action» of the target architecture's board (section «Поделиться»,
// HRN-1791), cell by cell: a change of who may do what is a change of this table first.
func TestRoleActionMatrix(t *testing.T) {
	const y, n = true, false
	//                          view comment edit share general transfer
	rows := []struct {
		role            string
		editorsCanShare bool
		want            [6]bool
	}{
		{"", false, [6]bool{n, n, n, n, n, n}},
		{roleView, false, [6]bool{y, n, n, n, n, n}},
		{roleView, true, [6]bool{y, n, n, n, n, n}},
		{roleComment, false, [6]bool{y, y, n, n, n, n}},
		{roleComment, true, [6]bool{y, y, n, n, n, n}},
		{roleEdit, false, [6]bool{y, y, y, n, n, n}},
		{roleEdit, true, [6]bool{y, y, y, y, n, n}}, // «если разрешил владелец»: shares, but not «anyone with the link»
		{roleManage, false, [6]bool{y, y, y, y, y, n}},
		{roleOwner, false, [6]bool{y, y, y, y, y, y}},
	}
	for _, r := range rows {
		got := CanAll(r.role, r.editorsCanShare)
		for i, a := range Actions {
			if got[a] != r.want[i] || Can(r.role, r.editorsCanShare, a) != r.want[i] {
				t.Errorf("role %q editorsCanShare=%v action %s: got %v, want %v", r.role, r.editorsCanShare, a, got[a], r.want[i])
			}
		}
	}
	if Can(roleOwner, true, "fly") {
		t.Error("an unknown action is never allowed")
	}
}

// TestWhoSharesEdits: over every shape of the fold (own grant, home team level, team grant with or without «may share»,
// lead, admin, kit or not, editors may share or not) whoever runs the access may change the object — the bug of the
// epic was a steward with «Поделиться» and «Редактировать» closed.
func TestWhoSharesEdits(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	roles := []string{"", roleView, roleComment, roleEdit, roleManage}
	levels := []string{levelNone, roleView, roleComment, roleEdit}
	for _, kind := range []string{KindDoc, KindDesign, KindApp, KindKit} {
		for _, own := range roles {
			for _, level := range levels {
				for _, grant := range []string{"", roleView, roleEdit} {
					for _, canShare := range []bool{false, true} {
						for _, ecs := range []bool{false, true} {
							for _, place := range []string{"member", "lead", "admin", "outsider"} {
								r := resource{Ref: Ref{Kind: kind, ID: "x"}, Owner: "owner@x.io", HomeTeam: "t1", TeamLevel: level, EditorsCanShare: ecs}
								p := personTeams{email: "p@x.io", teams: map[string]teamPlace{}, admin: map[string]bool{}}
								switch place {
								case "member":
									p.teams["t1"] = teamPlace{org: "o1"}
								case "lead":
									p.teams["t1"] = teamPlace{lead: true, org: "o1"}
								case "admin":
									p.admin["o1"] = true
								}
								var m *member
								if own != "" {
									m = &member{Email: p.email, Role: own}
								}
								var grants []teamGrant
								if grant != "" {
									p.teams["t2"] = teamPlace{org: "o1"}
									grants = []teamGrant{{Team: "t2", Role: grant, CanShare: canShare && grant == roleEdit}}
								}
								f := fold(r, m, grants, p, "o1", now)
								name := fmt.Sprintf("%s own=%q level=%s grant=%q/%v ecs=%v %s", kind, own, level, grant, canShare, ecs, place)
								if f.CanManage && !Can(f.Role, ecs, ActEdit) {
									t.Errorf("%s: may share (role %q) but not edit", name, f.Role)
								}
								if f.CanManage != Can(f.Role, ecs, ActShare) {
									t.Errorf("%s: canManage %v is not can(share) of role %q", name, f.CanManage, f.Role)
								}
								if f.Steward && f.Role != roleManage {
									t.Errorf("%s: a steward is a manager, got %q", name, f.Role)
								}
							}
						}
					}
				}
			}
		}
	}
}
