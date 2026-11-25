package designaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// the library's long poll wakes on a change of access, not only on a newer revision of the files. A grant to a
// person or a team, its revocation, a new role or term, the project's team level, an accepted invitation and the person
// joining or leaving a team or the space change no file of the store, so a page that waited on the files alone did not
// learn of them until its slow timer, or never while the timer was off.
//
// What is watched is cheap: a few counts and newest times over the space's access tables and the person's own teams
// (accessStamp). It is read every grantedPoll on every replica, so a change made through any replica wakes the page.
// A grant that runs out by time changes no row: the person's whole access is folded again every accessRefold, and its
// digest (what the page shows of it) decides whether the library moved.

// accessRefold is how often the person's access is folded again although no row changed: the grants that ran out.
var accessRefold = 30 * time.Second

// accessStamp is a fingerprint of the rows that decide who sees which project of the machine's space, and of the
// person's place in teams and spaces. Any write of access changes it; it says nothing about what the person may see.
func (s *Service) accessStamp(ctx context.Context, u store.User, m store.Machine) (string, error) {
	q := s.db.S().Q()
	space := m.SpaceID
	type part struct {
		sql  string
		args []any
	}
	parts := []part{
		{`SELECT COUNT(*), COALESCE(MAX(p.updated_at), 0) + COALESCE(MAX(p.meta_at), 0) FROM design_projects p JOIN machines m ON m.id = p.machine_id WHERE m.space_id = ?`, []any{space}},
		{`SELECT COUNT(*), COALESCE(MAX(g.granted_at), 0) FROM design_members g JOIN machines m ON m.id = g.machine_id WHERE m.space_id = ?`, []any{space}},
		{`SELECT COUNT(*), COALESCE(MAX(g.granted_at), 0) FROM design_team_grants g JOIN machines m ON m.id = g.machine_id WHERE m.space_id = ?`, []any{space}},
		{`SELECT COUNT(*), COALESCE(MAX(a.granted_at), 0) FROM design_invite_acceptances a WHERE a.person_email = ?`, []any{normEmail(u.Email)}},
		// The person's own grants in every space: another space's project shared with them by name (foreign.go).
		{`SELECT COUNT(*), COALESCE(MAX(g.granted_at), 0) FROM design_members g WHERE g.person_email = ?`, []any{normEmail(u.Email)}},
		{`SELECT COUNT(*), COALESCE(MAX(created_at), 0) FROM team_members WHERE user_id = ?`, []any{u.ID}},
		{`SELECT COUNT(*), COALESCE(MAX(created_at), 0) FROM memberships WHERE user_id = ?`, []any{u.ID}},
	}
	h := sha256.New()
	for _, p := range parts {
		var n, at int64
		if err := q.QueryRow(ctx, p.sql, p.args...).Scan(&n, &at); err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%d/%d;", n, at)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// accessDigest is what the person may see of the machine's space now: each project open to them with its role, where
// the role comes from and until when. Two digests differ exactly when the library's cards would.
func (s *Service) accessDigest(ctx context.Context, u store.User, m store.Machine) (string, error) {
	if _, stored := s.view.(Librarian); !stored {
		lib, err := s.grantedLibrary(ctx, u, m, true)
		if err != nil {
			return "", err
		}
		return grantedDigest(lib), nil
	}
	t, err := s.read(ctx, m)
	if err != nil {
		return "", err
	}
	rows, err := t.projects()
	if err != nil {
		return "", err
	}
	email := normEmail(u.Email)
	h := sha256.New()
	for _, p := range rows {
		f, err := t.fold(p, email)
		if err != nil {
			return "", err
		}
		if f.Role == "" {
			continue
		}
		until := ""
		if f.Until != nil {
			until = f.Until.UTC().Format(time.RFC3339Nano)
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\n", p.ID, p.Owner, f.Role, f.Source, until)
	}
	far, err := s.foreignLibrary(ctx, u, m)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(h, "foreign\x00%s\n", grantedDigest(storeLibrary{Artifacts: far}))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// accessWatch follows one person's access to a space while a long poll is held: it folds again only when the stamp
// moved or accessRefold passed.
type accessWatch struct {
	s      *Service
	u      store.User
	m      store.Machine
	stamp  string
	digest string
	folded time.Time
}

// current is the digest of what the person may see now.
func (w *accessWatch) current(ctx context.Context) (string, error) {
	stamp, err := w.s.accessStamp(ctx, w.u, w.m)
	if err != nil {
		return "", err
	}
	now := w.s.now()
	if w.digest != "" && stamp == w.stamp && now.Sub(w.folded) < accessRefold {
		return w.digest, nil
	}
	digest, err := w.s.accessDigest(ctx, w.u, w.m)
	if err != nil {
		return "", err
	}
	w.stamp, w.digest, w.folded = stamp, digest, now
	return digest, nil
}

// viewRev is the revision of what the page shows: a clock reading taken when the person's view (their access and, with
// the store, the files' revision) was first seen to differ, so it only grows, per person and space.
func (s *Service) viewRev(u store.User, m store.Machine, digest string) int64 {
	key := u.ID + "|" + m.SpaceID
	s.revs.mu.Lock()
	defer s.revs.mu.Unlock()
	if s.revs.seen == nil || len(s.revs.seen) >= maxGrantedRevs {
		s.revs.seen = map[string]grantedRev{}
	}
	cur := s.revs.seen[key]
	if cur.digest != digest {
		next := s.now().UnixMilli()
		if next <= cur.rev {
			next = cur.rev + 1
		}
		cur = grantedRev{digest: digest, rev: next}
		s.revs.seen[key] = cur
	}
	return cur.rev
}

// grantedRev is the person's view revision with no file store: the one kept while their access reads the same.
func (s *Service) grantedRev(ctx context.Context, u store.User, m store.Machine) (int64, error) {
	digest, err := s.accessDigest(ctx, u, m)
	if err != nil {
		return 0, err
	}
	return s.viewRev(u, m, fmt.Sprintf("%d\x00%s", int64(-1), digest)), nil
}
