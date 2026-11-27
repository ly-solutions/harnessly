// Package obsidianplugin carries the harnessly plugin of Obsidian (main.js, styles.css, manifest.json) for the hub to
// hand out at /dl/obsidian/. The files are the plugin itself: plain JavaScript, no build step.
package obsidianplugin

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"sync"
)

//go:embed main.js styles.css manifest.json
var files embed.FS

// Names are the plugin's files.
var Names = []string{"main.js", "styles.css", "manifest.json"}

// File is one file of the plugin; false when it has none of that name.
func File(name string) ([]byte, bool) {
	for _, n := range Names {
		if n == name {
			raw, err := files.ReadFile(name)
			return raw, err == nil
		}
	}
	return nil, false
}

// Version is the plugin's version from its manifest.
func Version() string {
	raw, _ := files.ReadFile("manifest.json")
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Version
}

var (
	zipOnce sync.Once
	zipRaw  []byte
)

// Zip is the plugin as an archive with a harnessly/ folder, to unpack into <vault>/.obsidian/plugins.
func Zip() []byte {
	zipOnce.Do(func() {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		for _, n := range Names {
			raw, _ := files.ReadFile(n)
			f, err := w.Create("harnessly/" + n)
			if err == nil {
				_, _ = f.Write(raw)
			}
		}
		_ = w.Close()
		zipRaw = b.Bytes()
	})
	return zipRaw
}
