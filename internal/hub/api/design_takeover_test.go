package api_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/filesync"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
	"github.com/ly-solutions/harnessly/internal/hubproto"
)

// A project's record in the hub cannot be taken from its owner: not by a member's machine naming the owner in
// an event, not by claiming a folder others filled, and not because the machine that recorded it is gone.

type takeover struct {
	s              *hubtest.Stack
	owner, mallory *hubtest.Person
	space          string
}

func newTakeover(t *testing.T, opts ...hubtest.Option) *takeover {
	t.Helper()
	s := hubtest.New(t, append([]hubtest.Option{hubtest.Env("HUB_ORGS", "on")}, opts...)...)
	r := &takeover{s: s, owner: s.SignIn("owner@example.com"), mallory: s.SignIn("mallory@example.com")}
	r.space, _ = r.owner.NewSpace("Acme")
	r.owner.Join(r.space, r.mallory)
	return r
}

func (r *takeover) machine(t *testing.T, p *hubtest.Person) *hubtest.Agent {
	t.Helper()
	// A member registers their own machine with a code of the space.
	a, _ := designMachine(t, r.s, p, r.space)
	// The store knows a writer by its machine's name; the test agents all register with one.
	r.rename(t, a, "vm-"+a.MachineID)
	return a
}

func (r *takeover) client(t *testing.T, a *hubtest.Agent) interface {
	Put(context.Context, string, []byte, string) (filestore.Entry, error)
} {
	t.Helper()
	l := linkOf(t, r.s, a, filepath.Join(t.TempDir(), "data"))
	if err := l.Ready(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	return l.Client(filestore.KindDesign)
}

func (r *takeover) rename(t *testing.T, a *hubtest.Agent, name string) {
	t.Helper()
	if _, err := r.s.DB.S().Q().Exec(context.Background(), `UPDATE machines SET name = ? WHERE id = ?`, name, a.MachineID); err != nil {
		t.Fatal(err)
	}
}

// reads says whether the machine's store session gives it the file.
func (r *takeover) reads(t *testing.T, a *hubtest.Agent, rel string) bool {
	t.Helper()
	l := linkOf(t, r.s, a, filepath.Join(t.TempDir(), "data"))
	if err := l.Ready(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	c := l.Client(filestore.KindDesign)
	e, err := c.Current(context.Background(), rel)
	if err != nil || e == nil {
		return false
	}
	var b bytes.Buffer
	return c.GetBlob(context.Background(), e.Sha, &b) == nil
}

func (r *takeover) owners(t *testing.T, project string) []string {
	t.Helper()
	rows, err := r.s.DB.S().Q().Query(context.Background(), `SELECT owner_email FROM design_projects WHERE project_id = ? ORDER BY owner_email`, project)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			t.Fatal(err)
		}
		out = append(out, o)
	}
	return out
}

func designEvent(a *hubtest.Agent, kind, project, person string) {
	a.Emit(map[string]any{"event": hubproto.EventDesign, "design": hubproto.DesignEvent{Kind: kind, Project: project, Person: person}})
}

// settle waits until the hub has taken every event the machine sent before: a link's events are handled in order.
func (r *takeover) settle(t *testing.T, a *hubtest.Agent, marker string) {
	t.Helper()
	designEvent(a, "design.created", marker, "")
	r.s.Eventually("the marker "+marker, func() bool { return len(r.owners(t, marker)) == 1 })
}

func TestMachineCannotDeleteAProjectByNamingItsOwner(t *testing.T) {
	r := newTakeover(t)
	mine, theirs := r.machine(t, r.owner), r.machine(t, r.mallory)
	created(mine, "secret", "owner@example.com")
	r.settle(t, mine, "m1")

	designEvent(theirs, "design.deleted", "secret", "owner@example.com")
	designEvent(theirs, "design.created", "secret", "mallory@example.com")
	r.settle(t, theirs, "m2")
	if got := r.owners(t, "secret"); len(got) != 1 || got[0] != "owner@example.com" {
		t.Fatalf("owners after a stranger's delete and create: %v", got)
	}

	// The owner deletes it from another machine of theirs, as the runtime reports it: with no person.
	second := r.machine(t, r.owner)
	designEvent(second, "design.deleted", "secret", "")
	r.settle(t, second, "m3")
	if got := r.owners(t, "secret"); len(got) != 0 {
		t.Fatalf("owners after the owner's delete: %v", got)
	}
}

func TestMachineEventCannotTakeAFolderOthersFilled(t *testing.T) {
	r := newTakeover(t, hubtest.Env("HUB_DESIGN_UNRECORDED", "closed"))
	mine, theirs := r.machine(t, r.owner), r.machine(t, r.mallory)
	c := r.client(t, mine)
	for _, p := range []string{"old/project/Main.dc.html", "claimed/project/Main.dc.html", "own/project/Main.dc.html"} {
		if _, err := c.Put(context.Background(), p, []byte("<h1>"+p+"</h1>"), "owner@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	// Nobody adds a file of their own to a folder others filled.
	if _, err := r.client(t, theirs).Put(context.Background(), "old/planted.txt", []byte("x"), "mallory@example.com"); !errors.Is(err, filesync.ErrRefused) {
		t.Fatalf("a planted file: %v", err)
	}
	designEvent(theirs, "design.created", "old", "mallory@example.com")
	designEvent(theirs, "design.claim", "claimed", "mallory@example.com")
	r.settle(t, theirs, "m1")
	for _, id := range []string{"old", "claimed"} {
		if got := r.owners(t, id); len(got) != 0 {
			t.Fatalf("%s taken: %v", id, got)
		}
		if r.reads(t, theirs, id+"/project/Main.dc.html") {
			t.Fatalf("%s: a stranger's machine reads the hidden project", id)
		}
	}
	// Naming one's machine as the writer's proves nothing.
	twin := r.machine(t, r.mallory)
	r.rename(t, twin, "vm-"+mine.MachineID)
	designEvent(twin, "design.created", "old", "mallory@example.com")
	r.settle(t, twin, "m0")
	if got := r.owners(t, "old"); len(got) != 0 {
		t.Fatalf("old taken by a machine of the same name: %v", got)
	}
	r.rename(t, twin, "vm-twin")
	// The news of a project that was lost is sent again after its files reached the store: from the machine that wrote
	// them, or another one of the same person, it still records the project.
	second := r.machine(t, r.owner)
	designEvent(second, "design.created", "own", "owner@example.com")
	designEvent(second, "design.claim", "old", "owner@example.com")
	r.settle(t, second, "m2")
	for _, id := range []string{"own", "old"} {
		if got := r.owners(t, id); len(got) != 1 || got[0] != "owner@example.com" {
			t.Fatalf("%s: %v", id, got)
		}
	}
	designEvent(mine, "design.created", "claimed", "owner@example.com")
	r.settle(t, mine, "m3")
	if got := r.owners(t, "claimed"); len(got) != 1 || got[0] != "owner@example.com" {
		t.Fatalf("claimed: %v", got)
	}
}

func TestProjectStaysRecordedWhenItsMachineIsRevoked(t *testing.T) {
	for _, mode := range []string{"open", "closed"} {
		t.Run(mode, func(t *testing.T) {
			r := newTakeover(t, hubtest.Env("HUB_DESIGN_UNRECORDED", mode))
			home, theirs := r.machine(t, r.owner), r.machine(t, r.mallory)
			created(home, "secret", "owner@example.com")
			r.settle(t, home, "m1")
			if _, err := r.client(t, home).Put(context.Background(), "secret/project/Main.dc.html", []byte("<h1>secret</h1>"), "owner@example.com"); err != nil {
				t.Fatal(err)
			}
			second := r.machine(t, r.owner)
			if resp := r.owner.Do("DELETE", "/api/machines/"+home.MachineID, nil); resp.Code >= 300 {
				t.Fatalf("revoke: %s", resp)
			}
			if r.reads(t, theirs, "secret/project/Main.dc.html") {
				t.Fatal("a private project opened to the space")
			}
			if !r.reads(t, second, "secret/project/Main.dc.html") {
				t.Fatal("the owner lost the project")
			}
			designEvent(theirs, "design.created", "secret", "mallory@example.com")
			r.settle(t, theirs, "m2")
			if got := r.owners(t, "secret"); len(got) != 1 || got[0] != "owner@example.com" {
				t.Fatalf("owners: %v", got)
			}
			lists := func(p *hubtest.Person, a *hubtest.Agent) bool {
				resp := p.Do("GET", "/m/"+a.MachineID+"/api/platform/design/store-library", nil)
				if resp.Code != 200 {
					t.Fatalf("library: %s", resp)
				}
				var lib struct{ Artifacts []struct{ ID string } }
				resp.JSON(&lib)
				for _, p := range lib.Artifacts {
					if p.ID == "secret" {
						return true
					}
				}
				return false
			}
			if !lists(r.owner, second) {
				t.Fatal("the library does not list the project to its owner")
			}
			// The owner still runs its access, and deleting it frees the record.
			base := "/m/" + second.MachineID + "/api/platform/design/access/secret"
			var v struct{ Role string }
			if resp := r.owner.Do("GET", base, nil); resp.Code != 200 {
				t.Fatalf("access view: %s", resp)
			} else if resp.JSON(&v); v.Role != "owner" {
				t.Fatalf("access view: %s", resp)
			}
			designEvent(second, "design.deleted", "secret", "")
			r.settle(t, second, "m3")
			if got := r.owners(t, "secret"); len(got) != 0 {
				t.Fatalf("owners after delete: %v", got)
			}
		})
	}
}

func TestProjectStaysRecordedWhenItsMachineMoves(t *testing.T) {
	r := newTakeover(t, hubtest.Env("HUB_DESIGN_UNRECORDED", "open"), hubtest.Env("HUB_DESIGN_SHARE_URL", "http://"+shareHost))
	home, theirs := r.machine(t, r.owner), r.machine(t, r.mallory)
	created(home, "secret", "owner@example.com")
	r.settle(t, home, "m1")
	if _, err := r.client(t, home).Put(context.Background(), "secret/project/Main.dc.html", []byte("<h1>secret</h1>"), "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	// A guest link and a private snapshot with its page go through the move with the record.
	if resp := r.owner.Do("POST", "/m/"+home.MachineID+"/api/platform/design/access/secret/shares", map[string]any{"rights": "view"}); resp.Code >= 300 {
		t.Fatalf("share: %s", resp)
	}
	for _, q := range []string{
		`INSERT INTO design_private_snapshots (machine_id, project_id, rev, created_by, created_at, expires_at) VALUES (?, 'secret', '1', 'owner@example.com', 1, 2)`,
		`INSERT INTO design_private_snapshot_pages (machine_id, project_id, position, board, title, jpeg) VALUES (?, 'secret', 0, 'Main', 'Main', 'x')`,
	} {
		if _, err := r.s.DB.S().Q().Exec(context.Background(), q, home.MachineID); err != nil {
			t.Fatal(err)
		}
	}
	other, _ := r.owner.NewSpace("Beta")
	if resp := r.owner.Do("POST", "/api/machines/"+home.MachineID+"/move", map[string]any{"tenantId": other}); resp.Code != 200 {
		t.Fatalf("move: %s", resp)
	}
	if r.reads(t, theirs, "secret/project/Main.dc.html") {
		t.Fatal("a private project opened to the space it stayed in")
	}
	second := r.machine(t, r.owner)
	if !r.reads(t, second, "secret/project/Main.dc.html") {
		t.Fatal("the owner lost the project in its space")
	}
	// The machine takes a record of its own along: what it syncs to the new space is not open there.
	var n int
	for _, q := range []string{
		`SELECT COUNT(*) FROM design_share_links l JOIN machines m ON m.id = l.machine_id JOIN resource_guest_links g ON g.link_id = l.id AND g.id = l.machine_id || '/' || l.project_id AND g.space_id = m.space_id WHERE m.space_id = ?`,
		`SELECT COUNT(*) FROM design_private_snapshot_pages p JOIN machines m ON m.id = p.machine_id WHERE m.space_id = ?`,
	} {
		if err := r.s.DB.S().Q().QueryRow(context.Background(), q, r.space).Scan(&n); err != nil || n != 1 {
			t.Fatalf("left behind: %d %v: %s", n, err, q)
		}
	}
	if err := r.s.DB.S().Q().QueryRow(context.Background(), `SELECT COUNT(*) FROM design_projects p JOIN machines m ON m.id = p.machine_id
		WHERE m.space_id = ? AND m.id = ? AND p.project_id = ? AND p.owner_email = ?`, other, home.MachineID, "secret", "owner@example.com").Scan(&n); err != nil || n != 1 {
		t.Fatalf("the record in the new space: %d %v", n, err)
	}
}
