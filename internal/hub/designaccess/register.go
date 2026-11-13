package designaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// GitHub #83. Before the store's library is closed to unrecorded projects (HUB_DESIGN_UNRECORDED=closed), every project
// folder the hub has no row for gets one: its owner file's person when they may open the machine that wrote it, else
// that machine's person, and the team level the hub gives new projects. Then «no row» never silently means «open to
// all», and a dry run says beforehand which projects become personal and which stay the team's.

// Folder is a project folder of a space's design library (storehub.DesignFolder).
type Folder struct {
	ID string
	// OwnerFile is the folder's .owner.json; nil without one.
	OwnerFile []byte
	// Member is the store member — a machine's name, else its id — that wrote the oldest file still in the folder.
	Member string
}

// RegisterReport is what a registration did, or would do in a dry run.
type RegisterReport struct {
	DryRun   bool
	Space    string
	Projects []ProjectRegister
}

type ProjectRegister struct {
	ID, Owner, Machine string
	// Source: «owner file» when the owner file named the owner, «machine» when it is the machine's person.
	Source string
	// Level is the team level the project gets; Team is who that level counts for: the owner's home team, or "" for
	// everyone who may open the machine.
	Level, Team string
	// Outcome: registered (or would be in a dry run), recorded (the hub has a row already), skipped, failed.
	Outcome string
	Reason  string
}

// Personal: nobody but the owner has a role in the project once it is registered.
func (p ProjectRegister) Personal() bool { return p.Level == levelNone }

func (r RegisterReport) count(outcome string) int {
	n := 0
	for _, p := range r.Projects {
		if p.Outcome == outcome {
			n++
		}
	}
	return n
}

// Failed: a project could not be registered; the space is not safe to close yet.
func (r RegisterReport) Failed() bool { return r.count("failed")+r.count("skipped") > 0 }

// Summary is the report for a person: one line per project the hub had no row for, then the totals.
func (r RegisterReport) Summary() string {
	var b strings.Builder
	verb := "registered"
	if r.DryRun {
		verb = "would register"
	}
	personal, team := 0, 0
	for _, p := range r.Projects {
		switch p.Outcome {
		case "recorded":
			continue
		case "registered":
			who := "personal"
			if !p.Personal() {
				who = "team " + p.Level
				if p.Team == "" {
					who += " (everyone who may open the machine)"
				} else {
					who += " (" + p.Team + ")"
				}
				team++
			} else {
				personal++
			}
			fmt.Fprintf(&b, "%s %s: owner %s (from the %s), %s\n", verb, p.ID, p.Owner, p.Source, who)
		default:
			fmt.Fprintf(&b, "%s %s: %s\n", p.Outcome, p.ID, p.Reason)
		}
	}
	fmt.Fprintf(&b, "space %s: %d personal, %d team, %d already recorded, %d skipped, %d failed\n", r.Space, personal, team,
		r.count("recorded"), r.count("skipped"), r.count("failed"))
	return b.String()
}

// RegisterUnrecorded gives each folder of a space's library the hub has no row for an owner and a team level.
func (s *Service) RegisterUnrecorded(ctx context.Context, spaceID string, folders []Folder, dryRun bool) (RegisterReport, error) {
	rep := RegisterReport{DryRun: dryRun, Space: spaceID}
	machines, err := s.db.S().MachinesBySpace(ctx, spaceID)
	if err != nil {
		return rep, err
	}
	byMember := map[string]store.Machine{}
	for _, m := range machines {
		if m.RevokedAt != nil {
			continue
		}
		byMember[m.ID] = m
		if n := strings.TrimSpace(m.Name); n != "" && len(n) <= 80 {
			byMember[n] = m
		}
	}
	for _, f := range folders {
		p := ProjectRegister{ID: f.ID}
		if !projectIDRe.MatchString(f.ID) {
			continue
		}
		m, ok := byMember[f.Member]
		if !ok {
			p.Outcome, p.Reason = "skipped", fmt.Sprintf("the machine that wrote it (%q) is not in the space any more", f.Member)
			rep.Projects = append(rep.Projects, p)
			continue
		}
		p.Machine = m.ID
		if err := s.registerOne(ctx, m, f, &p, dryRun); err != nil {
			p.Outcome, p.Reason = "failed", err.Error()
		}
		rep.Projects = append(rep.Projects, p)
	}
	return rep, nil
}

func (s *Service) registerOne(ctx context.Context, m store.Machine, f Folder, p *ProjectRegister, dryRun bool) error {
	t, err := s.read(ctx, m)
	if err != nil {
		return err
	}
	if row, err := t.getProject(f.ID, false); err != nil {
		return err
	} else if row != nil {
		p.Outcome, p.Owner, p.Level = "recorded", row.Owner, row.TeamLevel
		return nil
	}
	var file struct {
		Person string `json:"person"`
	}
	_ = json.Unmarshal(f.OwnerFile, &file)
	named := normEmail(file.Person)
	owner, err := s.eventPerson(ctx, m, named)
	if err != nil {
		return err
	}
	if owner == "" {
		p.Outcome, p.Reason = "skipped", "neither its owner file nor its machine names a person"
		return nil
	}
	p.Owner, p.Source = owner, "machine"
	if named != "" && named == owner {
		p.Source = "owner file"
	}
	home, err := t.w.homeTeamOf(owner)
	if err != nil {
		return err
	}
	p.Level = s.teamLevel
	if home != "" {
		p.Team = home
		if ref, ok := t.w.team(home); ok && ref.Name != "" {
			p.Team = ref.Name
		}
	}
	if dryRun {
		p.Outcome = "registered"
		return nil
	}
	return s.write(ctx, m, func(t *txn) error {
		out, err := t.insertProject(f.ID, owner, t.w.displayName(owner), s.teamLevel, true)
		if err != nil {
			return err
		}
		switch out {
		case regExists:
			p.Outcome = "recorded"
			return nil
		case regCap:
			p.Outcome, p.Reason = "failed", fmt.Sprintf("the owner has %d projects on the machine already", maxProjectsByOwner)
			return nil
		}
		p.Outcome = "registered"
		return t.writeLog(f.ID, "hub-register", "register", owner, map[string]any{"team": s.teamLevel, "source": p.Source})
	})
}
