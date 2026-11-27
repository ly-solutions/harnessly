package designaccess

import (
	"context"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// HRN-1790, stage 2. The apps' checks that may keep a project from being shared run on the owner's machine, and a
// machine that was off let every link through unchecked. The machine now runs them a moment after each change, at the
// revision of the store its copy is at, and writes the verdict into the project's record (cmd/runtime/share_verdicts.go).
// A hub whose machine is off reads that verdict instead: a verdict of the project's newest revision decides; without one
// the link waits for the machine. HRN-1795: for everyone, the switch hub-design-share-verdict is gone.

// RecordReader reads an entity's record from the space's collections (storehub.Store.Record).
type RecordReader func(ctx context.Context, spaceID, id string) (filestore.Record, bool, error)

// UseVerdicts makes a machine that is off answer by the verdict in the project's record. It is set once, before the hub
// serves.
func (s *Service) UseVerdicts(records RecordReader) { s.records = records }

// verdictsOn: the hub reads the stored verdict for a machine that is off.
func (s *Service) verdictsOn(m store.Machine) bool { return s.records != nil && s.storeServes(m) }

// storedVerdict decides a link by the verdict the machine left for the project's newest revision: nil may be shared,
// blocked() may not, unchecked() is a revision no verdict speaks for.
func (s *Service) storedVerdict(ctx context.Context, m store.Machine, project string) error {
	head, err := s.view.Head(ctx, m.SpaceID, project)
	if err != nil {
		s.log.Warn("design: the store did not answer a share verdict", "project", project, "error", err)
		return unchecked()
	}
	rec, ok, err := s.records(ctx, m.SpaceID, project)
	if err != nil {
		s.log.Warn("design: the project's record did not open for a share verdict", "project", project, "error", err)
		return unchecked()
	}
	if !ok || rec.Share == nil || head <= 0 || rec.Share.Rev != head {
		return unchecked()
	}
	if rec.Share.Blocked {
		return blocked()
	}
	return nil
}

// shareUnchecked is the code of a link refused because nobody ran the apps' checks on the project as it is now.
const shareUnchecked = "share_unchecked"

func unchecked() *Error {
	return fail(shareUnchecked, "Машина владельца выключена, а последнюю версию проекта проверки приложений ещё не смотрели. "+
		"Включите машину: она проверит проект, и ссылку можно будет создать, даже когда машина снова выключится")
}
