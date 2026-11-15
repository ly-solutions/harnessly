package proxy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ly-solutions/harnessly/internal/hub/metrics"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ly-solutions/harnessly/internal/blobserve"
	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/auth"
	"github.com/ly-solutions/harnessly/internal/hub/history"
	"github.com/ly-solutions/harnessly/internal/hub/httpx"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// The workspace's uploads in the hub's store (docs/hub/history.md): what a person puts into a chat goes to the
// machine as before, and the page puts the same bytes into the `uploads` collection of the machine's space, by their
// sha256, with the id the machine gave them. The chat's pictures and files are then read from the hub, with the
// machine on or off; an upload the hub has not got (older than this, or put while it was off) is read from the machine.
//
//	PUT  <platform api>/uploads/blobs/<sha256>?machine=<id>         the bytes (they must hash to the name)
//	GET  <platform api>/uploads/blobs/<sha256>?machine=<id>&type=   the bytes
//	PUT  <platform api>/uploads/refs/<upload id>?machine=<id>       {sha256, mime, name, size}: what the machine's upload is
//	GET  <platform api>/uploads/files/<upload id>?machine=<id>      the upload from the hub, else a redirect to the machine
//	GET  <platform api>/uploads/delivered/<sha256>?machine=<id>&type=&name=&path=[&download=1]
//	                                                                a file the agent handed over, which the machine put
//	                                                                into the space's `files` collection; else a redirect
//	                                                                to the machine's file-preview of the path
//	GET  <platform api>/uploads/delivered/by-path?machine=<id>&path=   the same, found by its path in the person's turns
//	                                                                (the pane opens a handed-over file by its path)
//
// The person must be one who may open the machine; a blob is read only within the machine's space, and a ref only by
// the person who named it (a chat is its owner's, so is what its pictures are).

// Uploads is the store's side the workspace needs (storehub.Store).
type Uploads interface {
	HasBlob(ctx context.Context, spaceID, kind, sha string) (bool, error)
	OpenBlob(ctx context.Context, spaceID, kind, sha string) (io.ReadCloser, error)
	PutBlob(ctx context.Context, spaceID, kind, sha string, r io.Reader) error
}

// UseUploads lets the workspace keep uploads in the hub's store; nil (no store) leaves them to the machines.
func (s *Service) UseUploads(u Uploads) {
	s.mu.Lock()
	s.uploads = u
	s.mu.Unlock()
}

func (s *Service) uploadsStore() Uploads {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.uploads
}

var (
	uploadsPath = regexp.MustCompile(`^/api/[A-Za-z0-9_-]+/uploads/(blobs|refs|files|delivered)/([A-Za-z0-9_.-]{1,128})$`)
	uploadID    = regexp.MustCompile(`^up_[A-Z2-7]{4,64}$`)
	// inlineTypes are shown in the page as they are; anything else is a download, so nothing put here runs on the hub's
	// origin.
	inlineTypes = map[string]bool{
		"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/avif": true,
		"application/pdf": true, "text/plain": true, "audio/mpeg": true, "audio/ogg": true, "audio/wav": true,
		"video/mp4": true, "video/webm": true,
	}
)

type uploadRef struct {
	Sha256 string `json:"sha256"`
	Mime   string `json:"mime"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

func (s *Service) serveUploads(w http.ResponseWriter, r *http.Request, p *auth.Principal, rest string) {
	u := s.uploadsStore()
	match := uploadsPath.FindStringSubmatch(rest)
	if u == nil || match == nil {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	kind, name := match[1], match[2]
	id := r.URL.Query().Get("machine")
	if id == "" || id == "local" {
		id = s.home(r.Context(), p.User.ID, true)
	}
	if !machineID.MatchString(id) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	m, err := s.admit(r.Context(), p.User.ID, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	switch {
	case kind == "blobs" && r.Method == http.MethodPut:
		if !filestore.ValidSha(name) {
			httpx.WriteError(w, r, httpx.Invalid("sha256", "64 hex digits"))
			return
		}
		// The bytes are hashed here, all of them, whether the store had them already or not: only bytes that hash to the
		// name make the person their owner.
		h := sha256.New()
		body := io.TeeReader(http.MaxBytesReader(w, r.Body, filestore.MaxFileSize), h)
		if err := u.PutBlob(ctx, m.SpaceID, filestore.KindUploads, name, body); err != nil {
			if errors.Is(err, filestore.ErrBadRequest) {
				httpx.WriteError(w, r, httpx.Invalid("body", "the bytes do not hash to the name, or are too big"))
				return
			}
			s.fail(w, r, err)
			return
		}
		if _, err := io.Copy(io.Discard, body); err != nil || hex.EncodeToString(h.Sum(nil)) != name {
			httpx.WriteError(w, r, httpx.Invalid("body", "the bytes do not hash to the name, or are too big"))
			return
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO upload_owners (machine_id, sha256, owner_id, created_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (machine_id, sha256, owner_id) DO NOTHING`, m.ID, name, p.User.ID, store.Ms(s.now())); err != nil {
			s.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case kind == "blobs" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		// Only bytes the person put: the collection is the space's machines', not the person's.
		if !s.ownsUpload(ctx, m.ID, p.User.ID, name) {
			httpx.WriteError(w, r, httpx.ErrNotFound)
			return
		}
		s.sendBlob(w, r, u, m, filestore.KindUploads, name, r.URL.Query().Get("type"), r.URL.Query().Get("name"))
	case kind == "refs" && r.Method == http.MethodPut:
		var ref uploadRef
		if !uploadID.MatchString(name) || json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&ref) != nil || !filestore.ValidSha(ref.Sha256) || ref.Size < 0 {
			httpx.WriteError(w, r, httpx.Invalid("ref", "an upload id and {sha256, mime, name, size}"))
			return
		}
		// Only what is in the store already may be named: a ref never points at bytes the hub has not checked.
		if ok, err := u.HasBlob(ctx, m.SpaceID, filestore.KindUploads, ref.Sha256); err != nil || !ok || !s.ownsUpload(ctx, m.ID, p.User.ID, ref.Sha256) {
			httpx.WriteError(w, r, httpx.Invalid("sha256", "put the blob first"))
			return
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO upload_refs (machine_id, upload_id, owner_id, sha256, mime, name, size, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (machine_id, upload_id, owner_id) DO NOTHING`,
			m.ID, name, p.User.ID, ref.Sha256, clip(ref.Mime, 128), clip(ref.Name, 255), ref.Size, store.Ms(s.now())); err != nil {
			s.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case kind == "files" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		if !uploadID.MatchString(name) {
			httpx.WriteError(w, r, httpx.ErrNotFound)
			return
		}
		var ref uploadRef
		err := s.db.QueryRow(ctx, `SELECT sha256, mime, name, size FROM upload_refs WHERE machine_id = ? AND upload_id = ? AND owner_id = ?`, m.ID, name, p.User.ID).
			Scan(&ref.Sha256, &ref.Mime, &ref.Name, &ref.Size)
		if err == nil {
			if ok, _ := u.HasBlob(ctx, m.SpaceID, filestore.KindUploads, ref.Sha256); ok {
				metrics.CountRead(metrics.ReadUpload, metrics.FromStore)
				s.sendBlob(w, r, u, m, filestore.KindUploads, ref.Sha256, ref.Mime, ref.Name)
				return
			}
		}
		// an upload the machine's move put into the store, with no ref of the person's yet (it was moved before
		// the move bound uploads to their person): it is the machine's person's, so it is bound now and read from the hub.
		if errors.Is(err, sql.ErrNoRows) && p.User.ID == m.CreatedBy {
			if ref, ok := s.adoptMoved(ctx, u, m, p.User.ID, name); ok {
				metrics.CountRead(metrics.ReadUpload, metrics.FromStore)
				s.sendBlob(w, r, u, m, filestore.KindUploads, ref.Sha256, ref.Mime, ref.Name)
				return
			}
		}
		metrics.CountRead(metrics.ReadUpload, metrics.FromMachine)
		s.noteUploadMiss(ctx, u, m, p.User.ID, name)
		// Not in the hub (yet): the machine's own copy, the way the page read it before.
		target := s.cfg.BasePath + workspaceBase + "/api/v2/uploads/" + url.PathEscape(name) + "?machine=" + url.QueryEscape(m.ID)
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	case kind == "delivered" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		q := r.URL.Query()
		sha, typ, title := name, q.Get("type"), q.Get("name")
		// by-path: the pane opens a handed-over file by its path; the person's own turns say which bytes it is.
		if name == "by-path" {
			sha = ""
			if f, ok, _ := history.DeliveredAt(ctx, s.db, p.User.ID, m.ID, q.Get("path")); ok {
				sha, typ, title = f.Sha256, f.Mime, f.Name
			}
		}
		// Only a file the person's own chats show: the collection is the space's machines', not the person's. Anything
		// else is the machine's to answer, the same way as a file the hub has not got.
		mine, _ := history.Delivered(ctx, s.db, p.User.ID, m.ID, sha)
		if ok, _ := u.HasBlob(ctx, m.SpaceID, filestore.KindFiles, sha); mine && ok {
			if q.Get("download") == "1" {
				typ = "" // a download is never shown in the page
			}
			metrics.CountRead(metrics.ReadDelivered, metrics.FromStore)
			s.sendBlob(w, r, u, m, filestore.KindFiles, sha, typ, title)
			return
		}
		// Not in the hub (older, too big, or put while it was away): the machine's own preview of the file, where the
		// machine decides again whether the person may read it.
		path := q.Get("path")
		if !strings.HasPrefix(path, "/") || len(path) > 4096 {
			httpx.WriteError(w, r, httpx.ErrNotFound)
			return
		}
		target := url.Values{}
		if q.Get("download") == "1" {
			target.Set("download", "1")
		}
		target.Set("path", path)
		target.Set("machine", m.ID)
		metrics.CountRead(metrics.ReadDelivered, metrics.FromMachine)
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, s.cfg.BasePath+workspaceBase+platformOf(rest)+"/file-preview?"+target.Encode(), http.StatusTemporaryRedirect)
	default:
		httpx.WriteError(w, r, httpx.ErrNotFound)
	}
}

// adoptMoved binds an upload the machine's own move verified in the store (migration_objects) to the machine's person,
// as the move does itself for the ones it reports now (migration.bindUpload). The journal keeps no type of the upload,
// so the type is read off the bytes; false when the journal or the store has not got it.
// noteUploadMiss says why a chat's upload was read from the machine, by the journal of the machine's move, counts it
// (data_read_misses_total) and writes one line with the machine and the object: the list of what is still to be
// delivered before the machine's copy can stop being the way out. A failed object is asked to be tried
// again only when the reader is the machine's own person, the one whose «Повторить» the journal takes: a read by
// anybody else of the space changes nothing of the machine's move.
func (s *Service) noteUploadMiss(ctx context.Context, u Uploads, m store.Machine, reader, id string) {
	reason, code := metrics.MissUnreported, ""
	var migration, state, sha, asked string
	err := s.db.QueryRow(ctx, `SELECT migration, state, sha256, code, asked FROM migration_objects
		WHERE machine_id = ? AND kind = ? AND object = ? ORDER BY changed_at DESC LIMIT 1`, m.ID, filestore.KindUploads, id).Scan(&migration, &state, &sha, &code, &asked)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return
	case state == "verified":
		reason = metrics.MissForeign
		if ok, _ := u.HasBlob(ctx, m.SpaceID, filestore.KindUploads, sha); !ok {
			reason = metrics.MissLost
		}
	case state == "failed":
		reason = metrics.MissFailed
		if asked != "retry" && reader != "" && reader == m.CreatedBy {
			_, _ = s.db.Exec(ctx, `UPDATE migration_objects SET asked = 'retry', changed_at = ?
				WHERE machine_id = ? AND migration = ? AND kind = ? AND object = ? AND state = 'failed'`,
				store.Ms(s.now()), m.ID, migration, filestore.KindUploads, id)
		}
	case state == "skipped":
		reason = metrics.MissSkipped
	default:
		reason = metrics.MissPending
	}
	metrics.CountMiss(metrics.ReadUpload, reason)
	s.log.Info("data read from the machine", "kind", "upload", "machine", m.ID, "object", id, "reason", reason, "code", code, "own", reader == m.CreatedBy)
}

func (s *Service) adoptMoved(ctx context.Context, u Uploads, m store.Machine, owner, id string) (uploadRef, bool) {
	var ref uploadRef
	if err := s.db.QueryRow(ctx, `SELECT sha256, size FROM migration_objects
		WHERE machine_id = ? AND kind = ? AND object = ? AND state = 'verified' ORDER BY changed_at DESC LIMIT 1`,
		m.ID, filestore.KindUploads, id).Scan(&ref.Sha256, &ref.Size); err != nil || !filestore.ValidSha(ref.Sha256) {
		return ref, false
	}
	rc, err := u.OpenBlob(ctx, m.SpaceID, filestore.KindUploads, ref.Sha256)
	if err != nil {
		return ref, false
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(rc, head)
	rc.Close()
	if kind, _, _ := mime.ParseMediaType(http.DetectContentType(head[:n])); inlineTypes[kind] {
		ref.Mime = kind
	}
	now := store.Ms(s.now())
	if _, err := s.db.Exec(ctx, `INSERT INTO upload_owners (machine_id, sha256, owner_id, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (machine_id, sha256, owner_id) DO NOTHING`, m.ID, ref.Sha256, owner, now); err != nil {
		return ref, false
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO upload_refs (machine_id, upload_id, owner_id, sha256, mime, name, size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (machine_id, upload_id, owner_id) DO NOTHING`,
		m.ID, id, owner, ref.Sha256, ref.Mime, "", ref.Size, now); err != nil {
		return ref, false
	}
	return ref, true
}

// ownsUpload tells whether the person put these bytes into the machine's space.
func (s *Service) ownsUpload(ctx context.Context, machine, user, sha string) bool {
	var one int
	return s.db.QueryRow(ctx, `SELECT 1 FROM upload_owners WHERE machine_id = ? AND sha256 = ? AND owner_id = ?`, machine, sha, user).Scan(&one) == nil
}

// platformOf is the platform api prefix of a workspace path (/api/acme).
func platformOf(rest string) string {
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) < 3 {
		return ""
	}
	return "/" + parts[1] + "/" + parts[2]
}

func (s *Service) sendBlob(w http.ResponseWriter, r *http.Request, u Uploads, m store.Machine, collection, sha, kind, name string) {
	if !filestore.ValidSha(sha) {
		httpx.WriteError(w, r, httpx.ErrNotFound)
		return
	}
	kind, _, _ = mime.ParseMediaType(kind)
	// Only the types a page shows as they are go inline; anything else is a download, so nothing put here runs on the
	// hub's origin.
	policy := blobserve.Policy{Disposition: "attachment", Name: clip(strings.TrimSpace(name), 255), Cache: blobserve.Immutable,
		CSP: "default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'; sandbox"}
	if inlineTypes[kind] {
		policy.Type, policy.Disposition = kind, "inline"
	}
	err := blobserve.Serve(w, r, blobserve.Blob{Sha: sha, Open: func(ctx context.Context) (io.ReadCloser, error) {
		return u.OpenBlob(ctx, m.SpaceID, collection, sha)
	}}, policy)
	switch {
	case errors.Is(err, blobserve.ErrNotFound):
		httpx.WriteError(w, r, httpx.ErrNotFound)
	case err != nil:
		s.fail(w, r, err)
	}
}

func clip(v string, n int) string {
	if len(v) <= n {
		return v
	}
	cut := v[:n]
	for !utf8.ValidString(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return cut
}
