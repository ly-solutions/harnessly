package designrender

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/httpx"
)

// The board check and the exports of a project whose machine is off (docs/hub/render-worker.md). The worker
// takes a copy of the project from the store at the job's revision (BundlePath), runs the machine's own check or export
// on it and sends the result back: the check's folder as a gzipped tar, or the export's file. The hub keeps it as a blob
// of the space's thumbs collection and its sha in the job, and answers the person's page from there.

// BundlePath is the worker's route for the copy of a check's or an export's project.
const BundlePath = "/agent/render/bundle"

// KindCheck is a check of a project's boards; the job's board is "*" for all of them, or the boards asked for, one a
// line.
const KindCheck = "check"

// KindExport is an export of a project; the job's board is "<format>\n<recording>\n<board>".
const KindExport = "export"

// MaxResultBytes caps what a worker sends back of a check or an export.
const MaxResultBytes = 512 << 20

// ErrNoWorker: the hub has no worker that runs checks and exports.
var ErrNoWorker = errors.New("designrender: no render worker for checks and exports")

func isWork(kind string) bool { return kind == KindCheck || kind == KindExport }

// leaseOf is how long a claimed job of a kind is the worker's: a check of many boards and a video take minutes.
func leaseOf(kind string) time.Duration {
	switch kind {
	case KindCheck:
		return 20 * time.Minute
	case KindExport:
		return 45 * time.Minute
	}
	return leaseFor
}

// Bundle writes a project at a revision as the worker lays it out: a gzipped tar of library/<project>/… and of the
// project's kit under kits/<kit>/….
type Bundle func(ctx context.Context, spaceID, project string, rev int64, w io.Writer) error

// UseBundles gives the queue the copies of projects: without them no check or export is queued.
func (s *Service) UseBundles(b Bundle) {
	if s != nil {
		s.bundle = b
	}
}

// Works tells whether the hub's worker runs checks and exports.
func (s *Service) Works() bool { return s != nil && s.bundle != nil }

// Work is a check's or an export's job as the hub answers about it.
type Work struct {
	ID          string
	Kind        string
	Board       string
	State       string // queued, claimed, done or failed
	Error       string
	Result      string // the sha of the result's blob, when done
	Disposition string // the export's Content-Disposition, when done
	Rev         int64
	Created     int64 // unix ms
	Updated     int64 // unix ms
}

const workColumns = `id, kind, board, state, error, result, disposition, rev, created_ms, updated_ms`

func scanWork(row interface{ Scan(...any) error }) (Work, error) {
	var w Work
	var result, disposition sql.NullString
	err := row.Scan(&w.ID, &w.Kind, &w.Board, &w.State, &w.Error, &result, &disposition, &w.Rev, &w.Created, &w.Updated)
	w.Result, w.Disposition = result.String, disposition.String
	return w, err
}

// Run queues a check or an export (j.Kind, its arguments in j.Board) of a project at a revision and returns its job: the
// one already queued, running or done with the same arguments at the same revision, or a failed one queued again.
func (s *Service) Run(ctx context.Context, j Job) (Work, error) {
	if !s.Works() {
		return Work{}, ErrNoWorker
	}
	if !isWork(j.Kind) {
		return Work{}, errors.New("designrender: not a check or an export")
	}
	if err := s.Want(ctx, j); err != nil {
		return Work{}, err
	}
	res, err := s.db.Exec(ctx, `UPDATE design_render_jobs SET state = 'queued', attempts = 0, error = '', result = NULL,
		disposition = NULL, lease_ms = 0, updated_ms = ? WHERE space_id = ? AND project_id = ? AND board = ? AND kind = ?
		AND rev = ? AND state = 'failed'`, s.now().UnixMilli(), j.SpaceID, j.Project, j.Board, j.Kind, j.Rev)
	if err != nil {
		return Work{}, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.nudge()
	}
	w, err := scanWork(s.db.QueryRow(ctx, `SELECT `+workColumns+` FROM design_render_jobs WHERE space_id = ? AND project_id = ?
		AND board = ? AND kind = ? AND rev = ?`, j.SpaceID, j.Project, j.Board, j.Kind, j.Rev))
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, errors.New("designrender: the job was dropped as it was queued")
	}
	return w, err
}

// Last is the newest check or export of a project; false when there is none (none was asked for in a day).
func (s *Service) Last(ctx context.Context, spaceID, project, kind string) (Work, bool, error) {
	if s == nil {
		return Work{}, false, nil
	}
	w, err := scanWork(s.db.QueryRow(ctx, `SELECT `+workColumns+` FROM design_render_jobs WHERE space_id = ? AND project_id = ?
		AND kind = ? ORDER BY updated_ms DESC LIMIT 1`, spaceID, project, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, false, nil
	}
	return w, err == nil, err
}

// WorkOf is a check or an export of a project by its id; false when there is none.
func (s *Service) WorkOf(ctx context.Context, spaceID, project, kind, id string) (Work, bool, error) {
	if s == nil || id == "" {
		return Work{}, false, nil
	}
	w, err := scanWork(s.db.QueryRow(ctx, `SELECT `+workColumns+` FROM design_render_jobs WHERE id = ? AND space_id = ?
		AND project_id = ? AND kind = ?`, id, spaceID, project, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, false, nil
	}
	return w, err == nil, err
}

// OpenResult opens the result of a done check or export.
func (s *Service) OpenResult(ctx context.Context, spaceID, sha string) (io.ReadCloser, error) {
	if s == nil || sha == "" {
		return nil, os.ErrNotExist
	}
	room, err := s.thumbs(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return room.OpenBlob(ctx, sha)
}

// ServeBundle is GET BundlePath?id=<job>: the copy of the project of a claimed check or export.
func (s *Service) ServeBundle(w http.ResponseWriter, r *http.Request) error {
	if !s.authorized(r) {
		return httpx.New(http.StatusUnauthorized, "unauthorized", "Нужен токен воркера рендера")
	}
	if s.bundle == nil {
		return httpx.New(http.StatusNotFound, "not_found", "Копий проектов хаб не даёт")
	}
	ctx := r.Context()
	var spaceID, project, kind, state string
	var rev int64
	err := s.db.QueryRow(ctx, `SELECT space_id, project_id, kind, state, rev FROM design_render_jobs WHERE id = ?`,
		r.URL.Query().Get("id")).Scan(&spaceID, &project, &kind, &state, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.New(http.StatusNotFound, "not_found", "Задачи нет")
	}
	if err != nil {
		return err
	}
	if !isWork(kind) || state != "claimed" {
		return httpx.Conflict("not_claimed", "Задача не выдана воркеру")
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Cache-Control", "no-store")
	cw := &countingWriter{w: w}
	if err := s.bundle(ctx, spaceID, project, rev, cw); err != nil {
		if cw.n == 0 {
			return err
		}
		// Half sent: the worker's reading of the archive fails and it says so.
		s.log.Warn("designrender: a project's copy broke off", "project", project, "rev", rev, "error", err)
	}
	return nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// doneWork takes the result of a check (the check's folder, a gzipped tar) or of an export (its file, with the
// download's Content-Disposition in X-Render-Disposition).
func (s *Service) doneWork(ctx context.Context, w http.ResponseWriter, r *http.Request, id string, j Job) error {
	f, err := os.CreateTemp("", "render-result-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r.Body, MaxResultBytes+1))
	if err != nil {
		return err
	}
	if n == 0 || n > MaxResultBytes {
		return httpx.Invalid("body", "Нужен результат до 512 МБ")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var head [2]byte
	if _, err := io.ReadFull(f, head[:]); j.Kind == KindCheck && (err != nil || !bytes.Equal(head[:], []byte{0x1f, 0x8b})) {
		return httpx.Invalid("body", "Нужен архив проверки (gzip)")
	}
	disposition := ""
	if j.Kind == KindExport {
		disposition = r.Header.Get("X-Render-Disposition")
		if !strings.HasPrefix(disposition, "attachment") || len(disposition) > 1024 || strings.ContainsAny(disposition, "\r\n") {
			disposition = "attachment"
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	sha := hex.EncodeToString(h.Sum(nil))
	room, err := s.thumbs(ctx, j.SpaceID)
	if err != nil {
		return err
	}
	if err := room.PutBlob(ctx, sha, f); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE design_render_jobs SET result = ?, disposition = ? WHERE id = ?`, sha, disposition, id); err != nil {
		return err
	}
	return s.finish(ctx, w, id, "done", "")
}
