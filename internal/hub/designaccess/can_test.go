package designaccess

import (
	"fmt"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/resaccess"
)

// TestDesignWhoSharesEdits: the design's fold answers by the one table (resaccess/can.go). Whoever runs a
// project's access may change it, so «Поделиться» never shows the window to one whose «Редактировать» is closed — a
// space's owner (the lead of its implicit team) or an organization's admin looking at a team project.
func TestDesignWhoSharesEdits(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	roles := []string{"", roleView, roleComment, roleEdit, roleManage}
	levels := []string{levelNone, roleView, roleComment, roleEdit}
	for _, own := range roles {
		for _, level := range levels {
			for _, grant := range []string{"", roleView, roleEdit} {
				for _, canShare := range []bool{false, true} {
					for _, ecs := range []bool{false, true} {
						for _, place := range []string{"member", "lead", "admin", "outsider"} {
							p := project{ID: "p1", Owner: "owner@x.io", HomeTeam: "t1", TeamLevel: level, EditorsCanShare: ecs}
							who := personTeams{email: "p@x.io", teams: map[string]teamPlace{}, admin: map[string]bool{}}
							switch place {
							case "member":
								who.teams["t1"] = teamPlace{org: "o1"}
							case "lead":
								who.teams["t1"] = teamPlace{lead: true, org: "o1"}
							case "admin":
								who.admin["o1"] = true
							}
							var m *member
							if own != "" {
								m = &member{Email: who.email, Role: own}
							}
							var grants []teamGrant
							if grant != "" {
								who.teams["t2"] = teamPlace{org: "o1"}
								grants = []teamGrant{{Team: "t2", Role: grant, CanShare: canShare && grant == roleEdit}}
							}
							f := foldAccess(p, m, grants, who, "o1", now)
							name := fmt.Sprintf("own=%q level=%s grant=%q/%v ecs=%v %s", own, level, grant, canShare, ecs, place)
							if f.CanManage && !resaccess.Can(f.Role, ecs, resaccess.ActEdit) {
								t.Errorf("%s: may share (role %q) but not edit", name, f.Role)
							}
							if f.CanManage != resaccess.Can(f.Role, ecs, resaccess.ActShare) {
								t.Errorf("%s: canManage %v is not can(share) of role %q", name, f.CanManage, f.Role)
							}
							steward := level != levelNone && (place == "lead" || place == "admin")
							if f.Steward != steward {
								t.Errorf("%s: steward %v, want %v", name, f.Steward, steward)
							}
							if steward && (f.Role != roleManage || f.Source == "") {
								t.Errorf("%s: a steward is a manager with a source, got %q from %q", name, f.Role, f.Source)
							}
						}
					}
				}
			}
		}
	}
}
