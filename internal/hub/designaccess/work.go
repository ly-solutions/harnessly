package designaccess

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// The board check and the exports of a project whose machine is off (docs/hub/render-worker.md): the hub
// queues them for its render worker, which runs the machine's own check or export on a copy of the project from the
// store, and answers the page's routes the way the machine does. A machine that is on answers them itself, as before.

// Work is a check or an export the render worker runs (designrender.Work).
type Work struct {
	ID          string
	Args        string // the job's arguments: the boards of a check, "<format>\n<recording>\n<board>" of an export
	State       string // queued, claimed, done or failed
	Error       string
	Result      string // the sha of the result, when done
	Disposition string
	Created     int64 // unix ms
	Updated     int64 // unix ms
}

// WorkQueue is the part of a RenderQueue that runs checks and exports (designrender.Service).
type WorkQueue interface {
	Works() bool
	RunWork(ctx context.Context, kind string, j RenderJob) (Work, error)
	LastWork(ctx context.Context, spaceID, project, kind string) (Work, bool, error)
	WorkOf(ctx context.Context, spaceID, project, kind, id string) (Work, bool, error)
	OpenResult(ctx context.Context, spaceID, sha string) (io.ReadCloser, error)
}

// The kinds of work (designrender.KindCheck, designrender.KindExport).
const (
	WorkCheck  = "check"
	WorkExport = "export"
)

var workPath = regexp.MustCompile(`^(/api/[A-Za-z0-9_-]+)/design/(library/check|library/shot|deck/export|deck/export/file)$`)

// hubJobRe is the id of a job of the hub's queue (designrender: 12 random bytes in hex); the machine's are other.
var hubJobRe = regexp.MustCompile(`^[0-9a-f]{24}$`)

// exportFormats are the machine's export formats (design.DeckExportFormats); ofBoard those made of one board.
var (
	exportFormats = map[string]bool{"pdf": true, "pptx": true, "mp4": true, "png": true, "html": true, "zip": true, "bundle": true}
	ofBoard       = map[string]bool{"pdf": true, "html": true, "png": true}
)

// maxCheckBoards caps the boards of one check (as the machine reads them).
const maxCheckBoards = 400

func (s *Service) serveWork(w http.ResponseWriter, r *http.Request, u store.User, m store.Machine) bool {
	mm := workPath.FindStringSubmatch(strings.TrimPrefix(r.URL.EscapedPath(), "/m/"+m.ID))
	if mm == nil {
		return false
	}
	prefix, route := mm[1], mm[2]
	post := r.Method == http.MethodPost
	if r.Method != http.MethodGet && !(post && (route == "library/check" || route == "deck/export")) {
		return false
	}
	q, ok := s.render.(WorkQueue)
	if !ok || !q.Works() {
		return false
	}
	ctx := r.Context()
	query := r.URL.Query()
	job := query.Get("job")
	// An export the hub ran is the hub's to answer, whether the machine came back meanwhile or not.
	hubJob := !post && (route == "deck/export" || route == "deck/export/file") && hubJobRe.MatchString(job)
	if !hubJob && (query.Get("from") != "store" && s.machineUp(m.ID) || !s.machineOff(ctx, m)) {
		return false
	}
	var body struct {
		Artifact string   `json:"artifact"`
		Boards   []string `json:"boards"`
		Format   string   `json:"format"`
		Rec      string   `json:"rec"`
		Board    string   `json:"board"`
		Publish  bool     `json:"publish"`
	}
	if post {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
		if err != nil || len(raw) > 64<<10 || json.Unmarshal(raw, &body) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный запрос"})
			return true
		}
	} else {
		body.Artifact = query.Get("artifact")
	}
	project := body.Artifact
	if !projectIDRe.MatchString(project) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неизвестный макет"})
		return true
	}
	t, err := s.read(ctx, m)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	p, stored, err := t.effective(project)
	if err != nil || !stored {
		return false
	}
	f, err := t.fold(p, u.Email)
	if err != nil {
		s.writeError(w, err)
		return true
	}
	need := roleView
	if post && route == "library/check" {
		need = roleEdit
	}
	if post && route == "deck/export" && (body.Format == "zip" || body.Format == "bundle") {
		need = roleEdit // an archive hands over the project's files, not what it shows
	}
	if f.Role == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Макета больше нет"})
		return true
	}
	if rank(f.Role) < rank(need) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Нет доступа к проекту", "code": "no_access", "role": f.Role})
		return true
	}
	w.Header().Set("X-Harnessly-From", "store")
	j := RenderJob{SpaceID: m.SpaceID, MachineID: m.ID, OwnerID: m.CreatedBy, Prefix: prefix, Project: project}
	switch route {
	case "library/check":
		if post {
			s.startCheck(w, r, q, j, body.Boards)
		} else {
			s.lastCheck(w, r, q, j)
		}
	case "library/shot":
		s.checkShot(w, r, q, j, query.Get("file"))
	case "deck/export":
		if post {
			s.startExport(w, r, q, j, body.Format, body.Rec, body.Board, body.Publish)
		} else {
			s.exportJob(w, r, q, j, job)
		}
	case "deck/export/file":
		s.exportFile(w, r, q, j, job)
	}
	return true
}

// runningReport is a check the worker has not finished, as the machine's report of a running check reads.
func runningReport(project string, wk Work) map[string]any {
	return map[string]any{"artifact": project, "startedAt": wk.Created, "finishedAt": nil, "done": 0, "total": 0,
		"boards": []any{}, "cover": nil, "failure": nil}
}

func (s *Service) startCheck(w http.ResponseWriter, r *http.Request, q WorkQueue, j RenderJob, boards []string) {
	if len(boards) > maxCheckBoards {
		boards = boards[:maxCheckBoards]
	}
	clean := make([]string, 0, len(boards))
	for _, b := range boards {
		if b = strings.TrimSpace(b); b != "" && len(b) <= 256 && !strings.ContainsAny(b, "\r\n") {
			clean = append(clean, b)
		}
	}
	sort.Strings(clean)
	j.Board = strings.Join(clean, "\n")
	if j.Board == "" || len(j.Board) > 512 { // designrender.MaxBoardLen
		j.Board = "*"
	}
	at, err := s.view.Head(r.Context(), j.SpaceID, j.Project)
	if err != nil {
		s.writeError(w, err)
		return
	}
	j.Rev = at
	wk, err := q.RunWork(r.Context(), WorkCheck, j)
	if err != nil {
		s.workError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"report": checkReport(r.Context(), q, j, wk, s)})
}

func (s *Service) lastCheck(w http.ResponseWriter, r *http.Request, q WorkQueue, j RenderJob) {
	wk, ok, err := q.LastWork(r.Context(), j.SpaceID, j.Project, WorkCheck)
	if err != nil {
		s.workError(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"report": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": checkReport(r.Context(), q, j, wk, s)})
}

// checkReport is the report of a check of the hub's queue: the worker's own when done.
func checkReport(ctx context.Context, q WorkQueue, j RenderJob, wk Work, s *Service) any {
	switch wk.State {
	case "done":
		var report json.RawMessage
		err := readCheckFile(ctx, q, j.SpaceID, wk.Result, "report.json", func(rd io.Reader) error {
			raw, err := io.ReadAll(io.LimitReader(rd, 8<<20))
			if err == nil && json.Valid(raw) {
				report = raw
			}
			return err
		})
		if err == nil && report != nil {
			return report
		}
		s.log.Warn("design check: the worker's report was not read", "project", j.Project, "error", err)
		failed := runningReport(j.Project, wk)
		failed["finishedAt"], failed["failure"] = wk.Updated, "Отчёт проверки не прочитан"
		return failed
	case "failed":
		failed := runningReport(j.Project, wk)
		why := wk.Error
		if why == "" {
			why = "Проверка не прошла"
		}
		failed["finishedAt"], failed["failure"] = wk.Updated, why
		return failed
	}
	return runningReport(j.Project, wk)
}

// readCheckFile finds a file of the check's folder in its result (a gzipped tar) and reads it.
func readCheckFile(ctx context.Context, q WorkQueue, spaceID, sha, name string, read func(io.Reader) error) error {
	rc, err := q.OpenResult(ctx, spaceID, sha)
	if err != nil {
		return err
	}
	defer rc.Close()
	gz, err := gzip.NewReader(rc)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errNoFile
			}
			return err
		}
		if h.Typeflag == tar.TypeReg && path.Clean(h.Name) == name {
			return read(tr)
		}
	}
}

var errNoFile = errors.New("designaccess: no such file in the check")

func (s *Service) checkShot(w http.ResponseWriter, r *http.Request, q WorkQueue, j RenderJob, file string) {
	if !shotFileRe.MatchString(file) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	wk, ok, err := q.LastWork(r.Context(), j.SpaceID, j.Project, WorkCheck)
	if err != nil || !ok || wk.State != "done" {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err = readCheckFile(r.Context(), q, j.SpaceID, wk.Result, file, func(rd io.Reader) error {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, err := io.Copy(w, io.LimitReader(rd, 32<<20))
		return err
	})
	if errors.Is(err, errNoFile) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
	} else if err != nil {
		s.log.Warn("design check: a shot was not read", "project", j.Project, "error", err)
	}
}

func (s *Service) startExport(w http.ResponseWriter, r *http.Request, q WorkQueue, j RenderJob, format, rec, board string, publish bool) {
	bad := func(msg string) { writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg}) }
	if !exportFormats[format] {
		bad("Неизвестный формат")
		return
	}
	if rec != "" && !deckIDRe.MatchString(rec) {
		bad("Неизвестная запись")
		return
	}
	if board != "" && (!ofBoard[format] || len(board) > 256 || strings.ContainsAny(board, "\r\n")) {
		bad("Неизвестная доска")
		return
	}
	if publish {
		// The PDF for guests lives in the machine's folder of the project, beside its recordings, not in the store.
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PDF для гостей публикует машина: включите её", "code": "machine_off"})
		return
	}
	at, err := s.view.Head(r.Context(), j.SpaceID, j.Project)
	if err != nil {
		s.writeError(w, err)
		return
	}
	j.Rev, j.Board = at, format+"\n"+rec+"\n"+board
	wk, err := q.RunWork(r.Context(), WorkExport, j)
	if err != nil {
		s.workError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, exportValue(wk))
}

// shotFileRe is a screenshot of a check (design.shotFileRe).
var shotFileRe = regexp.MustCompile(`^\d{3}-(desktop|phone)\.jpg$`)

var deckIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// exportValue is an export of the hub's queue as the machine's export job reads (design.DeckExportJob).
func exportValue(wk Work) map[string]any {
	format, _, _ := strings.Cut(wk.Args, "\n")
	v := map[string]any{"id": wk.ID, "format": format, "state": "queued", "progress": 0, "file": nil, "error": nil,
		"at": time.UnixMilli(wk.Created).UTC().Format(time.RFC3339)}
	switch wk.State {
	case "claimed":
		v["state"] = "running"
	case "done":
		v["state"], v["progress"], v["file"] = "done", 1, wk.ID+"."+format
	case "failed":
		why := wk.Error
		if why == "" {
			why = "Экспорт не удался"
		}
		v["state"], v["error"] = "failed", why
	}
	return v
}

func (s *Service) exportJob(w http.ResponseWriter, r *http.Request, q WorkQueue, j RenderJob, id string) {
	wk, ok, err := q.WorkOf(r.Context(), j.SpaceID, j.Project, WorkExport, id)
	if err != nil {
		s.workError(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Такого экспорта нет: возможно, он старше суток"})
		return
	}
	writeJSON(w, http.StatusOK, exportValue(wk))
}

func (s *Service) exportFile(w http.ResponseWriter, r *http.Request, q WorkQueue, j RenderJob, id string) {
	wk, ok, err := q.WorkOf(r.Context(), j.SpaceID, j.Project, WorkExport, id)
	if err != nil || !ok || wk.State != "done" {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	rc, err := q.OpenResult(r.Context(), j.SpaceID, wk.Result)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer rc.Close()
	h := w.Header()
	h.Set("Content-Type", exportType(wk.Disposition))
	h.Set("Cache-Control", "private, no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Disposition", wk.Disposition)
	if _, err := io.Copy(w, rc); err != nil {
		s.log.Warn("design export: the file broke off", "project", j.Project, "error", err)
	}
}

// exportTypes are the types of the machine's export files (design.deckTypes), by the extension of the file's name.
var exportTypes = map[string]string{
	".pdf":  "application/pdf",
	".png":  "image/png",
	".zip":  "application/zip",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".html": "text/html; charset=utf-8",
	".mp4":  "video/mp4",
}

// exportType is the file's type after the name the worker gave it in the download's Content-Disposition.
func exportType(disposition string) string {
	if _, params, err := mime.ParseMediaType(disposition); err == nil {
		if t := exportTypes[strings.ToLower(path.Ext(params["filename"]))]; t != "" {
			return t
		}
	}
	return "application/octet-stream"
}

func (s *Service) workError(w http.ResponseWriter, err error) {
	s.log.Warn("design work: the hub's queue did not answer", "error", err)
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Хаб не смог поставить задачу воркеру", "code": "store_unavailable"})
}
