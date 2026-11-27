package designaccess

import (
	"context"
	"errors"
	"strings"
)

// What the hub's view of the store (internal/hub/designview, which implements Viewer) answers with. They live here and
// not there: the hub's packages are imported by the agent's tests, and the view runs the runtime's design code, which
// imports the agent; designaccess must not.

var (
	// ErrConflict is a write made on top of a revision that is not the file's newest one.
	ErrConflict = errors.New("designview: the file changed since its base revision")
	// ErrBadPath is a path outside the project, or one the store does not keep.
	ErrBadPath = errors.New("designview: not a file of the project")
	// ErrReadOnly is a store the hub only reads.
	ErrReadOnly = errors.New("designview: the store is not writable here")
)

// Written is the outcome of a write: the file's revision now, and the project's newest one.
type Written struct {
	Rev, Head int64
}

// EditRoutes are the editor's saves the hub makes itself, by their path under <platform api>/design/; the view runs
// each with the runtime's handler of the same name (designview.EditRoutes, which a test keeps equal to this).
var EditRoutes = map[string]bool{
	"deck": true, "deck/settings": true, "deck/notes": true, "deck/patch": true, "deck/comments": true, "deck/pdf": true,
	"video": true, "video/settings": true, "video/scene": true, "video/overlay": true, "video/comments": true,
	"scene3d/comments": true, "site/comments": true, "site/props": true,
}

// servedSegment: a first segment of a path the public route answers. The names that begin with two underscores are the
// library's own, and only the guest script, the vendor files and the motion kit with the video runtime (HRN-1258:
// what a snapshot needs to play without its machine) are the route's to serve; a board itself may not be named so.
func servedSegment(first string) bool {
	if !strings.HasPrefix(first, "__") {
		return true
	}
	return first == "__guest" || first == "__vendor" || first == "__lib"
}

// Version is one line of a project's history in the store: a run of revisions by one author, named by its newest.
type Version struct {
	Rev     int64
	At      int64 // ms
	Author  string
	Machine string
	Files   []VersionFile
}

// VersionFile is a file a version changed, by its path under the project.
type VersionFile struct{ Path, Kind string }

// Change is a board changed between a version and now (Kind: added, changed, removed).
type Change struct{ Path, Kind, Board string }

// Versioner is the part of a Viewer that answers the versions panel from the store (designview.View; HRN-1797).
type Versioner interface {
	Versions(ctx context.Context, spaceID, project string, limit int) ([]Version, error)
	// Compare lists the boards that differ between revision at and the newest, and how many other files do.
	Compare(ctx context.Context, spaceID, project string, at int64) ([]Change, int, error)
	// Restore commits the files of revision at on top of the newest revision; base (0: any) is the newest one the
	// editor saw. ErrConflict when a write came between.
	Restore(ctx context.Context, spaceID, project string, at, base int64, author string) (Written, error)
}
