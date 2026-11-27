package designview

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"

	"github.com/ly-solutions/harnessly/internal/design"
	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/designaccess"
)

// The versions of a project from the store: its history is the store's revisions under the project's
// folder, so the panel works while the owner's machine is off.

// versionGap is how far apart, at most, two revisions of one author are to count as one version: a sync or a save of
// several files lands as a run of revisions, and the panel shows it as a line.
const versionGap = 60_000

// versionScan is how many revisions of the folder one listing reads.
const versionScan = 2000

// Versions lists the project's versions newest first, up to limit: runs of revisions by one author within a minute
// are one version, named by its newest revision.
func (v *View) Versions(ctx context.Context, spaceID, project string, limit int) ([]designaccess.Version, error) {
	if !design.IsLibraryID(project) {
		return nil, ErrMissing
	}
	room, err := v.o.Rooms(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	entries, err := room.HistoryUnder(ctx, project+"/", versionScan)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Rev > entries[j].Rev })
	var out []designaccess.Version
	for _, e := range entries {
		rel, ok := strings.CutPrefix(e.Path, project+"/")
		if !ok || rel == "" {
			continue
		}
		kind := "changed"
		if e.Deleted {
			kind = "removed"
		}
		file := designaccess.VersionFile{Path: rel, Kind: kind}
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.Author == e.Author && last.Machine == e.Machine && last.At-e.At <= versionGap && last.At >= e.At {
				last.Files = append(last.Files, file)
				continue
			}
		}
		if len(out) == limit {
			break
		}
		out = append(out, designaccess.Version{Rev: e.Rev, At: e.At, Author: e.Author, Machine: e.Machine, Files: []designaccess.VersionFile{file}})
	}
	return out, nil
}

// then and now are the files of the project at a revision and at its newest one, by path under the project.
func (v *View) then(ctx context.Context, spaceID, project string, at int64) (room Room, then, now map[string]filestore.Entry, err error) {
	if !design.IsLibraryID(project) {
		return nil, nil, nil, ErrMissing
	}
	room, err = v.o.Rooms(ctx, spaceID)
	if err != nil {
		return nil, nil, nil, err
	}
	head, err := v.Head(ctx, spaceID, project)
	if err != nil {
		return nil, nil, nil, err
	}
	if at <= 0 || head == 0 || at > head {
		return nil, nil, nil, ErrMissing
	}
	read := func(rev int64) (map[string]filestore.Entry, error) {
		s, err := room.Snapshot(ctx, project+"/", rev)
		if err != nil {
			return nil, err
		}
		files := make(map[string]filestore.Entry, len(s.Entries))
		for _, e := range s.Entries {
			if rel, ok := strings.CutPrefix(e.Path, project+"/"); ok && rel != "" && path.Clean(rel) == rel {
				files[rel] = e
			}
		}
		return files, nil
	}
	if then, err = read(at); err != nil {
		return nil, nil, nil, err
	}
	if now, err = read(head); err != nil {
		return nil, nil, nil, err
	}
	return room, then, now, nil
}

func sameFile(a, b filestore.Entry) bool {
	if a.Tag != "" || b.Tag != "" {
		return a.Tag == b.Tag
	}
	return a.Sha == b.Sha
}

// Compare is what changed between revision at and the project's newest one: its boards by name, and how many other
// files (kinds are from the version to now).
func (v *View) Compare(ctx context.Context, spaceID, project string, at int64) ([]designaccess.Change, int, error) {
	_, then, now, err := v.then(ctx, spaceID, project, at)
	if err != nil {
		return nil, 0, err
	}
	changes := []designaccess.Change{}
	other := 0
	note := func(rel, kind string) {
		if _, board, ok := design.BoardOfPath(project + "/" + rel); ok {
			changes = append(changes, designaccess.Change{Path: strings.TrimPrefix(rel, design.ProjectDirname+"/"), Kind: kind, Board: board})
			return
		}
		other++
	}
	for rel, n := range now {
		if t, ok := then[rel]; !ok {
			note(rel, "added")
		} else if !sameFile(t, n) {
			note(rel, "changed")
		}
	}
	for rel := range then {
		if _, ok := now[rel]; !ok {
			note(rel, "removed")
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, other, nil
}

// Restore commits the files of revision at as new revisions on top of the project's newest one: each file only when
// it differs, on the revision it has now, so a write that came between ends it with ErrConflict. base, when set, is
// the newest revision the editor saw: a newer one is a conflict at once. The owner file is the project's, not the
// version's, and stays.
func (v *View) Restore(ctx context.Context, spaceID, project string, at, base int64, author string) (designaccess.Written, error) {
	room, then, now, err := v.then(ctx, spaceID, project, at)
	if err != nil {
		return designaccess.Written{}, err
	}
	w, ok := room.(Writer)
	if !ok {
		return designaccess.Written{}, ErrReadOnly
	}
	head, err := v.Head(ctx, spaceID, project)
	if err != nil {
		return designaccess.Written{}, err
	}
	if base > 0 && base != head {
		return designaccess.Written{Rev: head}, ErrConflict
	}
	out := designaccess.Written{}
	commit := func(req filestore.CommitRequest) error {
		e, conflict, err := w.Commit(ctx, req, HubMachine)
		if err != nil {
			return err
		}
		if e != nil {
			out.Rev = e.Rev
		}
		if conflict {
			return ErrConflict
		}
		return nil
	}
	rels := make([]string, 0, len(then)+len(now))
	for rel := range then {
		rels = append(rels, rel)
	}
	for rel := range now {
		if _, ok := then[rel]; !ok {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if strings.EqualFold(rel, filestore.OwnerFile) {
			continue
		}
		t, had := then[rel]
		n, has := now[rel]
		if had && has && sameFile(t, n) {
			continue
		}
		req := filestore.CommitRequest{Path: project + "/" + rel, Base: n.Rev, Author: author}
		if had {
			req.Sha, req.Tag, req.Size, req.Exec = t.Sha, t.Tag, t.Size, t.Exec
		} else {
			req.Deleted = true
		}
		if err := commit(req); err != nil {
			if errors.Is(err, filestore.ErrBadRequest) {
				err = ErrBadPath
			}
			return out, err
		}
	}
	out.Head, _ = v.Head(ctx, spaceID, project)
	return out, nil
}
