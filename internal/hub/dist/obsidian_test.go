package dist

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestObsidianPlugin(t *testing.T) {
	srv, _ := newServer(t, nil)
	res, body := get(t, srv.URL+"/dl/obsidian/manifest.json")
	var m struct{ ID, Version string }
	if res.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &m) != nil || m.ID != "harnessly" || m.Version == "" {
		t.Fatalf("manifest: %d %q", res.StatusCode, body)
	}
	if res, body := get(t, srv.URL+"/dl/obsidian/main.js"); res.StatusCode != http.StatusOK || !strings.Contains(body, "registerObsidianProtocolHandler") {
		t.Fatalf("main.js: %d", res.StatusCode)
	}
	res, body = get(t, srv.URL+"/dl/obsidian/harnessly-obsidian.zip")
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("zip: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	z, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "harnessly/main.js,harnessly/styles.css,harnessly/manifest.json" {
		t.Fatalf("zip files: %v", names)
	}
	for _, p := range []string{"/dl/obsidian/data.json", "/dl/obsidian/..%2fdist.go", "/dl/obsidian/embed.go"} {
		if res, _ := get(t, srv.URL+p); res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %d", p, res.StatusCode)
		}
	}
}
