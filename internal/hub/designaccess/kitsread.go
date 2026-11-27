package designaccess

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/ly-solutions/harnessly/internal/hub/resaccess"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// HRN-1799 (stage 4). With the machine off, the reads of UI kits come from the space's kits collection: only
// the kits the person may see (the shared model of access to a kit, resaccess.KitView), at their newest revisions. With
// the machine on the machine answers, as before, unless the page asks for the store (?from=store).

var kitReadPath = regexp.MustCompile(`^/api/[A-Za-z0-9_-]+/design/(kit/(?:canvases|canvas|file|fonts|layouts)|kits/state)$`)

// KitReader is a Viewer that answers the reads of kits from the store (designview.View.ServeKits).
type KitReader interface {
	ServeKits(w http.ResponseWriter, r *http.Request, spaceID string, kits []string, route string) error
}

func (s *Service) serveKits(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	match := kitReadPath.FindStringSubmatch(strings.TrimPrefix(r.URL.EscapedPath(), "/m/"+m.ID))
	if match == nil || m.SpaceID == "" || s.kits == nil {
		return false
	}
	kr, ok := s.view.(KitReader)
	if !ok {
		return false
	}
	ctx := r.Context()
	if r.URL.Query().Get("from") != "store" && s.machineUp(m.ID) {
		return false
	}
	if !s.machineOff(ctx, m) {
		return false
	}
	verified, err := s.db.S().VerifiedEmails(ctx, u.ID)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	view, err := s.kits.KitView(ctx, m.SpaceID, resaccess.Actor{UserID: u.ID, Email: normEmail(u.Email), Name: u.Name, Verified: verified})
	if err != nil {
		s.writeError(w, err)
		return true
	}
	kits := make([]string, 0, len(view))
	for id, g := range view {
		if g.See {
			kits = append(kits, strings.TrimPrefix(id, m.SpaceID+"/"))
		}
	}
	// A kit asked for by name that the person may not see is as missing as one nobody made.
	if kit := r.URL.Query().Get("kit"); kit != "" && !contains(kits, kit) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Кит не установлен", "code": "not_found"})
		return true
	}
	w.Header().Set("X-Harnessly-From", "store")
	if err := kr.ServeKits(w, r, m.SpaceID, kits, match[1]); err != nil {
		if errors.Is(err, r.Context().Err()) {
			return true
		}
		s.log.Warn("design kits: not read from the store", "machine", m.ID, "route", match[1], "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Киты сейчас не прочитать: попробуйте ещё раз", "code": "store_unavailable"})
	}
	return true
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
