package renderworker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func bundleOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o600, Size: 2, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte("hi"))
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}

// A project's copy lays out under the worker's folder only: the project and its kit, nothing else.
func TestUnpackKeepsTheCopyInItsFolder(t *testing.T) {
	dir := t.TempDir()
	if err := unpack(bytes.NewReader(bundleOf(t, "library/p1/index.html", "kits/k/kit.css")), dir, "p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kits", "k", "kit.css")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../x", "library/p1/../../x", "/etc/x", "library/p2/a.html", "other/a"} {
		if err := unpack(bytes.NewReader(bundleOf(t, "library/p1/index.html", bad)), t.TempDir(), "p1"); err == nil {
			t.Fatalf("%q was laid out", bad)
		}
	}
	if err := unpack(bytes.NewReader(bundleOf(t, "kits/k/kit.css")), t.TempDir(), "p1"); err == nil {
		t.Fatal("a copy without the project was taken")
	}
	// The check's folder goes back as a gzipped tar of its files.
	out := filepath.Join(t.TempDir(), "c.tar.gz")
	if err := packDir(filepath.Join(dir, "library", "p1"), out); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(out)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	h, err := tar.NewReader(gz).Next()
	if err != nil || h.Name != "index.html" {
		t.Fatalf("packed: %+v %v", h, err)
	}
}
