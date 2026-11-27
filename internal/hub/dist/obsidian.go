package dist

import (
	"bytes"
	"net/http"
	"time"

	obsidianplugin "github.com/ly-solutions/harnessly/clients/obsidian"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
)

// built stands for the plugin files' time: they are built into the hub.
var built = time.Now()

// obsidian hands out the harnessly plugin of Obsidian (clients/obsidian): its three files, which the plugin also reads
// to update itself, and harnessly-obsidian.zip with them in a harnessly/ folder for <vault>/.obsidian/plugins.
func (h *Handler) obsidian(w http.ResponseWriter, r *http.Request) error {
	name := r.PathValue("file")
	var raw []byte
	kind := ""
	switch name {
	case "harnessly-obsidian.zip":
		raw, kind = obsidianplugin.Zip(), "application/zip"
		w.Header().Set("Content-Disposition", `attachment; filename="harnessly-obsidian.zip"`)
	case "main.js":
		raw, _ = obsidianplugin.File(name)
		kind = "text/javascript; charset=utf-8"
	case "styles.css":
		raw, _ = obsidianplugin.File(name)
		kind = "text/css; charset=utf-8"
	case "manifest.json":
		raw, _ = obsidianplugin.File(name)
		kind = "application/json"
	default:
		return httpx.ErrNotFound
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, built, bytes.NewReader(raw))
	return nil
}
