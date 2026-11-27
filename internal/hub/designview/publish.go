package designview

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/ly-solutions/harnessly/internal/design"
	"github.com/ly-solutions/harnessly/internal/hub/designaccess"
	"github.com/ly-solutions/harnessly/internal/sitesclient"
)

// HRN-1799 (stage 7). «Опубликовать как сайт» with the machine off: the project at a revision of the store,
// with the kit it is pinned to at the kit's newest revision, is turned into a site's files by the runtime's own code
// (Library.PublishSnapshot), as the machine does it from its folders.

// PublishFiles is what a site made from the project at revision at holds: its title and files. pages narrows the
// boards published, as the runtime takes it (nil is every board).
func (v *View) PublishFiles(ctx context.Context, spaceID, project string, at int64, pages []string) (string, []sitesclient.DeployFile, error) {
	s, err := v.snapshot(ctx, spaceID, project, at)
	if err != nil {
		return "", nil, err
	}
	kitsDir := ""
	if pin, ok := s.api.L.KitPinOf(project); ok && validKit(pin.ID) && v.o.Kits != nil {
		set, err := v.kitSet(ctx, spaceID, []string{pin.ID})
		if err != nil {
			return "", nil, err
		}
		kitsDir = filepath.Join(set.dir, "kits")
	}
	lib := s.api.L
	if kitsDir != "" {
		lib = design.New(design.Config{
			LibraryDir: s.base,
			KitsDir:    kitsDir,
			DataDir:    filepath.Join(v.o.Dir, "data"),
			Passive:    true,
			ShareProbe: func(string) design.ShareFSReport { return design.ShareFSReport{OK: true, At: time.Now()} },
		})
	}
	title, out, err := lib.PublishSnapshot(project, pages)
	if errors.Is(err, design.ErrPublishEmpty) {
		err = designaccess.ErrPublishEmpty
	}
	return title, out, err
}
