package designaccess

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ly-solutions/harnessly/internal/hub/link"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// workQueue is the hub's render worker for checks and exports: it keeps the jobs and their results in memory.
type workQueue struct {
	mu      sync.Mutex
	jobs    []Work
	kinds   []string
	asked   []RenderJob
	results map[string][]byte
}

func (q *workQueue) WantThumb(context.Context, RenderJob) error { return nil }
func (q *workQueue) Works() bool                                { return true }

func (q *workQueue) RunWork(_ context.Context, kind string, j RenderJob) (Work, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.asked = append(q.asked, j)
	for i, wk := range q.jobs {
		if q.kinds[i] == kind && wk.Args == j.Board {
			return wk, nil
		}
	}
	wk := Work{ID: strings.Repeat("a", 23) + strconv.Itoa(len(q.jobs)), Args: j.Board, State: "queued", Created: 1_764_000_000_000}
	q.jobs, q.kinds = append(q.jobs, wk), append(q.kinds, kind)
	return wk, nil
}

func (q *workQueue) LastWork(_ context.Context, _, _ string, kind string) (Work, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := len(q.jobs) - 1; i >= 0; i-- {
		if q.kinds[i] == kind {
			return q.jobs[i], true, nil
		}
	}
	return Work{}, false, nil
}

func (q *workQueue) WorkOf(_ context.Context, _, _ string, kind, id string) (Work, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, wk := range q.jobs {
		if q.kinds[i] == kind && wk.ID == id {
			return wk, true, nil
		}
	}
	return Work{}, false, nil
}

func (q *workQueue) OpenResult(_ context.Context, _, sha string) (io.ReadCloser, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	b, ok := q.results[sha]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// finish is the worker's answer: the job done with its result, or failed.
func (q *workQueue) finish(id string, result []byte, disposition, why string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.jobs {
		if q.jobs[i].ID != id {
			continue
		}
		q.jobs[i].Updated = q.jobs[i].Created + 5000
		if why != "" {
			q.jobs[i].State, q.jobs[i].Error = "failed", why
			return
		}
		sha := "sha-" + id
		q.results[sha] = result
		q.jobs[i].State, q.jobs[i].Result, q.jobs[i].Disposition = "done", sha, disposition
	}
}

func checkTar(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(tw, body)
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}

// with the owner's machine off, the board check and the exports run on the hub's render worker, from the
// store, and the page reads them on the machine's own routes; a machine that is on answers them itself.
func TestMachineOffChecksAndExportsOnTheWorker(t *testing.T) {
	r, _, _ := offRig(t)
	q := &workQueue{results: map[string][]byte{}}
	r.svc.UseRenderQueue(q)
	ask := func(u store.User, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/m/"+r.owner.ID+"/api/platform/design/"+path, strings.NewReader(body))
		w := httptest.NewRecorder()
		if !r.svc.ServeMachine(w, req, u, r.owner) {
			t.Fatalf("%s %s was left to the machine", method, path)
		}
		return w
	}

	// The check: an editor starts it, a viewer reads it, a viewer may not start it.
	if w := ask(r.strangeU, "POST", "library/check", `{"artifact":"plan"}`); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer started a check: %d %s", w.Code, w.Body)
	}
	if w := ask(r.mateU, "GET", "library/check?artifact=plan", ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"report":null}` {
		t.Fatalf("no check yet: %d %s", w.Code, w.Body)
	}
	w := ask(r.mateU, "POST", "library/check", `{"artifact":"plan","boards":["b.html","a.html"]}`)
	if w.Code != http.StatusAccepted || w.Header().Get("X-Harnessly-From") != "store" || !strings.Contains(w.Body.String(), `"finishedAt":null`) {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
	if j := q.asked[0]; j.Board != "a.html\nb.html" || j.Rev != 4 || j.Prefix != "/api/platform" || j.OwnerID != r.owner.CreatedBy || j.MachineID != r.owner.ID {
		t.Fatalf("check job: %+v", j)
	}
	report := `{"artifact":"plan","finishedAt":1,"done":2,"total":2,"boards":[{"path":"a.html"}]}`
	q.finish(q.jobs[0].ID, checkTar(t, map[string]string{"report.json": report, "000-desktop.jpg": "jpeg"}), "", "")
	if w := ask(r.strangeU, "GET", "library/check?artifact=plan", ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"report":`+report+`}` {
		t.Fatalf("done check: %d %s", w.Code, w.Body)
	}
	if w := ask(r.strangeU, "GET", "library/shot?artifact=plan&file=000-desktop.jpg&v=1", ""); w.Code != http.StatusOK || w.Body.String() != "jpeg" ||
		w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("shot: %d %s", w.Code, w.Body)
	}
	for _, file := range []string{"report.json", "../x.jpg", "001-desktop.jpg"} {
		if w := ask(r.strangeU, "GET", "library/shot?artifact=plan&file="+file, ""); w.Code != http.StatusNotFound {
			t.Fatalf("shot %s: %d", file, w.Code)
		}
	}

	// The exports: a picture of the deck for a viewer, its files for an editor; the PDF for guests needs the machine.
	if w := ask(r.strangeU, "POST", "deck/export", `{"artifact":"plan","format":"zip"}`); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer took the files: %d %s", w.Code, w.Body)
	}
	if w := ask(r.mateU, "POST", "deck/export", `{"artifact":"plan","format":"pdf","publish":true}`); w.Code != http.StatusConflict {
		t.Fatalf("publish with the machine off: %d %s", w.Code, w.Body)
	}
	for _, bad := range []string{`{"artifact":"plan","format":"exe"}`, `{"artifact":"plan","format":"pptx","board":"a.html"}`,
		`{"artifact":"plan","format":"pdf","rec":"../x"}`} {
		if w := ask(r.mateU, "POST", "deck/export", bad); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", bad, w.Code, w.Body)
		}
	}
	w = ask(r.strangeU, "POST", "deck/export", `{"artifact":"plan","format":"png","board":"a.html"}`)
	var job struct {
		ID, Format, State, At string
		Progress              float64
		File, Error           *string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil || w.Code != http.StatusAccepted || job.State != "queued" ||
		job.Format != "png" || job.File != nil || job.At == "" {
		t.Fatalf("export: %d %s", w.Code, w.Body)
	}
	if args := q.jobs[1].Args; args != "png\n\na.html" {
		t.Fatalf("export args %q", args)
	}
	q.finish(job.ID, []byte("PNG"), `attachment; filename="plan - A.png"`, "")
	w = ask(r.strangeU, "GET", "deck/export?artifact=plan&job="+job.ID, "")
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil || job.State != "done" || job.Progress != 1 || job.File == nil {
		t.Fatalf("export done: %d %s", w.Code, w.Body)
	}
	// The file is the hub's even when the machine came back meanwhile.
	r.svc.reg = link.NewRegistry()
	r.svc.reg.Add(&link.Link{MachineID: r.owner.ID})
	w = ask(r.strangeU, "GET", "deck/export/file?artifact=plan&job="+job.ID, "")
	if w.Code != http.StatusOK || w.Body.String() != "PNG" || w.Header().Get("Content-Type") != "image/png" ||
		w.Header().Get("Content-Disposition") != `attachment; filename="plan - A.png"` {
		t.Fatalf("file: %d %v %s", w.Code, w.Header(), w.Body)
	}
	if w := ask(r.strangeU, "GET", "deck/export?artifact=plan&job="+strings.Repeat("b", 24), ""); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown export: %d", w.Code)
	}
	// Anything else with the machine on is the machine's.
	req := httptest.NewRequest("POST", "/m/"+r.owner.ID+"/api/platform/design/library/check", strings.NewReader(`{"artifact":"plan"}`))
	if r.svc.serveWork(httptest.NewRecorder(), req, r.mateU, r.owner) {
		t.Fatal("the hub ran a check of a machine that is on")
	}

	// A check that failed on the worker says why, as a finished report.
	r.svc.reg = link.NewRegistry()
	ask(r.mateU, "POST", "library/check", `{"artifact":"plan"}`)
	q.finish(q.jobs[2].ID, nil, "", "Chrome упал")
	if w := ask(r.mateU, "GET", "library/check?artifact=plan", ""); !strings.Contains(w.Body.String(), `"failure":"Chrome упал"`) ||
		strings.Contains(w.Body.String(), `"finishedAt":null`) {
		t.Fatalf("failed check: %s", w.Body)
	}
}
