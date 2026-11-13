package design

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// A snapshot of a project is read without its machine: the hub answers the same public route a guest link
// has. A board asks for the motion kit and the video runtime at /__lib/..., the machine's library server's addresses,
// which are nothing on the hub. The public route therefore serves them from the library's own build, and a board of the
// snapshot is rewritten to ask for them next to its files, under a version: the answer is then immutable.

const (
	libVersionParam = "v"
	libImmutable    = "public, max-age=31536000, immutable"
	libRevalidate   = "public, max-age=300"
)

// libFileVersion is the version of a file the library serves from its build: the start of its hash.
func (l *Library) libFileVersion(name string) string {
	switch {
	case name == "video-runtime.js":
		sum := sha256.Sum256([]byte(l.text("video-runtime.js")))
		return hex.EncodeToString(sum[:6])
	case strings.HasPrefix(name, "motion/"):
		f := motionKit()[strings.TrimPrefix(name, "motion/")]
		if f == nil {
			return ""
		}
		return strings.Trim(f.etag, `"`)[:12]
	}
	return ""
}

// libAddress matches the library's addresses in a board: "/__lib/motion/<file>" and "/__lib/video-runtime.js" in
// an attribute or a CSS url().
var libAddress = regexp.MustCompile(`(["'(])/__lib/(motion/[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*|video-runtime\.js)`)

// libAddressesAt points a board's motion kit and runtime addresses at the snapshot's own, relative to the board: prefix
// is the climb from the board's folder to the project's root.
func (l *Library) libAddressesAt(source, prefix string) string {
	return libAddress.ReplaceAllStringFunc(source, func(m string) string {
		sub := libAddress.FindStringSubmatch(m)
		return sub[1] + prefix + PublicLibDir + "/" + sub[2] + l.libVersionQuery(sub[2])
	})
}

// libVersionQuery is "?v=<version>" of a file of the library by its name, "" for a file the build does not hold.
func (l *Library) libVersionQuery(name string) string {
	v := l.libFileVersion(name)
	if v == "" {
		return ""
	}
	return "?" + libVersionParam + "=" + v
}

// withVideoRuntime puts a video scene's runtime at the head of a board, before the board's own scripts.
func (l *Library) withVideoRuntime(source, prefix string) string {
	tag := `<script src="` + prefix + PublicLibDir + `/video-runtime.js` + l.libVersionQuery("video-runtime.js") + `"></script>`
	if m := headTag.FindString(source); m != "" {
		return strings.Replace(source, m, m+tag, 1)
	}
	return tag + source
}

// publicLib answers __lib/<name> of the public route: the motion kit and the video runtime.
func (a *API) publicLib(w http.ResponseWriter, r *http.Request, rest []string, head bool) {
	name := strings.Join(rest, "/")
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	switch {
	case name == "video-runtime.js":
		v := a.L.libFileVersion(name)
		cache := libRevalidate
		if got, ok := search(r.URL.Query(), libVersionParam); ok && got == v {
			cache = libImmutable
		}
		send(w, 200, "text/javascript; charset=utf-8", []byte(guestSceneRuntime(a.L.text("video-runtime.js"))), head, "Cache-Control", cache)
	case strings.HasPrefix(name, "motion/") && path.Clean(name) == name:
		a.L.serveMotionKit(w, r, "/"+PublicLibDir+"/"+name, head)
	default:
		publicText(w, 404, "Not found")
	}
}

// isRecordClip tells a recording's clip in a project (records/<name>.mp4) from its light copy.
func isRecordClip(segments []string) bool {
	if len(segments) < 2 || segments[0] != RecordsDir {
		return false
	}
	name := segments[len(segments)-1]
	return strings.HasSuffix(strings.ToLower(name), ".mp4") && !strings.HasSuffix(strings.ToLower(name), viewCopySuffix)
}

// viewCopySegments is the path of a clip's light copy: records/<name>.view.mp4.
func viewCopySegments(segments []string) []string {
	out := append([]string{}, segments...)
	name := out[len(out)-1]
	out[len(out)-1] = name[:len(name)-len(".mp4")] + viewCopySuffix
	return out
}

// viewSegments are the segments a request is answered from: a recording's light copy when the project has one (the
// view needs the picture, not the final render's resolution), otherwise the file itself.
func viewSegments(a *API, lib *Library, project string, segments []string) []string {
	if !isRecordClip(segments) {
		return segments
	}
	light := viewCopySegments(segments)
	if f, _, _, ok := a.openPublic(lib, project, light); ok {
		f.Close()
		return light
	}
	return segments
}
