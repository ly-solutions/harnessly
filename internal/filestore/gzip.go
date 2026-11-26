package filestore

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// text travels compressed between a machine and the store, both ways. What the store keeps and what a blob's
// sha names are the bytes as they are; only the transfer is gzip. A side that does not know it gets plain bytes: a
// download is compressed only for a caller that asks, and a machine compresses an upload only after the store said it
// takes that.

const (
	gzipMin   = 1024
	gzipLevel = 5
)

// compressible tells by its first bytes whether contents are worth compressing: text, not pictures, video, fonts or
// archives.
func compressible(head []byte) bool {
	if len(head) < gzipMin {
		return false
	}
	t := http.DetectContentType(head)
	return strings.HasPrefix(t, "text/") || strings.Contains(t, "json") || strings.Contains(t, "xml")
}

// Gzip compresses contents worth compressing; false when they are not or would not get smaller.
func Gzip(data []byte) ([]byte, bool) {
	if !compressible(data) {
		return nil, false
	}
	var buf bytes.Buffer
	z, _ := gzip.NewWriterLevel(&buf, gzipLevel)
	if _, err := z.Write(data); err != nil || z.Close() != nil || buf.Len() >= len(data) {
		return nil, false
	}
	return buf.Bytes(), true
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// sendBlob writes a blob's bytes, compressed when the caller takes that and they are text.
func sendBlob(w http.ResponseWriter, r *http.Request, sha string, f io.Reader) {
	br := bufio.NewReaderSize(f, 4096)
	head, _ := br.Peek(gzipMin)
	w.Header().Set("Vary", "Accept-Encoding")
	if !acceptsGzip(r) || !compressible(head) {
		w.Header().Set("ETag", `"`+sha+`"`)
		_, _ = io.Copy(w, br)
		return
	}
	w.Header().Set("ETag", `W/"`+sha+`"`)
	w.Header().Set("Content-Encoding", "gzip")
	z, _ := gzip.NewWriterLevel(w, gzipLevel)
	if _, err := io.Copy(z, br); err != nil {
		// Left without its end: the caller sees a broken stream, not a short file.
		return
	}
	_ = z.Close()
}

// blobBody is the upload's bytes as they are, at most limit of them, whether they came compressed or not.
func blobBody(w http.ResponseWriter, r *http.Request, limit int64) (io.Reader, error) {
	body := http.MaxBytesReader(w, r.Body, limit)
	if r.Header.Get("Content-Encoding") != "gzip" {
		return body, nil
	}
	z, err := gzip.NewReader(body)
	if err != nil {
		return nil, err
	}
	return http.MaxBytesReader(w, z, limit), nil
}
