package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/hubtest"
)

// shopFiles is what newDesignRuntime keeps in the machine's folder for the project "shop", under the same words.
var shopFiles = map[string]string{
	"artifact.json":          `{"title":"Shop"}`,
	"project/Main.dc.html":   `<html><body>shop main</body></html>`,
	"project/Secret.dc.html": `<html><body>shop secret</body></html>`,
}

// putStore commits files of a project into the hub's store of the space, as a machine's sync does. HRN-1755: a guest
// link reads the store and nothing else, so a project a test opens as a guest has to be there first. It returns the
// revision of the last file.
func putStore(t *testing.T, s *hubtest.Stack, space, project string, files map[string]string) int64 {
	t.Helper()
	ctx := context.Background()
	room, err := s.Hub.Store.DesignRoom(ctx, space)
	if err != nil {
		t.Fatal(err)
	}
	var rev int64
	for name, body := range files {
		path := project + "/" + name
		sum := sha256.Sum256([]byte(body))
		sha := hex.EncodeToString(sum[:])
		if err := room.PutBlob(ctx, sha, bytes.NewReader([]byte(body))); err != nil {
			t.Fatal(err)
		}
		var base int64
		if cur, err := room.Current(ctx, path); err == nil && cur != nil {
			base = cur.Rev
		}
		e, conflict, err := room.Commit(ctx, filestore.CommitRequest{Path: path, Base: base, Sha: sha, Size: int64(len(body)), Author: "owner"}, "vm-owner")
		if err != nil || conflict {
			t.Fatalf("commit %s: %v %v", path, err, conflict)
		}
		rev = e.Rev
	}
	return rev
}
