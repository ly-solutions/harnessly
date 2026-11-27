package renderworker

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ly-solutions/harnessly/internal/design"
)

// The board check and the exports of a project whose machine is off (designrender.KindCheck, designrender.KindExport):
// the worker takes a copy of the project from the hub's store, runs the machine's own check or export on it in a
// library of its own, and sends back the check's folder (a gzipped tar) or the export's file.

const (
	kindCheck  = "check"
	kindExport = "export"
	bundlePath = "/agent/render/bundle"
)

// Limits of a project's copy: what one project of the store may hold, with its kit.
const (
	maxBundleBytes = 2 << 30
	maxBundleFiles = 50000
)

// How long a check and an export may run: a little under the hub's lease of the job (designrender.leaseOf).
var (
	checkLimit  = 18 * time.Minute
	exportLimit = 43 * time.Minute
	workPoll    = time.Second
)

func isWork(kind string) bool { return kind == kindCheck || kind == kindExport }

// work runs a check or an export and sends its result or why there is none.
func (w *Worker) work(ctx context.Context, j job) {
	started := time.Now()
	limit := checkLimit
	if j.Kind == kindExport {
		limit = exportLimit
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	dir, err := os.MkdirTemp("", "render-work-")
	if err != nil {
		w.done(ctx, j, nil, err.Error())
		return
	}
	defer os.RemoveAll(dir)
	if err := w.bundle(ctx, j, dir); err != nil {
		w.Log.Warn("render-worker: the project's copy was not taken", "project", j.Project, "error", err)
		w.finishWork(ctx, j, nil, "", "Копия проекта не получена: "+err.Error())
		return
	}
	kits := filepath.Join(dir, "kits")
	if _, err := os.Stat(kits); err != nil {
		kits = ""
	}
	l := design.New(design.Config{LibraryDir: filepath.Join(dir, "library"), KitsDir: kits, DataDir: filepath.Join(dir, "data"),
		Passive: true})
	defer func() {
		l.StopPages()
		l.StopModes()
		l.StopServer()
	}()
	var file, disposition string
	if j.Kind == kindCheck {
		file, err = w.check(ctx, l, j, dir)
	} else {
		file, disposition, err = w.export(ctx, l, j)
	}
	if err != nil {
		w.Log.Warn("render-worker: a "+j.Kind+" failed", "project", j.Project, "error", err)
		w.finishWork(ctx, j, nil, "", err.Error())
		return
	}
	f, err := os.Open(file)
	if err != nil {
		w.finishWork(ctx, j, nil, "", err.Error())
		return
	}
	defer f.Close()
	w.finishWork(ctx, j, f, disposition, "")
	w.Log.Info("render-worker: ran a "+j.Kind, "project", j.Project, "rev", j.Rev, "ms", time.Since(started).Milliseconds())
}

// check runs the check and packs its folder.
func (w *Worker) check(ctx context.Context, l *design.Library, j job, dir string) (string, error) {
	var only []string
	if j.Board != "*" {
		only = strings.Split(j.Board, "\n")
	}
	if l.StartCheck(j.Project, only) == nil {
		return "", errors.New("Нет такого макета или такой доски")
	}
	for {
		if _, r := l.ReadCheckReport(j.Project); r != nil && r.FinishedAt != nil {
			break
		}
		select {
		case <-ctx.Done():
			return "", errors.New("Проверка шла слишком долго и была остановлена")
		case <-time.After(workPoll):
		}
	}
	out := filepath.Join(dir, "check.tar.gz")
	return out, packDir(l.CheckDir(j.Project), out)
}

// export runs the export and gives its file with the download's Content-Disposition.
func (w *Worker) export(ctx context.Context, l *design.Library, j job) (string, string, error) {
	parts := strings.SplitN(j.Board, "\n", 3)
	if len(parts) != 3 {
		return "", "", errors.New("Неверная задача экспорта")
	}
	started, err := l.StartBoardExport(j.Project, design.DeckExportFormat(parts[0]), parts[1], parts[2], false)
	if err != nil {
		return "", "", err
	}
	for {
		ej, _, ok := l.DeckExportJobOf(j.Project, started.ID)
		if !ok {
			return "", "", errors.New("Экспорт пропал")
		}
		switch ej.State {
		case design.DeckJobDone:
			d, ok := l.DeckExportDownload(j.Project, started.ID)
			if !ok {
				return "", "", errors.New("Файла экспорта нет")
			}
			return d.Path, d.Disposition, nil
		case design.DeckJobFailed:
			why := "Экспорт не удался"
			if ej.Error != nil {
				why = *ej.Error
			}
			return "", "", errors.New(why)
		}
		select {
		case <-ctx.Done():
			return "", "", errors.New("Экспорт шёл слишком долго и был остановлен")
		case <-time.After(workPoll):
		}
	}
}

// bundle takes the project's copy from the hub and lays it out under dir: library/<project>/… and kits/….
func (w *Worker) bundle(ctx context.Context, j job, dir string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.Hub+bundlePath+"?"+url.Values{"id": {j.ID}}.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)
	resp, err := w.long().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bundle: %s", resp.Status)
	}
	return unpack(resp.Body, dir, j.Project)
}

// unpack lays out a project's copy: regular files under library/<project>/ and kits/ only, nothing outside dir.
func unpack(r io.Reader, dir, project string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	var total int64
	files := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := h.Name
		if path.Clean(name) != name || path.IsAbs(name) || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") ||
			!(strings.HasPrefix(name, "library/"+project+"/") || strings.HasPrefix(name, "kits/")) {
			return fmt.Errorf("bundle: a bad path %q", name)
		}
		if files++; files > maxBundleFiles {
			return errors.New("bundle: too many files")
		}
		if total += h.Size; h.Size < 0 || total > maxBundleBytes {
			return errors.New("bundle: too big")
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if rel, err := filepath.Rel(dir, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("bundle: a bad path %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(tr, h.Size))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "library", project)); err != nil {
		return errors.New("bundle: no project in the copy")
	}
	return nil
}

// packDir packs the regular files right under dir (a check's folder) into a gzipped tar at out.
func packDir(dir, out string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if err := addFile(tw, filepath.Join(dir, e.Name()), e.Name()); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}

func addFile(tw *tar.Writer, file, name string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: st.Size(), ModTime: st.ModTime(), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// finishWork sends the result of a check or an export (a file), or why there is none.
func (w *Worker) finishWork(ctx context.Context, j job, body *os.File, disposition, why string) {
	q := url.Values{"id": {j.ID}}
	if why != "" {
		q.Set("error", line(why))
	}
	var rd io.Reader
	if body != nil {
		rd = body
	}
	// The job's own time may be up: the answer still goes, with a little time of its own.
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(sctx, http.MethodPost, w.Hub+donePath+"?"+q.Encode(), rd)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
		if disposition != "" {
			req.Header.Set("X-Render-Disposition", disposition)
		}
	}
	resp, err := w.long().Do(req)
	if err != nil {
		w.Log.Warn("render-worker: the hub did not take the answer", "project", j.Project, "error", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		w.Log.Warn("render-worker: the hub refused the answer", "project", j.Project, "status", resp.Status)
	}
}

// long is the client for a project's copy and a result, which may take minutes: its time is the request's context's.
func (w *Worker) long() *http.Client {
	c := *w.Client
	c.Timeout = 0
	return &c
}

// line is the first line of a message, at most 400 characters (what the page shows).
func line(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(s); len(r) > 400 {
		s = string(r[:400])
	}
	return s
}
