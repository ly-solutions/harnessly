package designview

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/store"
	"github.com/ly-solutions/harnessly/internal/hub/storehub"
)

// sqlDerived is the kept answers of the hub: the rows in its database (design_derived), the blobs in the spaces'
// `files` collections of its store.
type sqlDerived struct {
	db *store.DB
	st *storehub.Store
}

// derivedKept is how many revisions of a project keep their answers; older ones are dropped as new ones come.
const derivedKept = 8

func (d sqlDerived) Lookup(ctx context.Context, key string) (DerivedAnswer, bool) {
	a := DerivedAnswer{Key: key}
	var immutable int
	var headers string
	err := d.db.QueryRow(ctx, `SELECT space_id, project, rev, sha256, size, type, immutable, headers FROM design_derived WHERE key = ?`, key).
		Scan(&a.SpaceID, &a.Project, &a.Rev, &a.Sha, &a.Size, &a.Type, &immutable, &headers)
	if err != nil || !filestore.ValidSha(a.Sha) {
		return a, false
	}
	a.Immutable = immutable != 0
	if headers != "" && json.Unmarshal([]byte(headers), &a.Headers) != nil {
		return a, false
	}
	return a, true
}

func (d sqlDerived) Keep(ctx context.Context, a DerivedAnswer, body []byte) error {
	if err := d.st.PutBlob(ctx, a.SpaceID, filestore.KindFiles, a.Sha, bytes.NewReader(body)); err != nil {
		return err
	}
	headers, err := json.Marshal(a.Headers)
	if err != nil {
		return err
	}
	immutable := 0
	if a.Immutable {
		immutable = 1
	}
	if _, err := d.db.Exec(ctx, `INSERT INTO design_derived (key, space_id, project, rev, sha256, size, type, immutable, headers, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET sha256 = excluded.sha256, size = excluded.size, type = excluded.type, immutable = excluded.immutable,
			headers = excluded.headers, created_at = excluded.created_at`,
		a.Key, a.SpaceID, a.Project, a.Rev, a.Sha, a.Size, a.Type, immutable, string(headers), time.Now().UnixMilli()); err != nil {
		return err
	}
	// The answers of the revisions long passed go: a link pinned to one of them works its answers out again.
	_, err = d.db.Exec(ctx, `DELETE FROM design_derived WHERE space_id = ? AND project = ? AND rev < (
		SELECT COALESCE(MIN(rev), 0) FROM (SELECT DISTINCT rev FROM design_derived WHERE space_id = ? AND project = ? ORDER BY rev DESC LIMIT ?) kept)`,
		a.SpaceID, a.Project, a.SpaceID, a.Project, derivedKept)
	return err
}

func (d sqlDerived) Open(ctx context.Context, spaceID, sha string) (io.ReadCloser, error) {
	return d.st.OpenBlob(ctx, spaceID, filestore.KindFiles, sha)
}
