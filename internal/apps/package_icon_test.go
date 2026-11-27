package apps

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageIcon(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "harnessly"), 0700); err != nil {
		t.Fatal(err)
	}
	p := Package{Dir: dir}
	if value, err := p.readIcon(); err != nil || value != "" {
		t.Fatalf("missing icon: %q %v", value, err)
	}
	for _, width := range []int{32, 129} {
		var buf bytes.Buffer
		if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, 32))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, appIconPath), buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		value, err := p.readIcon()
		if width > 128 {
			if err == nil {
				t.Fatal("oversize icon accepted")
			}
			continue
		}
		if want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()); err != nil || value != want {
			t.Fatalf("valid icon: %q %v", value, err)
		}
	}
}
