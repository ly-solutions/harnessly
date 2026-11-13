package docs

import (
	"errors"
	"html"
	"io"
	"net/http"
	"regexp"

	"github.com/ly-solutions/harnessly/internal/hub/resaccess"
)

// guestPage is what a guest link opens (GET /d/{token}, the address resaccess hands out): the document itself as a
// page of its own, in the sandbox the viewer uses, without signing in. The link's version stays pinned; a link that
// allows comments opens the document to read here, the comments go through the guest API.
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
	if s.art == nil {
		guestError(w, http.StatusServiceUnavailable, "Документы сейчас недоступны")
		return
	}
	res, err := s.art.page(r.Context(), g.ID, g.Pinned, themeOf(r.URL.Query().Get("theme")))
	if err != nil {
		if isStatus(err, http.StatusNotFound) {
			guestError(w, http.StatusNotFound, "Документ не найден")
			return
		}
		s.log.Warn("documents service unavailable", "route", "guest page", "error", err)
		guestError(w, http.StatusBadGateway, "Сервис документов не отвечает")
		return
	}
	defer func() { _ = res.Body.Close() }()
	page, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		guestError(w, http.StatusBadGateway, "Сервис документов не отвечает")
		return
	}
	s.access.RecordOpen(r.Context(), g.LinkID)
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "sandbox allow-scripts allow-popups")
	_, _ = w.Write(page)
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
