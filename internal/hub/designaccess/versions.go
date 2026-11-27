package designaccess

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/storerev"
)

// The versions panel of a project from the hub's store, with the owner's machine off: the list of versions,
// a project read at one of them, what changed since it, and the restore. The answers have the shape of the runtime's
// own (design/project/versions), so the panel takes either. Whoever may read f/ may read these; a restore is an edit.

// versionAt reads a version of the store: its id (storerev.ID) or the revision number.
func versionAt(raw string, head int64) (int64, bool) {
	at, ok := storerev.Of(raw)
	if !ok {
		n, err := strconv.ParseInt(raw, 10, 64)
		at, ok = n, err == nil && n > 0
	}
	return at, ok && at <= head
}

func (s *Service) storeVersions(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine, project, section, file string,
	head int64, f folded, foreign, keyed bool, card string) error {
	vs, ok := s.view.(Versioner)
	if !ok {
		return fail("not_found", "Версии из хранилища недоступны")
	}
	ctx := r.Context()
	switch section {
	case "versions":
		if r.Method != http.MethodGet || file != "" {
			return fail("bad_request", "Метод не поддерживается")
		}
		list, err := vs.Versions(ctx, m.SpaceID, project, 100)
		if err != nil {
			return err
		}
		points := make([]map[string]any, 0, len(list))
		for _, v := range list {
			who := v.Author
			if who == "" {
				who = v.Machine
			}
			subject := v.Files[0].Path
			if n := len(v.Files); n > 1 {
				subject += " +" + strconv.Itoa(n-1)
			}
			points = append(points, map[string]any{"id": storerev.ID(v.Rev), "rev": v.Rev, "at": v.At, "who": who, "agent": false,
				"auto": v.Author == "", "subject": subject, "files": v.Files})
		}
		writeJSON(w, http.StatusOK, map[string]any{"points": points, "named": []any{}, "unsaved": false, "head": head, "version": storerev.ID(head)})
		return nil
	case "v":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			return fail("bad_request", "Метод не поддерживается")
		}
		ver, rest, _ := strings.Cut(file, "/")
		at, ok := versionAt(ver, head)
		if !ok {
			return fail("not_found", "Версии нет")
		}
		kind, path, _ := strings.Cut(rest, "/")
		switch kind {
		case "index.json", "diff":
			if path != "" {
				return fail("not_found", "Маршрут не найден")
			}
			if kind == "index.json" {
				return s.view.Serve(isolated(w, r), r, m.SpaceID, project, at, nil, nil, true)
			}
			changes, other, err := vs.Compare(ctx, m.SpaceID, project, at)
			if err != nil {
				return err
			}
			boards := make([]map[string]string, 0, len(changes))
			for _, c := range changes {
				boards = append(boards, map[string]string{"path": c.Path, "kind": c.Kind, "board": c.Board})
			}
			writeJSON(w, http.StatusOK, map[string]any{"revision": storerev.ID(at), "boards": boards, "other": other, "kitThen": nil, "kitNow": nil})
			return nil
		case "f":
			segs, ok := segments(path)
			if !ok || !servedSegment(segs[0]) {
				return fail("not_found", "Файл не найден")
			}
			// A page opened by its plain address goes to its keyed one, so the files it loads reach the hub.
			if !keyed && s.files != nil && storeNavigation(r) {
				h := w.Header()
				loc := keyedLocation(segs, s.files.mintCard(u.ID, project, foreignCard(foreign, card), s.now()), withoutCard(r.URL.Query(), r.URL.RawQuery))
				// Two levels more than a board's: v/<version>/ stand between the page and the project.
				h.Set("Location", "../../"+strings.Replace(loc, "/f/", "/v/"+ver+"/f/", 1))
				h.Set("Cache-Control", "no-store")
				h.Set("Referrer-Policy", "strict-origin")
				w.WriteHeader(http.StatusFound)
				return nil
			}
			return s.view.Serve(isolated(w, r), r, m.SpaceID, project, at, segs, nil, true)
		}
		return fail("not_found", "Маршрут не найден")
	}
	// restore
	if r.Method != http.MethodPost || file != "" {
		return fail("bad_request", "Метод не поддерживается")
	}
	if foreign {
		return fail("forbidden", "Проект другого пространства открыт только для просмотра")
	}
	if rank(f.Role) < rank(roleEdit) {
		return fail("forbidden", "Вернуть версию может только редактор")
	}
	var body struct {
		Version string `json:"version"`
		Base    int64  `json:"base"`
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<10))
	if err != nil || json.Unmarshal(raw, &body) != nil || body.Base < 0 {
		return fail("bad_request", "Нужно тело {version}")
	}
	at, ok := versionAt(body.Version, head)
	if !ok {
		return fail("not_found", "Версии нет")
	}
	out, err := vs.Restore(ctx, m.SpaceID, project, at, body.Base, normEmail(u.Email))
	switch {
	case errors.Is(err, ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "Проект уже изменили: откройте версии заново", "code": "conflict", "rev": out.Rev})
		return nil
	case errors.Is(err, ErrReadOnly):
		return fail("unavailable", "Хранилище сейчас только для чтения")
	case err != nil:
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": storerev.ID(out.Head), "rev": out.Rev, "version": out.Head})
	return nil
}
