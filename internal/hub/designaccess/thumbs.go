package designaccess

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/safego"
)

// The cards of the library are the pictures of the projects' first boards, which the machine draws and serves
// (GET <platform>/design/library/thumb). While the machine is off the hub's library still lists its projects from the
// store, and the hub answers their pictures too: the newest one the machine drew, which it put into the store's thumbs
// collection. A machine that is on is always asked: it draws the picture of the board as it is now.

// Thumber is the part of a Viewer that gives the board pictures kept in the store (designview.View).
type Thumber interface {
	// Thumb is the picture of a board, at most limit bytes, with its sha; nil when there is none.
	Thumb(ctx context.Context, spaceID, project, boardPath string, limit int64) ([]byte, string, error)
}

// ThumbWanter is the part of a Viewer that tells whether a board's picture in the store is missing or older than the
// project (designview.View).
type ThumbWanter interface {
	ThumbWant(ctx context.Context, spaceID, project, boardPath string) (rev int64, w, h int, want bool, err error)
}

// RenderJob is a board picture the hub's render worker is asked to draw (designrender.Job).
type RenderJob struct {
	SpaceID, MachineID, OwnerID, Prefix, Project, Board string
	Rev                                                 int64
	BoardW, BoardH                                      int
}

// RenderQueue is the hub's queue of the pictures its render worker draws (designrender.Service): with it the library's
// pictures of a machine that is off are drawn again when the project changes, not only when the machine comes back.
type RenderQueue interface {
	WantThumb(ctx context.Context, j RenderJob) error
}

// PageDrawer is the part of a RenderQueue that draws a page of a guest link's snapshot at a pinned revision of the store
// and waits for it (designrender.Service.Page): with it a snapshot does not need the owner's machine.
type PageDrawer interface {
	DrawPage(ctx context.Context, j RenderJob) ([]byte, error)
}

// CoverWanter is the part of a Viewer that tells whether a project's cover in the store is missing or older than the
// project while the board is its first one, whose picture is the cover (designview.View).
type CoverWanter interface {
	CoverWant(ctx context.Context, spaceID, project, boardPath string) (rev int64, want bool, err error)
}

// CoverQueue is a RenderQueue that also draws a project's cover from its first board (designrender.Service).
type CoverQueue interface {
	WantCover(ctx context.Context, j RenderJob) error
}

// UseRenderQueue gives the service the hub's render queue.
func (s *Service) UseRenderQueue(q RenderQueue) { s.render = q }

// renderChecks remembers when a board's picture was last looked at for the queue, so a library page that asks for
// every card again and again costs one look a board per renderRecheck.
type renderChecks struct {
	mu   sync.Mutex
	at   map[string]time.Time
	busy int
}

const (
	renderRecheck = 30 * time.Second
	renderBusyMax = 4
)

// take says whether key is due a look now, and holds one of the few looks running at once.
func (c *renderChecks) take(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy >= renderBusyMax || now.Sub(c.at[key]) < renderRecheck {
		return false
	}
	if c.at == nil || len(c.at) > 10000 {
		c.at = map[string]time.Time{}
	}
	c.at[key] = now
	c.busy++
	return true
}

func (c *renderChecks) done() {
	c.mu.Lock()
	c.busy--
	c.mu.Unlock()
}

// wantRender queues the board's picture for the render worker when the store's one is missing or older than the
// project; it looks in the background and never holds the answer.
func (s *Service) wantRender(r *http.Request, m store.Machine, project, board string) {
	tw, ok := s.view.(ThumbWanter)
	if !ok || s.render == nil || m.CreatedBy == "" {
		return
	}
	prefix, _, found := strings.Cut(strings.TrimPrefix(r.URL.Path, "/m/"+m.ID), "/design/library/thumb")
	if !found || !strings.HasPrefix(prefix, "/api/") {
		return
	}
	key := m.SpaceID + "/" + project + "/" + board
	if !s.renderChecks.take(key, s.now()) {
		return
	}
	safego.Go("design render want", func() {
		defer s.renderChecks.done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rev, bw, bh, want, err := tw.ThumbWant(ctx, m.SpaceID, project, board)
		if err == nil && want {
			err = s.render.WantThumb(ctx, RenderJob{SpaceID: m.SpaceID, MachineID: m.ID, OwnerID: m.CreatedBy, Prefix: prefix,
				Project: project, Board: board, Rev: rev, BoardW: bw, BoardH: bh})
		}
		if err != nil {
			s.log.Warn("design: a board picture was not queued for drawing", "project", project, "error", err)
		}
		cw, okW := s.view.(CoverWanter)
		cq, okQ := s.render.(CoverQueue)
		if !okW || !okQ {
			return
		}
		rev, want, err = cw.CoverWant(ctx, m.SpaceID, project, board)
		if err == nil && want {
			err = cq.WantCover(ctx, RenderJob{SpaceID: m.SpaceID, MachineID: m.ID, OwnerID: m.CreatedBy, Prefix: prefix,
				Project: project, Board: board, Rev: rev, BoardW: bw, BoardH: bh})
		}
		if err != nil {
			s.log.Warn("design: a project's cover was not queued for drawing", "project", project, "error", err)
		}
	})
}

// RenderLocation is the address, relative to the hub, the render worker opens a board at: the board in the store
// under a live key of the project's owner; "" without a key (the hub has no master key) or for a bad project.
func (s *Service) RenderLocation(machineID, ownerID, prefix, project, board string, at int64) string {
	if s == nil || s.files == nil || !projectIDRe.MatchString(project) || !strings.HasPrefix(prefix, "/api/") {
		return ""
	}
	key := s.files.mint(ownerID, project, s.now())
	if key == "" {
		return ""
	}
	segs := strings.Split(board, "/")
	for i, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return ""
		}
		segs[i] = url.PathEscape(seg)
	}
	ver := ""
	if at > 0 {
		// A pinned revision (a snapshot's page): the board as it was then, read only (team.go, v/<version>/f/...).
		ver = "v/" + strconv.FormatInt(at, 10) + "/"
	}
	return "/m/" + machineID + prefix + "/design/store/" + project + "/k/" + key + "/" + ver + "f/" + strings.Join(segs, "/")
}

// thumbPath is the machine's route of a board's picture under a platform prefix.
var thumbPath = regexp.MustCompile(`^/api/[A-Za-z0-9_-]+/design/library/thumb$`)

// maxThumb caps a board picture the hub serves.
const maxThumb = 1 << 20

// maxThumbBoard caps the board path of a picture asked for (design.LimitBoardPath).
const maxThumbBoard = 512

// serveThumb answers a board's picture from the store when the machine is off and the hub owns the design facts of its
// person; false leaves the request to the machine.
func (s *Service) serveThumb(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine) bool {
	th, ok := s.view.(Thumber)
	if !ok || m.SpaceID == "" || (s.reg != nil && s.reg.Get(m.ID) != nil) || !s.OwnerOn(r.Context(), m) {
		return false
	}
	q := r.URL.Query()
	id, board := q.Get("artifact"), q.Get("board")
	none := func() bool {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	if !projectIDRe.MatchString(id) || board == "" || len(board) > maxThumbBoard || strings.ContainsRune(board, 0) {
		return none()
	}
	if may, err := s.mayOpen(r.Context(), u, m, id); err != nil || !may {
		return none()
	}
	s.wantRender(r, m, id, board)
	b, sha, err := th.Thumb(r.Context(), m.SpaceID, id, board, maxThumb)
	if err != nil || len(b) == 0 {
		if err != nil {
			s.log.Warn("design: a board picture was not read from the store", "project", id, "error", err)
		}
		return none()
	}
	etag := `"` + sha[:16] + `"`
	w.Header().Set("ETag", etag)
	// The picture may be of an earlier version of the board: it is checked again each time, as the machine's earlier one.
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(b)
	return true
}
