package storehub

import (
	"context"
	"io"
	"sort"
	"strings"

	"github.com/ly-solutions/harnessly/internal/filestore"
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
