package filestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Two callers of one room, each seeing its own folder: a hash alone neither reads the other's file nor puts
// it into the caller's folder, while the sync of one's own files goes on as before.
func TestHashAloneGivesNothing(t *testing.T) {
	eachCombo(t, func(t *testing.T, r *rig) {
		srv := NewServer(r.meta, r.blobs, func(req *http.Request, _ string) (Access, error) {
			who := req.Header.Get("X-Who")
			// Both machines carry one name: what a caller sent is kept under its Holder.
			return Access{Member: "laptop", Holder: who, Room: "design",
				Sees: func(p string) bool { return strings.HasPrefix(p, who+"/") }}, nil
		})
		h := httptest.NewServer(srv.Handler())
		t.Cleanup(h.Close)
		do := func(who, method, path string, body []byte) (int, []byte) {
			req, err := http.NewRequest(method, h.URL+"/v1/collections/design"+path, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Who", who)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			out, _ := io.ReadAll(res.Body)
			return res.StatusCode, out
		}
		want := func(what string, code, expected int) {
			t.Helper()
			if code != expected {
				t.Fatalf("%s: %d, want %d", what, code, expected)
			}
		}
		secret := []byte("only anna's")
		sum := sha256.Sum256(secret)
		sha := hex.EncodeToString(sum[:])
		commit := func(who, path string) int {
			body, _ := json.Marshal(CommitRequest{Path: path, Sha: sha, Size: int64(len(secret))})
			code, _ := do(who, "POST", "/commit", body)
			return code
		}
		code, _ := do("anna", "PUT", "/blobs/"+sha, secret)
		want("anna sends", code, 204)
		want("anna commits", commit("anna", "anna/board.dc.html"), 200)
		want("anna renames", commit("anna", "anna/copy.dc.html"), 200)

		code, _ = do("boris", "GET", "/blobs/"+sha, nil)
		want("boris reads by hash", code, 404)
		code, _ = do("boris", "HEAD", "/blobs/"+sha, nil)
		want("boris asks by hash", code, 404)
		ask, _ := json.Marshal(MissingBlobsRequest{Shas: []string{sha}})
		_, out := do("boris", "POST", "/blobs/missing", ask)
		var missing MissingBlobsResponse
		if err := json.Unmarshal(out, &missing); err != nil || len(missing.Missing) != 1 {
			t.Fatalf("boris asks in a batch: %s", out)
		}
		want("boris takes it into his folder", commit("boris", "boris/stolen.dc.html"), 428)
		code, _ = do("boris", "PUT", "/blobs/"+sha, []byte("a guess"))
		want("boris sends something else under the hash", code, 400)
		want("a wrong body proves nothing", commit("boris", "boris/stolen.dc.html"), 428)
		open, _ := json.Marshal(UploadRequest{Sha: sha, Size: int64(len(secret))})
		_, out = do("boris", "POST", "/uploads", open)
		var state UploadState
		if err := json.Unmarshal(out, &state); err != nil || state.Done {
			t.Fatalf("an upload of another's contents is not done: %s", out)
		}

		// Who has the contents may of course keep them: sending them is the proof.
		code, _ = do("boris", "PUT", "/blobs/"+sha, secret)
		want("boris sends the contents", code, 204)
		want("boris commits his own", commit("boris", "boris/own.dc.html"), 200)
		code, out = do("boris", "GET", "/blobs/"+sha, nil)
		want("boris reads his own", code, 200)
		if !bytes.Equal(out, secret) {
			t.Fatalf("read back %q", out)
		}
	})
}
