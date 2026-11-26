package storehub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// spaceMachines lists a space's machines, to tell whether a machine's name is its alone.
type spaceMachines interface {
	MachinesBySpace(ctx context.Context, spaceID string) ([]store.Machine, error)
}

// shelves rules the layout files (.folders/<person>.json) of a design room for one machine. It has those of the people
// it is for (Design.ShelfKeys). Anyone else's it keeps only while the latest revision is its own, which is what a
// shared machine writes for a person who works on it without being named; a file that does not exist yet may be
// started by any machine of the space.
func (s *Store) shelves(ctx context.Context, m store.Machine, room string) (may func(p string, write bool) bool, scope string, err error) {
	keys, err := s.design.ShelfKeys(ctx, m)
	if err != nil {
		return nil, "", err
	}
	own := make(map[string]bool, len(keys))
	for _, k := range keys {
		own[filestore.FoldersDir+"/"+k+".json"] = true
	}
	sum := sha256.Sum256([]byte(strings.Join(keys, ",")))
	// A revision names its writer, not its machine's id: a name two machines of the space share proves nothing.
	unique := false
	if db, ok := s.db.(spaceMachines); ok {
		all, err := db.MachinesBySpace(ctx, m.SpaceID)
		if err != nil {
			return nil, "", err
		}
		unique = true
		for _, other := range all {
			if other.ID != m.ID && memberName(other) == memberName(m) {
				unique = false
			}
		}
	}
	return func(p string, write bool) bool {
		if own[p] {
			return true
		}
		rs, err := s.srv.Room(ctx, room)
		if err != nil {
			return false
		}
		e, err := rs.Current(ctx, p)
		if err != nil {
			return false
		}
		if e == nil {
			return write
		}
		return unique && e.Machine == memberName(m)
	}, "/shelves:" + hex.EncodeToString(sum[:6]), nil
}
