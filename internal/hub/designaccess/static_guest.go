package designaccess

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"image/jpeg"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hubproto"
	"github.com/ly-solutions/harnessly/internal/storerev"
)

// StaticPrefix addresses immutable snapshots on the Hub. It never reaches a runtime.
const StaticPrefix = "/guest/design/"

// The standard ingress has a one-hour read timeout. Return a definite publication result before it.
const staticPublicationTimeout = 50 * time.Minute

const maxStaticBoards = 64
const maxStaticPage = 2 << 20
const maxStaticBundle = 16 << 20

func isStaticToken(token string) bool {
	return strings.HasPrefix(token, "snap-") && shareTokenRe.MatchString(strings.TrimPrefix(token, "snap-"))
}

type staticPage struct{ Path, Title, Image string }

func cleanStaticPage(raw []byte) (string, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2400 || cfg.Height > 2400 {
		return "", errRuntime
	}
	pic, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", errRuntime
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, pic, &jpeg.Options{Quality: 90}); err != nil || b.Len() > maxStaticPage {
		return "", errRuntime
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// capture pins the version a snapshot of the pages is drawn at and draws them. With the store and the hub's render
// worker the version is the store's head, the boards are listed from the store at it and the worker draws
// each one there: the owner's machine takes no part and may be off. A hub without a worker asks the owner's machine,
// which draws them from its working copy.
func (s *Service) capture(ctx context.Context, m store.Machine, project string, in shareInput) (string, []staticPage, error) {
	pd, ok := s.render.(PageDrawer)
	if !ok || !s.storeServes(m) || m.CreatedBy == "" {
		rev, err := s.pinCapture(ctx, m, project)
		if err != nil {
			return "", nil, err
		}
		in.Rev = rev
		pages, err := s.captureFromMachine(ctx, m.ID, project, in)
		return rev, pages, err
	}
	rev, err := s.pin(ctx, m, project)
	if err != nil {
		return "", nil, err
	}
	at, _ := storerev.Of(rev)
	in.Rev = rev
	pages, err := s.captureFromStore(ctx, m, project, at, in, pd)
	return rev, pages, err
}

// captureFromMachine asks the owner's machine for the index of the boards and the picture of each.
func (s *Service) captureFromMachine(ctx context.Context, machine, project string, in shareInput) ([]staticPage, error) {
	req := hubproto.DesignPublicTokenRequest{Project: project, Rev: in.Rev, Capture: true, Pages: in.Pages}
	unavailable := fail("unavailable", "Снимок страниц не получился: машина владельца не дорисовала все страницы. Попробуйте ещё раз, а если повторится — выберите меньше страниц")
	get := func(segments []string, limit int64) ([]byte, error) {
		res, err := s.machineFetch(ctx, machine, req, segments, nil)
		if err != nil {
			s.log.Debug("design snapshot: capture request failed", "raster", len(segments) == 2, "error", err)
			return nil, unavailable
		}
		if res.StatusCode != http.StatusOK || len(segments) == 2 && (res.Header.Get("X-Design-Static-Page") != segments[1] || res.Header.Get("Content-Type") != "image/jpeg") {
			res.Body.Close()
			s.log.Debug("design snapshot: unexpected capture response", "raster", len(segments) == 2, "status", res.StatusCode)
			return nil, unavailable
		}
		b, err := readAll(res, limit)
		if err != nil {
			return nil, unavailable
		}
		return b, nil
	}
	raw, err := get(nil, maxIndex)
	if err != nil {
		return nil, err
	}
	return drawStatic(raw, in, unavailable, func(b staticBoard) ([]byte, error) {
		return get([]string{"__raster", fmt.Sprintf("%x", sha256.Sum256([]byte(b.Path)))}, maxStaticPage)
	})
}

// captureFromStore lists the boards of the project at the revision from the store and has the render worker draw each.
func (s *Service) captureFromStore(ctx context.Context, m store.Machine, project string, at int64, in shareInput, pd PageDrawer) ([]staticPage, error) {
	req := hubproto.DesignPublicTokenRequest{Project: project, Rev: in.Rev, Pages: in.Pages}
	unavailable := fail("unavailable", "Снимок страниц не получился: не все страницы нарисовались. Попробуйте ещё раз, а если повторится — выберите меньше страниц")
	res, _, err := s.readStore(ctx, m, req, at, nil, nil, nil)
	if err != nil {
		s.log.Warn("design snapshot: the store did not list the boards", "project", project, "error", err)
		return nil, unavailable
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, unavailable
	}
	raw, err := readAll(res, maxIndex)
	if err != nil {
		return nil, unavailable
	}
	prefix := s.prefixOf(m.ID)
	// A worker that is down or busy does not keep the owner waiting past this.
	ctx, cancel := context.WithTimeout(ctx, captureWait)
	defer cancel()
	return drawStatic(raw, in, unavailable, func(b staticBoard) ([]byte, error) {
		pic, err := pd.DrawPage(ctx, RenderJob{SpaceID: m.SpaceID, MachineID: m.ID, OwnerID: m.CreatedBy, Prefix: prefix,
			Project: project, Board: b.Path, Rev: at, BoardW: b.W, BoardH: b.H})
		if err != nil {
			s.log.Warn("design snapshot: the render worker did not draw a page", "project", project, "error", err)
			return nil, unavailable
		}
		return pic, nil
	})
}

// captureWait is how long the render worker may take over all the pages of one snapshot.
var captureWait = 2 * time.Minute

// staticBoard is a board of the index a snapshot is drawn from.
type staticBoard struct {
	Path, Title string
	W, H        int
}

// drawStatic checks the index of the boards (raw) against the pages asked for and draws each board by raster.
func drawStatic(raw []byte, in shareInput, unavailable error, raster func(staticBoard) ([]byte, error)) ([]staticPage, error) {
	var index struct {
		Kind   *string
		Boards []staticBoard
	}
	if json.Unmarshal(raw, &index) != nil || index.Kind != nil || len(index.Boards) == 0 || len(index.Boards) > maxStaticBoards {
		return nil, unavailable
	}
	if in.Pages != nil && len(in.Pages) != len(index.Boards) {
		return nil, unavailable
	}
	scope := shareTarget{Pages: in.Pages}
	pages := make([]staticPage, 0, len(index.Boards))
	seen := map[string]bool{}
	size := 0
	for _, b := range index.Boards {
		if b.Path == "" || len(b.Path) > maxBoard || seen[b.Path] || !scope.allows(b.Path) {
			return nil, unavailable
		}
		seen[b.Path] = true
		raw, err := raster(b)
		if err != nil {
			return nil, err
		}
		pic, err := cleanStaticPage(raw)
		if err != nil {
			return nil, unavailable
		}
		size += len(pic)
		if size > maxStaticBundle {
			return nil, unavailable
		}
		pages = append(pages, staticPage{Path: b.Path, Title: clip(b.Title, maxTitle), Image: pic})
	}
	return pages, nil
}

func (t *txn) saveStatic(row shareRow, pages []staticPage) error {
	for i, p := range pages {
		if _, err := t.q.Exec(t.ctx, `INSERT INTO design_static_pages(link_id,rev,position,board,title,jpeg) VALUES(?,?,?,?,?,?)`, row.ID, row.Rev, i, p.Path, p.Title, p.Image); err != nil {
			return err
		}
	}
	return nil
}

var staticView = template.Must(template.New("snapshot").Parse(`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title><style>body{margin:0;font:1rem/1.5 system-ui,sans-serif}nav{display:flex;flex-wrap:wrap;gap:1em;padding:.6em}main{text-align:center;background:#eee}img{max-width:100%;height:auto;vertical-align:middle}</style><nav>{{range .Pages}}<a href="?page={{.N}}">{{.Title}}</a>{{end}}</nav><main><img src="{{.Image}}" alt="{{.Title}}"></main></html>`))

func (s *Service) StaticGuest(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src data:; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'none'; base-uri 'none'")
	deny := func() { http.NotFound(w, r) }
	if !s.staticGuests || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		deny()
		return
	}
	token := strings.TrimPrefix(r.URL.Path, StaticPrefix)
	if !isStaticToken(token) {
		deny()
		return
	}
	state, l, err := s.resolveShare(r.Context(), s.now(), token)
	if err != nil || state != "ok" || l == nil || l.Rights != roleView || l.Version != "pinned" || l.Rev == "" || l.Expires == nil {
		deny()
		return
	}
	m, err := s.db.S().MachineByID(r.Context(), l.Machine)
	if err != nil {
		deny()
		return
	}
	t, err := s.read(r.Context(), m)
	if err != nil {
		deny()
		return
	}
	ok, err := t.makerMayShare(l.Project, l.CreatedBy)
	if err != nil || !ok {
		deny()
		return
	}
	page := 0
	if q := r.URL.Query(); len(q) > 0 {
		v := q.Get("page")
		page, err = strconv.Atoi(v)
		if len(q) != 1 || len(q["page"]) != 1 || err != nil || page < 0 || strconv.Itoa(page) != v {
			deny()
			return
		}
	}
	rows, err := s.db.Query(r.Context(), `SELECT position,board,title FROM design_static_pages WHERE link_id=? AND rev=? ORDER BY position`, l.ID, l.Rev)
	if err != nil {
		deny()
		return
	}
	type navPage struct {
		N     int
		Title string
	}
	var nav []navPage
	var title string
	found := false
	for rows.Next() {
		var item navPage
		var path string
		if err = rows.Scan(&item.N, &path, &item.Title); err != nil {
			break
		}
		if !l.allows(path) {
			continue
		}
		if item.Title == "" {
			item.Title = strconv.Itoa(item.N + 1)
		}
		nav = append(nav, item)
		if item.N == page {
			title = item.Title
			found = true
		}
	}
	rowsErr := rows.Err()
	rows.Close()
	if err != nil || rowsErr != nil || !found {
		deny()
		return
	}
	h.Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	var encoded string
	if s.db.QueryRow(r.Context(), `SELECT jpeg FROM design_static_pages WHERE link_id=? AND rev=? AND position=?`, l.ID, l.Rev, page).Scan(&encoded) != nil {
		deny()
		return
	}
	// Only cleanStaticPage's canonical base64 JPEG is stored in this column.
	_ = staticView.Execute(w, struct {
		Title string
		Pages []navPage
		Image template.URL
	}{title, nav, template.URL("data:image/jpeg;base64," + encoded)}) // #nosec G203 -- image is re-encoded before storage.
}
