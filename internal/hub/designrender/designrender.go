// Package designrender is the hub's queue of the design pictures its render worker draws (HRN-1798,
// docs/hub/render-worker.md). While a project's machine is off, the library's pictures come from the store; a board
// whose picture the store lacks, or whose picture is older than the project's newest revision, is queued here. The
// worker (`harnessly render-worker`, a pod of the hub with Chromium) claims a job, opens the board from the store at the
// job's address and sends the picture back, which goes into the space's thumbs collection like one the machine drew.
// Nothing here needs the project's machine.
package designrender

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// The worker's routes on the hub, with its bearer token (HUB_RENDER_TOKEN).
const (
	ClaimPath = "/agent/render/claim"
	DonePath  = "/agent/render/done"
)

// KindThumb is a board's gallery picture.
const KindThumb = "thumb"

// KindCover is a project's cover: its first board drawn like a picture and kept in the covers collection, as the
// machine's check keeps the shot of the first board (design.CoverFile).
const KindCover = "cover"

// KindPage is a page of a guest link's snapshot: a board drawn at a pinned revision of the store, only when it drew
// cleanly (loaded, no errors, nothing failed), as the machine draws one (design.pinnedRaster). The picture is not
// committed anywhere: its blob goes into the thumbs collection and its sha into the job, and the hub that asked for the
// snapshot (Page) takes it from there. Someone waits for it, so it is given out before the gallery's pictures.
const KindPage = "page"

const (
	// claimWait is how long a claim waits for a job before it answers that there is none.
	claimWait = 20 * time.Second
	// leaseFor is how long a claimed job is the worker's; one not answered by then is given out again.
	leaseFor = 2 * time.Minute
	// maxAttempts: a job that failed so many times is left failed until the project changes again.
	maxAttempts = 3
	// keepFor: finished rows and rows no worker took are dropped after it.
	keepFor = 24 * time.Hour
	// MaxBoardLen caps a job's board path (design.LimitBoardPath).
	MaxBoardLen = 512
)

// Room is a space's thumbs collection as the hub writes it (filestore.Store).
type Room interface {
	Current(ctx context.Context, path string) (*filestore.Entry, error)
	PutBlob(ctx context.Context, sha string, r io.Reader) error
	Commit(ctx context.Context, req filestore.CommitRequest, machine string) (*filestore.Entry, bool, error)
	OpenBlob(ctx context.Context, sha string) (io.ReadCloser, error)
}

// Rooms opens a space's thumbs collection.
type Rooms func(ctx context.Context, spaceID string) (Room, error)

// Job is a picture to draw.
type Job struct {
	ID        string `json:"id"`
	SpaceID   string `json:"-"`
	MachineID string `json:"-"`
	OwnerID   string `json:"-"`
	// Prefix is the platform prefix of the machine's routes (/api/<name>) the board's address is under.
	Prefix  string `json:"-"`
	Project string `json:"project"`
	Board   string `json:"board"`
	Kind    string `json:"kind"`
	Rev     int64  `json:"rev"`
	// BoardW and BoardH are the board's own size when the project says it (0 otherwise): the worker draws at it.
	BoardW int `json:"boardW,omitempty"`
	BoardH int `json:"boardH,omitempty"`
	// Path is the address the worker opens on the hub, relative to it: the board in the store under a key of the
	// project's owner, minted when the job is claimed.
	Path string `json:"path"`
}

// Locate is the hub's address of a job's board under a live key of its owner, relative to the hub; "" when it cannot
// be made (the hub has no key, or the person or project is gone).
type Locate func(j Job) string

// Service is the queue. A nil *Service queues nothing.
type Service struct {
	db     *store.DB
	thumbs Rooms
	covers Rooms
	locate Locate
	bundle Bundle
	token  string
	now    func() time.Time
	log    *slog.Logger

	mu   sync.Mutex
	wake chan struct{}
	gc   time.Time
}

// New makes the queue; nil without a token (no worker is set up, so nothing is queued).
func New(db *store.DB, thumbs Rooms, locate Locate, token string, now func() time.Time, log *slog.Logger) *Service {
	if db == nil || thumbs == nil || locate == nil || strings.TrimSpace(token) == "" {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, thumbs: thumbs, locate: locate, token: strings.TrimSpace(token), now: now, log: log, wake: make(chan struct{})}
}

// UseCovers gives the queue the spaces' covers collections: without them no cover is queued.
func (s *Service) UseCovers(r Rooms) {
	if s != nil {
		s.covers = r
	}
}

// On tells whether the hub has a worker to draw for it.
func (s *Service) On() bool { return s != nil }

// Want queues a picture unless the same board at the same revision is already queued or drawn.
func (s *Service) Want(ctx context.Context, j Job) error {
	if s == nil {
		return nil
	}
	if j.SpaceID == "" || j.MachineID == "" || j.OwnerID == "" || j.Project == "" || j.Board == "" ||
		!strings.HasPrefix(j.Prefix, "/api/") || len(j.Board) > MaxBoardLen {
		return errors.New("designrender: an incomplete job")
	}
	if j.Kind == "" {
		j.Kind = KindThumb
	}
	if j.Kind != KindThumb && j.Kind != KindPage && (j.Kind != KindCover || s.covers == nil) && (!isWork(j.Kind) || s.bundle == nil) {
		return nil
	}
	now := s.now().UnixMilli()
	res, err := s.db.Exec(ctx, `INSERT INTO design_render_jobs (id, space_id, project_id, board, kind, rev, machine_id, owner_id,
		prefix, board_w, board_h, state, attempts, lease_ms, error, created_ms, updated_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', 0, 0, '', ?, ?) ON CONFLICT DO NOTHING`,
		newID(), j.SpaceID, j.Project, j.Board, j.Kind, j.Rev, j.MachineID, j.OwnerID, j.Prefix, j.BoardW, j.BoardH, now, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.nudge()
	}
	return nil
}

func (s *Service) nudge() {
	s.mu.Lock()
	close(s.wake)
	s.wake = make(chan struct{})
	s.mu.Unlock()
}

func (s *Service) waiter() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wake
}

// authorized: the request carries the worker's token.
func (s *Service) authorized(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(s.token)) == 1
}

// ServeClaim is POST ClaimPath: the next job, waiting up to claimWait for one; 204 when there is none.
func (s *Service) ServeClaim(w http.ResponseWriter, r *http.Request) error {
	if !s.authorized(r) {
		return httpx.New(http.StatusUnauthorized, "unauthorized", "Нужен токен воркера рендера")
	}
	ctx := r.Context()
	deadline := time.NewTimer(claimWait)
	defer deadline.Stop()
	for {
		wake := s.waiter()
		// A worker busy with a check or an export asks for pictures only (?pictures=1), which take a second.
		j, err := s.claim(ctx, r.URL.Query().Get("pictures") == "1")
		if err != nil {
			return err
		}
		if j != nil {
			httpx.WriteJSON(w, http.StatusOK, j)
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-deadline.C:
			httpx.NoContent(w)
			return nil
		case <-wake:
		case <-time.After(3 * time.Second): // a job queued on another replica
		}
	}
}

// claim takes the oldest job that is queued or whose worker went quiet; nil when there is none.
func (s *Service) claim(ctx context.Context, picturesOnly bool) (*Job, error) {
	only := ""
	if picturesOnly {
		only = ` AND kind NOT IN ('` + KindCheck + `', '` + KindExport + `')`
	}
	now := s.now()
	s.sweep(ctx, now)
	for range 5 {
		var j Job
		err := s.db.QueryRow(ctx, `SELECT id, space_id, project_id, board, kind, rev, machine_id, owner_id, prefix, board_w, board_h
			FROM design_render_jobs WHERE (state = 'queued' OR (state = 'claimed' AND lease_ms < ?)) AND attempts < ?`+only+`
			ORDER BY CASE kind WHEN 'page' THEN 0 WHEN 'check' THEN 1 WHEN 'export' THEN 1 ELSE 2 END, created_ms LIMIT 1`, now.UnixMilli(), maxAttempts).
			Scan(&j.ID, &j.SpaceID, &j.Project, &j.Board, &j.Kind, &j.Rev, &j.MachineID, &j.OwnerID, &j.Prefix, &j.BoardW, &j.BoardH)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !isWork(j.Kind) {
			j.Path = s.locate(j)
		}
		if j.Path == "" && !isWork(j.Kind) {
			// Nothing to open it at: the job is dropped rather than given out again and again.
			if _, err := s.db.Exec(ctx, `UPDATE design_render_jobs SET state = 'failed', error = 'no address', updated_ms = ?
				WHERE id = ?`, now.UnixMilli(), j.ID); err != nil {
				return nil, err
			}
			continue
		}
		res, err := s.db.Exec(ctx, `UPDATE design_render_jobs SET state = 'claimed', attempts = attempts + 1, lease_ms = ?,
			updated_ms = ? WHERE id = ? AND (state = 'queued' OR (state = 'claimed' AND lease_ms < ?))`,
			now.Add(leaseOf(j.Kind)).UnixMilli(), now.UnixMilli(), j.ID, now.UnixMilli())
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return &j, nil
		}
		// Another replica took it first: look again.
	}
	return nil, nil
}

// sweep drops, at most once a minute, the rows that are done with and those no worker took for a day.
func (s *Service) sweep(ctx context.Context, now time.Time) {
	s.mu.Lock()
	due := now.Sub(s.gc) > time.Minute
	if due {
		s.gc = now
	}
	s.mu.Unlock()
	if !due {
		return
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM design_render_jobs WHERE updated_ms < ? AND (state <> 'claimed' OR lease_ms < ?)`,
		now.Add(-keepFor).UnixMilli(), now.UnixMilli()); err != nil {
		s.log.Warn("designrender: old jobs were not dropped", "error", err)
	}
}

// ServeDone is POST DonePath?id=<job>[&error=<why>]: the picture of a claimed job (image/jpeg), or why it was not drawn.
func (s *Service) ServeDone(w http.ResponseWriter, r *http.Request) error {
	if !s.authorized(r) {
		return httpx.New(http.StatusUnauthorized, "unauthorized", "Нужен токен воркера рендера")
	}
	ctx := r.Context()
	id := r.URL.Query().Get("id")
	var j Job
	var state string
	var attempts int
	err := s.db.QueryRow(ctx, `SELECT space_id, project_id, board, kind, state, attempts FROM design_render_jobs WHERE id = ?`, id).
		Scan(&j.SpaceID, &j.Project, &j.Board, &j.Kind, &state, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.New(http.StatusNotFound, "not_found", "Задачи нет")
	}
	if err != nil {
		return err
	}
	if state != "claimed" {
		return httpx.Conflict("not_claimed", "Задача не выдана воркеру")
	}
	if why := r.URL.Query().Get("error"); why != "" {
		next := "queued"
		// A page someone waits for is not drawn three times: a board that does not draw cleanly does not on the next try
		// either, and the owner is told at once. Nor is a check or an export: the person who asked reads why.
		if attempts >= maxAttempts || j.Kind == KindPage || isWork(j.Kind) {
			next = "failed"
		}
		s.log.Warn("designrender: a picture was not drawn", "project", j.Project, "board", j.Board, "error", why)
		return s.finish(ctx, w, id, next, why)
	}
	if isWork(j.Kind) {
		return s.doneWork(ctx, w, r, id, j)
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, filestore.MaxThumbBytes+1))
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > filestore.MaxThumbBytes || !bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
		return httpx.Invalid("body", "Нужна картинка JPEG до 1 МБ")
	}
	if j.Kind == KindPage {
		sha, err := s.putPage(ctx, j, data)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, `UPDATE design_render_jobs SET result = ? WHERE id = ?`, sha, id); err != nil {
			return err
		}
		return s.finish(ctx, w, id, "done", "")
	}
	put := s.putThumb
	if j.Kind == KindCover {
		put = s.putCover
	}
	if err := put(ctx, j, data); err != nil {
		return err
	}
	return s.finish(ctx, w, id, "done", "")
}

// putPage keeps a snapshot's page as a blob of the space's thumbs collection: the store's GC keeps a blob nothing
// refers to for a day, far longer than the hub that waits for it needs.
func (s *Service) putPage(ctx context.Context, j Job, data []byte) (string, error) {
	room, err := s.thumbs(ctx, j.SpaceID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	return sha, room.PutBlob(ctx, sha, bytes.NewReader(data))
}

// pagePoll is how often a hub waiting for a snapshot's page looks at its job: the worker may answer another replica.
var pagePoll = 300 * time.Millisecond

// ErrPageFailed: the worker could not draw the page cleanly.
var ErrPageFailed = errors.New("designrender: the page was not drawn")

// Page asks the worker to draw a board at a revision as a page of a guest link's snapshot and waits for it (until ctx
// ends): the JPEG, ErrPageFailed when the worker could not draw it cleanly. The same board at the same revision drawn
// earlier is given at once; one that failed before is tried again.
func (s *Service) Page(ctx context.Context, j Job) ([]byte, error) {
	if s == nil {
		return nil, errors.New("designrender: no render worker")
	}
	j.Kind = KindPage
	if err := s.Want(ctx, j); err != nil {
		return nil, err
	}
	// again queues the page anew when it is in state: a page that failed before this ask, or one whose blob is gone.
	again := func(state string) error {
		res, err := s.db.Exec(ctx, `UPDATE design_render_jobs SET state = 'queued', attempts = 0, error = '', result = NULL,
			lease_ms = 0, updated_ms = ? WHERE space_id = ? AND project_id = ? AND board = ? AND kind = ? AND rev = ?
			AND state = ?`, s.now().UnixMilli(), j.SpaceID, j.Project, j.Board, KindPage, j.Rev, state)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			s.nudge()
		}
		return nil
	}
	if err := again("failed"); err != nil {
		return nil, err
	}
	redrawn := false
	tick := time.NewTicker(pagePoll)
	defer tick.Stop()
	for {
		var state string
		var result sql.NullString
		err := s.db.QueryRow(ctx, `SELECT state, result FROM design_render_jobs WHERE space_id = ? AND project_id = ?
			AND board = ? AND kind = ? AND rev = ?`, j.SpaceID, j.Project, j.Board, KindPage, j.Rev).Scan(&state, &result)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// Swept between the queueing and now: queued again.
			if err := s.Want(ctx, j); err != nil {
				return nil, err
			}
		case err != nil:
			return nil, err
		case state == "failed":
			return nil, ErrPageFailed
		case state == "done":
			pic, err := s.pageBlob(ctx, j.SpaceID, result.String)
			if err == nil {
				return pic, nil
			}
			if redrawn {
				return nil, err
			}
			// The blob of a page drawn long ago is gone: drawn again, once.
			redrawn = true
			if err := again("done"); err != nil {
				return nil, err
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}

func (s *Service) pageBlob(ctx context.Context, spaceID, sha string) ([]byte, error) {
	room, err := s.thumbs(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	rc, err := room.OpenBlob(ctx, sha)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, filestore.MaxThumbBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > filestore.MaxThumbBytes || !bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
		return nil, errors.New("designrender: the page's blob is not a picture")
	}
	return data, nil
}

func (s *Service) finish(ctx context.Context, w http.ResponseWriter, id, state, why string) error {
	if len(why) > 500 {
		why = why[:500]
	}
	if _, err := s.db.Exec(ctx, `UPDATE design_render_jobs SET state = ?, error = ?, lease_ms = 0, updated_ms = ? WHERE id = ?`,
		state, why, s.now().UnixMilli(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// putThumb writes the picture into the space's thumbs collection, over the one there.
func (s *Service) putThumb(ctx context.Context, j Job, data []byte) error {
	path := filestore.ThumbPath(j.Project, j.Board)
	if !(filestore.Thumbs{}).Synced(path) {
		return httpx.Invalid("board", "Такой доски нет")
	}
	return s.put(ctx, s.thumbs, j.SpaceID, path, data)
}

// putCover writes the picture of the project's first board into the space's covers collection, over the one there.
func (s *Service) putCover(ctx context.Context, j Job, data []byte) error {
	path := filestore.CoverPath(j.Project)
	if s.covers == nil || !(filestore.Covers{}).Synced(path) {
		return httpx.Invalid("project", "Обложку этого проекта не пишут")
	}
	return s.put(ctx, s.covers, j.SpaceID, path, data)
}

// put writes data at path of a space's collection, over the file there.
func (s *Service) put(ctx context.Context, rooms Rooms, spaceID, path string, data []byte) error {
	room, err := rooms(ctx, spaceID)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	if err := room.PutBlob(ctx, sha, bytes.NewReader(data)); err != nil {
		return err
	}
	for range 3 {
		cur, err := room.Current(ctx, path)
		if err != nil {
			return err
		}
		var base int64
		if cur != nil {
			base = cur.Rev
		}
		_, conflict, err := room.Commit(ctx, filestore.CommitRequest{Path: path, Base: base, Sha: sha, Size: int64(len(data)), Author: "render"}, "hub-render")
		if err != nil {
			return err
		}
		if !conflict {
			return nil
		}
	}
	return httpx.Conflict("busy", "Картинку доски пишут одновременно")
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
