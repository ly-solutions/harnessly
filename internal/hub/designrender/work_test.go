package designrender

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/httpx"
)

func workSetup(t *testing.T) (*Service, *fakeRoom, http.Handler) {
	t.Helper()
	s, room, _ := setup(t)
	s.UseBundles(func(_ context.Context, spaceID, project string, rev int64, w io.Writer) error {
		_, err := io.WriteString(w, spaceID+"/"+project+"@"+string(rune('0'+rev)))
		return err
	})
	mux := http.NewServeMux()
	mux.Handle("POST "+ClaimPath, httpx.Handler(s.ServeClaim))
	mux.Handle("POST "+DonePath, httpx.Handler(s.ServeDone))
	mux.Handle("GET "+BundlePath, httpx.Handler(s.ServeBundle))
	return s, room, mux
}

func workJob(kind, args string) Job {
	return Job{SpaceID: "sp", MachineID: "m1", OwnerID: "u1", Prefix: "/api/x", Project: "p1", Board: args, Kind: kind, Rev: 3}
}

func claimed(t *testing.T, h http.Handler, query string) Job {
	t.Helper()
	rec := call(h, ClaimPath+query, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", rec.Code, rec.Body)
	}
	var j Job
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestCheckRunsOnTheProjectsCopy(t *testing.T) {
	s, room, h := workSetup(t)
	ctx := context.Background()
	if _, err := New(nil, nil, nil, token, nil, nil).Run(ctx, workJob(KindCheck, "*")); err == nil {
		t.Fatal("a hub without copies of projects queued a check")
	}
	wk, err := s.Run(ctx, workJob(KindCheck, "*"))
	if err != nil || wk.State != "queued" || wk.ID == "" {
		t.Fatalf("run: %+v %v", wk, err)
	}
	if again, _ := s.Run(ctx, workJob(KindCheck, "*")); again.ID != wk.ID {
		t.Fatal("the same check at the same revision was queued twice")
	}
	// A worker busy with a check or an export takes pictures only.
	short, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if j, err := s.claim(short, true); err != nil || j != nil {
		t.Fatalf("pictures only gave a check: %+v %v", j, err)
	}
	j := claimed(t, h, "")
	if j.Kind != KindCheck || j.Board != "*" || j.Path != "" || j.Project != "p1" {
		t.Fatalf("job: %+v", j)
	}
	// The copy is the worker's only: by its token, of a claimed job.
	req := httptest.NewRequest(http.MethodGet, BundlePath+"?id="+j.ID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("a copy without the token: %d", rec.Code)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "sp/p1@3" {
		t.Fatalf("copy: %d %q", rec.Code, rec.Body)
	}
	if rec := call(h, DonePath+"?id="+j.ID, []byte("not an archive"), token); rec.Code != http.StatusBadRequest {
		t.Fatalf("a check's result that is not gzip: %d", rec.Code)
	}
	result := append([]byte{0x1f, 0x8b}, bytes.Repeat([]byte{7}, 50)...)
	if rec := call(h, DonePath+"?id="+j.ID, result, token); rec.Code != http.StatusNoContent {
		t.Fatalf("done: %d %s", rec.Code, rec.Body)
	}
	last, ok, err := s.Last(ctx, "sp", "p1", KindCheck)
	if err != nil || !ok || last.State != "done" || last.Result == "" {
		t.Fatalf("last: %+v %v %v", last, ok, err)
	}
	rc, err := s.OpenResult(ctx, "sp", last.Result)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, result) || len(room.blobs) != 1 {
		t.Fatalf("stored result: %d bytes, %d blobs", len(got), len(room.blobs))
	}
	// A claimed job is no longer given its copy once done.
	req = httptest.NewRequest(http.MethodGet, BundlePath+"?id="+j.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("a copy of a done job: %d", rec.Code)
	}
}

func TestExportKeepsItsDispositionAndFailedRunsAgain(t *testing.T) {
	s, _, h := workSetup(t)
	ctx := context.Background()
	wk, err := s.Run(ctx, workJob(KindExport, "pdf\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	j := claimed(t, h, "")
	if j.ID != wk.ID || j.Kind != KindExport {
		t.Fatalf("job: %+v", j)
	}
	if rec := call(h, DonePath+"?id="+j.ID+"&error=no+chrome", nil, token); rec.Code != http.StatusNoContent {
		t.Fatalf("error: %d %s", rec.Code, rec.Body)
	}
	failed, ok, _ := s.WorkOf(ctx, "sp", "p1", KindExport, wk.ID)
	if !ok || failed.State != "failed" || failed.Error != "no chrome" {
		t.Fatalf("a work job that failed is not left failed at once: %+v", failed)
	}
	again, err := s.Run(ctx, workJob(KindExport, "pdf\n\n"))
	if err != nil || again.ID != wk.ID || again.State != "queued" || again.Error != "" {
		t.Fatalf("a failed export asked again: %+v %v", again, err)
	}
	j = claimed(t, h, "")
	req := httptest.NewRequest(http.MethodPost, DonePath+"?id="+j.ID, bytes.NewReader([]byte("%PDF-1.7 …")))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Render-Disposition", `attachment; filename="deck.pdf"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("done: %d %s", rec.Code, rec.Body)
	}
	done, ok, _ := s.WorkOf(ctx, "sp", "p1", KindExport, wk.ID)
	if !ok || done.State != "done" || done.Disposition != `attachment; filename="deck.pdf"` || done.Board != "pdf\n\n" {
		t.Fatalf("done: %+v", done)
	}
	if _, ok, _ := s.WorkOf(ctx, "other", "p1", KindExport, wk.ID); ok {
		t.Fatal("another space sees the export")
	}
}
