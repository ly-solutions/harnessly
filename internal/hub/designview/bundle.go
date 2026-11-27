package designview

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Bundle writes a project at a revision of the store as the render worker lays it out to check or export it while the
// machine is off: a gzipped tar of library/<project>/… and of the project's kit under kits/<kit>/….
func (v *View) Bundle(ctx context.Context, spaceID, project string, rev int64, w io.Writer) error {
	s, err := v.snapshot(ctx, spaceID, project, rev)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := tarDir(ctx, tw, filepath.Join(s.base, project), "library/"+project); err != nil {
		return err
	}
	if kits := v.projectKits(ctx, spaceID, s.base, project); kits != "" {
		if err := tarDir(ctx, tw, kits, "kits"); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// tarDir puts the regular files under dir into tw under prefix.
func tarDir(ctx context.Context, tw *tar.Writer, dir, prefix string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: prefix + "/" + filepath.ToSlash(rel), Mode: 0o600, Size: st.Size(),
			ModTime: st.ModTime(), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	})
}
