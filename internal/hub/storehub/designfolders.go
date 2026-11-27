package storehub

import (
	"context"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// DesignFolder is a project folder of a space's design library as the store holds it now (GitHub #83): what the
// registration of the projects the hub has no row for starts from.
type DesignFolder struct {
	ID string
	// OwnerFile is the folder's .owner.json; nil without one.
	OwnerFile []byte
	// Member is the store member (a machine's name, else its id) that wrote the oldest file still in the folder.
	Member string
}

// DesignSpaces are the spaces whose design library has a room.
func (s *Store) DesignSpaces(ctx context.Context) ([]string, error) {
	return s.spacesOf(ctx, filestore.KindDesign)
}

// allMachines is the part of the hub's tables that lists a space's machines, the revoked ones too.
type allMachines interface {
	AllMachinesBySpace(ctx context.Context, spaceID string) ([]store.Machine, error)
}

// WrittenByOthers says whether a project's folder, in any letter case, holds a revision that no machine of m's person
// wrote.
func (s *Store) WrittenByOthers(ctx context.Context, m store.Machine, project string) (bool, error) {
	names, err := s.ownNames(ctx, m)
	if err != nil {
		return false, err
	}
	rs, err := s.DesignRoom(ctx, m.SpaceID)
	if err != nil {
		return false, err
	}
	return rs.WrittenByOthers(ctx, project+"/", names)
}

// reservedMembers are the names the hub writes into a room under: no machine proves anything by bearing one.
var reservedMembers = map[string]bool{"hub": true} // designview.HubMachine and the apps view

// ownNames are the names under which the machines of m's person write. The store knows a writer by its machine's name,
// so a name that another person's machine of the space bears too proves nothing and counts as another's.
func (s *Store) ownNames(ctx context.Context, m store.Machine) ([]string, error) {
	machines := []store.Machine{m}
	if db, ok := s.db.(allMachines); ok {
		var err error
		if machines, err = db.AllMachinesBySpace(ctx, m.SpaceID); err != nil {
			return nil, err
		}
	} else if db, ok := s.db.(spaceMachines); ok {
		all, err := db.MachinesBySpace(ctx, m.SpaceID)
		if err != nil {
			return nil, err
		}
		machines = append(machines, all...)
	}
	own, others := map[string]bool{}, map[string]bool{}
	for _, o := range machines {
		if m.CreatedBy != "" && o.CreatedBy == m.CreatedBy || o.ID == m.ID {
			own[memberName(o)] = true
		} else {
			others[memberName(o)] = true
		}
	}
	var names []string
	for n := range own {
		if !others[n] && !reservedMembers[n] {
			names = append(names, n)
		}
	}
	return names, nil
}

// writers tells, for one request of a machine, what in a room is its person's own by who wrote it.
type writers struct {
	s     *Store
	ctx   context.Context
	m     store.Machine
	room  string
	once  sync.Once
	names []string
	err   error
}

func (s *Store) writers(ctx context.Context, m store.Machine, room string) *writers {
	return &writers{s: s, ctx: ctx, m: m, room: room}
}

func (w *writers) open() (*filestore.Store, error) {
	w.once.Do(func() { w.names, w.err = w.s.ownNames(w.ctx, w.m) })
	if w.err != nil {
		return nil, w.err
	}
	return w.s.srv.Room(w.ctx, w.room)
}

// path says whether a path never existed (free) and whether its latest revision is the person's own. Both are false
// when the room cannot be read: what cannot be told is not given.
func (w *writers) path(p string) (free, own bool) {
	rs, err := w.open()
	if err != nil {
		w.s.log.Warn("store: who wrote a path could not be told", "room", w.room, "err", err)
		return false, false
	}
	e, err := rs.Current(w.ctx, p)
	if err != nil {
		w.s.log.Warn("store: who wrote a path could not be told", "room", w.room, "err", err)
		return false, false
	}
	if e == nil {
		return true, false
	}
	return false, slices.Contains(w.names, e.Machine)
}

// folder says whether every revision under a folder, in any letter case, is the person's own (an empty one is).
func (w *writers) folder(id string) bool {
	rs, err := w.open()
	if err == nil {
		var others bool
		if others, err = rs.WrittenByOthers(w.ctx, id+"/", w.names); err == nil {
			return !others
		}
	}
	w.s.log.Warn("store: who wrote a folder could not be told", "room", w.room, "folder", id, "err", err)
	return false
}

// DesignFolders lists the project folders of a space's design library, in the order of their ids. A folder whose name
// is not a project id (another letter case, a foreign letter) is not one.
func (s *Store) DesignFolders(ctx context.Context, spaceID string) ([]DesignFolder, error) {
	rs, err := s.DesignRoom(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	type seen struct {
		rev      int64
		member   string
		ownerSha string
	}
	folders := map[string]*seen{}
	var since int64
	for {
		ch, err := rs.Changes(ctx, since, 1000)
		if err != nil {
			return nil, err
		}
		for _, e := range ch.Entries {
			id, ok := projectOf(e.Path)
			if e.Deleted || !ok || !projectIDRe.MatchString(id) || !strings.HasPrefix(e.Path, id+"/") {
				continue
			}
			f := folders[id]
			if f == nil {
				f = &seen{rev: e.Rev, member: e.Machine}
				folders[id] = f
			} else if e.Rev < f.rev {
				f.rev, f.member = e.Rev, e.Machine
			}
			if e.Path == id+"/"+filestore.OwnerFile {
				f.ownerSha = e.Sha
			}
		}
		if !ch.More || ch.Rev <= since {
			break
		}
		since = ch.Rev
	}
	out := make([]DesignFolder, 0, len(folders))
	for id, f := range folders {
		d := DesignFolder{ID: id, Member: f.member}
		if f.ownerSha != "" {
			if rc, err := rs.OpenBlob(ctx, f.ownerSha); err == nil {
				d.OwnerFile, _ = io.ReadAll(io.LimitReader(rc, 8<<10))
				rc.Close()
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
