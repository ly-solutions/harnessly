package designaccess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/sitesclient"
	"github.com/ly-solutions/harnessly/internal/storerev"
)

// HRN-1799 (stage 7). «Опубликовать как сайт» with the machine off: the hub makes the site's files from the
// project's newest revision in the store (or the version asked for) and deploys them as the next version of the site,
// as the machine would; GET design/published says which site the project has. The apps' checks are not passed by: the
// verdict the machine left for the newest revision decides, and with none the project is not published.

var (
	publishPath   = regexp.MustCompile(`^/api/[A-Za-z0-9_-]+/design/publish$`)
	publishedPath = regexp.MustCompile(`^/api/[A-Za-z0-9_-]+/design/published$`)
)

// SitePublish is a site to deploy for a machine's space.
type SitePublish struct {
	Name, Access, Origin string
	Files                []sitesclient.DeployFile
}

// PublishedSite is a site made from a project, as «Поделиться» shows it.
type PublishedSite struct {
	URL     string `json:"url"`
	Name    string `json:"name"`
	Access  string `json:"access"`
	Version int    `json:"version"`
}

// ErrSiteNameTaken: a site of that name is someone else's, or another project's.
var ErrSiteNameTaken = errors.New("designaccess: the site's name is taken")

// SiteRefusal is the hub's sites refusing a deployment with words for the person (a limit, a secret in a page).
type SiteRefusal struct{ Message string }

func (e *SiteRefusal) Error() string { return "designaccess: the site was refused: " + e.Message }

// SitePublisher deploys a project's files as a site of the machine's space, as the machine's own publishing session
// would (the site is the machine's, bound to the project by its origin), for the person who asked.
type SitePublisher interface {
	PublishSite(ctx context.Context, m store.Machine, u store.User, p SitePublish) (PublishedSite, error)
	FindSite(ctx context.Context, m store.Machine, origin string) (*PublishedSite, error)
}

// ErrPublishEmpty is what a PublishFiler answers when nothing of the project is left to publish (design.ErrPublishEmpty
// on the machine: the hub's packages do not import internal/design, which imports internal/agent).
var ErrPublishEmpty = errors.New("nothing to publish")

// PublishFiler is a Viewer that makes a site's files from a revision of the store (designview.View.PublishFiles).
type PublishFiler interface {
	PublishFiles(ctx context.Context, spaceID, project string, at int64, pages []string) (string, []sitesclient.DeployFile, error)
}

// UseSites gives the hub's way to publish sites; it is set once, before the hub serves.
func (s *Service) UseSites(p SitePublisher) { s.sites = p }

type publishBody struct {
	Project  string    `json:"project"`
	Name     string    `json:"name"`
	Access   string    `json:"access"`
	Pages    *[]string `json:"pages"`
	Revision string    `json:"revision"`
}

func (s *Service) servePublish(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine) bool {
	rest := strings.TrimPrefix(r.URL.EscapedPath(), "/m/"+m.ID)
	post := r.Method == http.MethodPost && publishPath.MatchString(rest)
	get := r.Method == http.MethodGet && publishedPath.MatchString(rest)
	if !post && !get {
		return false
	}
	files, ok := s.view.(PublishFiler)
	if !ok || s.sites == nil || s.machineUp(m.ID) {
		return false
	}
	ctx := r.Context()
	if !s.machineOff(ctx, m) {
		return false
	}
	var b publishBody
	if post {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
		if err != nil || len(raw) > 64<<10 || json.Unmarshal(raw, &b) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный запрос"})
			return true
		}
	} else {
		b.Project = r.URL.Query().Get("project")
	}
	if !projectIDRe.MatchString(b.Project) || (post && strings.TrimSpace(b.Name) == "") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный запрос"})
		return true
	}
	t, err := s.read(ctx, m)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	p, stored, err := t.effective(b.Project)
	if err != nil || !stored {
		return false
	}
	f, err := t.fold(p, u.Email)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	if rank(f.Role) < rank(roleManage) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Нет доступа к проекту", "code": "no_access", "role": f.Role})
		return true
	}
	origin := "sinopia:" + b.Project // design.PublishOrigin: the hub does not import the machine's design package
	if get {
		site, err := s.sites.FindSite(ctx, m, origin)
		if err != nil {
			s.log.Warn("design publish: the hub's sites did not answer", "machine", m.ID, "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Хаб не ответил"})
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"site": site})
		return true
	}
	at, err := s.view.Head(ctx, m.SpaceID, b.Project)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	if b.Revision != "" {
		rev, ok := storerev.Of(b.Revision)
		if !ok || rev <= 0 || rev > at {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Такой версии проекта нет"})
			return true
		}
		at = rev
	}
	if at <= 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Нет страниц для публикации"})
		return true
	}
	// A site is shared as much as a link: the apps' verdict for the newest revision is asked for, always.
	if err := s.storedVerdict(ctx, m, b.Project); err != nil {
		s.writeError(w, err)
		return true
	}
	var pages []string
	if b.Pages != nil {
		pages = append([]string{}, (*b.Pages)...)
	}
	title, out, err := files.PublishFiles(ctx, m.SpaceID, b.Project, at, pages)
	switch {
	case errors.Is(err, ErrPublishEmpty):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Нет страниц для публикации"})
		return true
	case err != nil:
		s.log.Warn("design publish: not read from the store", "machine", m.ID, "project", b.Project, "rev", at, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Проект сейчас не прочитать: попробуйте ещё раз", "code": "store_unavailable"})
		return true
	}
	access := b.Access
	if access != "team" && access != "link" {
		access = "team"
	}
	site, err := s.sites.PublishSite(ctx, m, u, SitePublish{Name: strings.TrimSpace(b.Name), Access: access, Origin: origin, Files: out})
	var refused *SiteRefusal
	switch {
	case errors.Is(err, ErrSiteNameTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Имя занято: выберите другое", "code": "name_taken"})
		return true
	case errors.As(err, &refused):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": refused.Message})
		return true
	case err != nil:
		s.log.Warn("design publish: the hub's sites did not take the site", "machine", m.ID, "project", b.Project, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Хаб не принял сайт"})
		return true
	}
	w.Header().Set("X-Harnessly-From", "store")
	writeJSON(w, http.StatusOK, map[string]any{"url": site.URL, "version": site.Version, "name": site.Name, "title": title, "files": len(out)})
	return true
}
