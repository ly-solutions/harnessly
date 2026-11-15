package docs

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/ly-solutions/harnessly/internal/hub/resaccess"
)

// guestPage is what a guest link opens (GET /d/{token}, the address resaccess hands out): the document itself as a
// page of its own, in the sandbox the viewer uses, without signing in. The link's version stays pinned; a link that
// allows comments opens the document to read here, the comments go through the guest API. A link to a board opens the
// board to read, as its service's guest view draws it.
func (s *Service) guestPage(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("Cache-Control", "no-store")
	g, err := s.access.Resolve(r.Context(), r.PathValue("token"))
	if err == nil && g.Kind != resaccess.KindDoc {
		err = errors.New("not a document link")
	}
	if err != nil {
		status, text := http.StatusNotFound, "Ссылка не найдена"
		var e *resaccess.Error
		if errors.As(err, &e) {
			status, text = resaccess.Status(err), e.Message
		}
		guestError(w, status, text)
		return
	}
	page, status, text := s.guestBody(r, g)
	if status != http.StatusOK {
		guestError(w, status, text)
		return
	}
	s.access.RecordOpen(r.Context(), g.LinkID)
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "sandbox allow-scripts allow-popups")
	_, _ = w.Write(page)
}

// targetBoard marks a guest link that was made for a board (resaccess.Info.Target).
const targetBoard = "board"

// linkTarget says what a slug is when a link to it is made: a board when the boards service has it and the documents
// service does not, otherwise a document.
func (s *Service) linkTarget(ctx context.Context, slug string) (string, error) {
	if !s.Status().Boards {
		return "", nil
	}
	if s.art != nil {
		docs, err := s.art.list(ctx)
		if err != nil {
			return "", err
		}
		for _, a := range docs {
			if a.Slug == slug {
				return "", nil
			}
		}
	}
	return s.boardsOnlyTarget(ctx, slug)
}

// boardsOnlyTarget is the boards service's half of linkTarget.
func (s *Service) boardsOnlyTarget(ctx context.Context, slug string) (string, error) {
	bs, err := s.boards(ctx)
	if err != nil {
		return "", err
	}
	for _, b := range bs {
		if b.Slug == slug {
			return targetBoard, nil
		}
	}
	return "", nil
}

// guestBody is the page of what the link opens. What that is was written into the link when it was made (its target):
// a board's link asks the boards service for its guest view, the read-only page that carries no key of any board
// any other link asks the documents service. The hub never tries the other service for a slug the first
// one does not have: its token would open there what the link was never given for.
func (s *Service) guestBody(r *http.Request, g *resaccess.Guest) ([]byte, int, string) {
	ctx := r.Context()
	if g.Target == targetBoard {
		page, err := s.boardGuestPage(ctx, g.ID)
		switch {
		case err == nil:
			return page, http.StatusOK, ""
		case errors.Is(err, errNoBoardView):
			s.log.Warn("boards service has no guest view", "route", "guest page")
			return nil, http.StatusServiceUnavailable, "Доска по ссылке пока не открывается"
		case errors.Is(err, errNoBoards), isStatus(err, http.StatusNotFound):
			return nil, http.StatusNotFound, "Доска не найдена"
		}
		s.log.Warn("boards service unavailable", "route", "guest page", "error", err)
		return nil, http.StatusBadGateway, "Сервис досок не отвечает"
	}
	if s.art == nil {
		return nil, http.StatusServiceUnavailable, "Документы сейчас недоступны"
	}
	res, err := s.art.page(ctx, g.ID, g.Pinned, themeOf(r.URL.Query().Get("theme")))
	if err != nil {
		s.log.Warn("documents service unavailable", "route", "guest page", "error", err)
		return nil, http.StatusBadGateway, "Сервис документов не отвечает"
	}
	defer func() { _ = res.Body.Close() }()
	// The answer's status is the service's word on the slug: its page of an error is never passed on as the document.
	switch res.StatusCode {
	case http.StatusOK:
		page, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		if err != nil {
			return nil, http.StatusBadGateway, "Сервис документов не отвечает"
		}
		return page, http.StatusOK, ""
	case http.StatusNotFound:
		return nil, http.StatusNotFound, "Документ не найден"
	}
	s.log.Warn("documents service unavailable", "route", "guest page", "status", res.StatusCode)
	return nil, http.StatusBadGateway, "Сервис документов не отвечает"
}

var (
	errNoBoards    = errors.New("the boards service is not configured")
	errNoBoardView = errors.New("the boards service did not answer with its guest view")
)

// boardGuestPage asks the boards service for the guest view of a board: its read-only page without any board key,
// which only the hub's token gets. The service marks it (X-Boards-View: guest); a build that does not know the view
// answers with its ordinary page, the board's key in it, and that page is never passed on.
func (s *Service) boardGuestPage(ctx context.Context, slug string) ([]byte, error) {
	if !s.Status().Boards {
		return nil, errNoBoards
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.BoardsURL+"/b/"+url.PathEscape(slug)+"?embed=1&guest=1", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.BoardsToken)
	req.Header.Set("Accept", "text/html")
	res, err := s.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, &upstreamError{Status: res.StatusCode}
	}
	if res.Header.Get("X-Boards-View") != "guest" {
		return nil, errNoBoardView
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}

func guestError(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(status)
	t := html.EscapeString(text)
	_, _ = io.WriteString(w, `<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`+t+
		`</title><style>body{font:16px/1.5 system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;margin:0;color:#333;background:#fafafa}@media(prefers-color-scheme:dark){body{color:#ddd;background:#161616}}</style></head><body><p>`+t+`</p></body></html>`)
}

var guestFontName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}\.woff2$`)

// guestFont serves one font of the document's page (GET /static/fonts/<name>.woff2): the page names its fonts by a
// path of its own origin, and on the hub's address they are read from the documents service, which gives them to
// anybody (they answered 404 on the share host, and the page fell back to the system font). The page is
// sandboxed (an origin of its own), so the font is given to any origin, as the documents service gives it.
func (s *Service) guestFont(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.art == nil || !guestFontName.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	res, err := s.art.raw(r.Context(), http.MethodGet, "/static/fonts/"+name, nil, "font/woff2")
	if err != nil {
		http.Error(w, "the documents service does not answer", http.StatusBadGateway)
		return
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "font/woff2")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, io.LimitReader(res.Body, 4<<20))
}
