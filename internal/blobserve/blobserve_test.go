package blobserve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const sha = "9f86d081884c7d659a2feae0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

type opened struct{ n int }

func (o *opened) blob(body string, seek bool) Blob {
	return Blob{Sha: sha, Open: func(context.Context) (io.ReadCloser, error) {
		o.n++
		if seek {
			return struct {
				io.ReadSeeker
				io.Closer
			}{bytes.NewReader([]byte(body)), io.NopCloser(nil)}, nil
		}
		return io.NopCloser(strings.NewReader(body)), nil
	}}
}

func ask(t *testing.T, b Blob, p Policy, method string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/x", nil)
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	if err := Serve(w, r, b, p); err != nil {
		t.Fatal(err)
	}
	return w
}

// The hash is the validator: an address that names its contents is kept for good, a living one is asked again and
// answered 304 without the store being opened.
func TestTheHashIsTheValidator(t *testing.T) {
	o := &opened{}
	w := ask(t, o.blob("body", false), Policy{Type: "image/png", Cache: Immutable}, "GET")
	if w.Code != 200 || w.Body.String() != "body" || w.Header().Get("ETag") != `"`+sha+`"` ||
		w.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" || w.Header().Get("Content-Type") != "image/png" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("an immutable blob: %d %v", w.Code, w.Header())
	}
	w = ask(t, o.blob("body", false), Policy{Cache: Revalidate, Public: true}, "GET")
	if w.Header().Get("Cache-Control") != "public, no-cache" || w.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("a living address: %v", w.Header())
	}
	before := o.n
	for _, match := range []string{`"` + sha + `"`, `W/"` + sha + `"`, `"other", "` + sha + `"`, "*"} {
		w = ask(t, o.blob("body", false), Policy{Cache: Revalidate}, "GET", "If-None-Match", match)
		if w.Code != 304 || w.Body.Len() != 0 || w.Header().Get("ETag") == "" {
			t.Fatalf("If-None-Match %s: %d", match, w.Code)
		}
	}
	if w = ask(t, o.blob("body", false), Policy{}, "HEAD"); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatalf("HEAD: %d %q", w.Code, w.Body.String())
	}
	if o.n != before {
		t.Fatalf("the store was opened %d times for answers without a body", o.n-before)
	}
	if w = ask(t, o.blob("body", false), Policy{Cache: Revalidate}, "GET", "If-None-Match", `"other"`); w.Code != 200 {
		t.Fatalf("another validator: %d", w.Code)
	}
}

// An answer that is not kept carries no validator and is never 304; a download says its name.
func TestNoStoreAndDisposition(t *testing.T) {
	o := &opened{}
	w := ask(t, o.blob("page", false), Policy{Type: "text/html; charset=utf-8", Cache: NoStore, CSP: "sandbox", NoIndex: true}, "GET", "If-None-Match", `"`+sha+`"`)
	if w.Code != 200 || w.Header().Get("ETag") != "" || w.Header().Get("Cache-Control") != "private, no-store" ||
		w.Header().Get("Content-Security-Policy") != "sandbox" || w.Header().Get("X-Robots-Tag") == "" {
		t.Fatalf("a page that is not kept: %d %v", w.Code, w.Header())
	}
	w = ask(t, o.blob("x", false), Policy{Disposition: "attachment", Name: "отчёт.pdf"}, "GET")
	if d := w.Header().Get("Content-Disposition"); !strings.HasPrefix(d, "attachment; filename") {
		t.Fatalf("disposition: %q", d)
	}
	if w = ask(t, o.blob("x", false), Policy{Disposition: "inline"}, "GET"); w.Header().Get("Content-Disposition") != "inline" {
		t.Fatalf("inline: %v", w.Header())
	}
}

// A store that can seek answers ranges; a missing blob or a failing store writes nothing, the caller answers.
func TestRangesAndErrors(t *testing.T) {
	o := &opened{}
	w := ask(t, o.blob("0123456789", true), Policy{Cache: Immutable}, "GET", "Range", "bytes=2-4")
	if w.Code != 206 || w.Body.String() != "234" {
		t.Fatalf("a range: %d %q", w.Code, w.Body.String())
	}
	for _, c := range []struct {
		err  error
		want error
	}{{os.ErrNotExist, ErrNotFound}, {io.ErrUnexpectedEOF, io.ErrUnexpectedEOF}} {
		rec := httptest.NewRecorder()
		err := Serve(rec, httptest.NewRequest("GET", "/x", nil), Blob{Sha: sha, Open: func(context.Context) (io.ReadCloser, error) { return nil, c.err }}, Policy{Cache: Immutable})
		if !errors.Is(err, c.want) || len(rec.Header()) != 0 || rec.Body.Len() != 0 {
			t.Fatalf("%v: %v, headers %v", c.err, err, rec.Header())
		}
	}
}

func TestTypeByName(t *testing.T) {
	for name, want := range map[string]string{"a.HTML": "text/html; charset=utf-8", "f/x.woff2": "font/woff2", "x.unknown": "", "noext": ""} {
		if got := TypeByName(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}
