package resaccess

import (
	"database/sql"
	"errors"
	"net/http"
)

// General access («Общий доступ → Все, у кого есть ссылка»): the one guest link of a resource that opens it
// for anyone, without an account, Google's general access. It is a guest link like the others, marked general, never
// listed among them, its address kept sealed (Seal) so the dialog can copy it again. Switched off it is revoked and
// kept: switched on again it opens at the same address, so a link already handed out works again.

const (
	generalTitle = "Все, у кого есть ссылка"
	sealTable    = "resource_guest_links"
)

// generalLink is the resource's general link, on or off, with its sealed address; false when it never had one.
func (t *txn) generalLink(ref Ref) (link, string, bool, error) {
	l, err := scanLink(t.q.QueryRow(t.ctx, `SELECT `+linkCols+` FROM resource_guest_links l WHERE l.kind = ? AND l.id = ? AND COALESCE(l.space_id, '') = ?
		AND l.general = 1 ORDER BY l.created_at DESC, l.link_id LIMIT 1`, ref.Kind, ref.ID, ref.Space))
	if errors.Is(err, sql.ErrNoRows) {
		return link{}, "", false, nil
	}
	if err != nil {
		return link{}, "", false, err
	}
	var enc sql.NullString
	if err := t.q.QueryRow(t.ctx, `SELECT token_enc FROM resource_guest_links WHERE link_id = ?`, l.ID).Scan(&enc); err != nil {
		return link{}, "", false, err
	}
	return l, enc.String, true, nil
}

// general is what the dialog shows: the level ("none", "view", "comment") and the address while it is on.
func (s *Service) general(t *txn, ref Ref, target string) (map[string]any, error) {
	off := map[string]any{"level": levelNone, "url": nil}
	l, enc, ok, err := t.generalLink(ref)
	if err != nil || !ok || l.Revoked != nil || (l.Expires != nil && !l.Expires.After(t.now)) {
		return off, err
	}
	out := map[string]any{"level": l.Rights, "url": nil, "opens": l.Opens}
	if token, ok := s.Seal.Open(sealTable, l.ID, enc); ok {
		out["url"] = s.linkURL(ref, target, token)
	}
	return out, nil
}

// setGeneral switches the general link to level; on, it keeps the address it had when the hub can open it.
func (s *Service) setGeneral(t *txn, ref Ref, actor, level, target string) error {
	l, enc, ok, err := t.generalLink(ref)
	if err != nil {
		return err
	}
	if level == levelNone {
		if !ok {
			return nil
		}
		return t.revokeLink(ref, actor, l.ID)
	}
	if ok {
		if _, kept := s.Seal.Open(sealTable, l.ID, enc); kept {
			if _, err := t.q.Exec(t.ctx, `UPDATE resource_guest_links SET revoked_at = NULL, expires_at = NULL, rights = ?, options = ? WHERE link_id = ?`,
				level, target, l.ID); err != nil {
				return err
			}
			return t.log(ref, actor, "link_general", l.ID, map[string]any{"rights": level})
		}
		// The hub's key changed: the old address cannot be copied again, a new one replaces it. The old link stops
		// being the general one, or a new link made in the same second could lose the pick to it.
		if err := t.revokeLink(ref, actor, l.ID); err != nil {
			return err
		}
		if _, err := t.q.Exec(t.ctx, `UPDATE resource_guest_links SET general = 0 WHERE link_id = ?`, l.ID); err != nil {
			return err
		}
	}
	made, token, err := t.createLink(ref, actor, linkInput{Rights: level, Title: generalTitle, Target: target, Term: ttl{none: true}})
	if err != nil {
		return err
	}
	if _, err := t.q.Exec(t.ctx, `UPDATE resource_guest_links SET general = 1, token_enc = ? WHERE link_id = ?`,
		nullStr(s.Seal.Seal(sealTable, made.ID, token)), made.ID); err != nil {
		return err
	}
	return t.log(ref, actor, "link_general", made.ID, map[string]any{"rights": level})
}

// generalRoute is PUT …/general {level}: whoever may make a guest link may set it.
func (s *Service) generalRoute(r *http.Request, ref Ref, actor string, info Info, change func(func(*txn) error) (any, int, error)) (any, int, error) {
	b, err := readBody(r)
	if err != nil {
		return nil, 0, err
	}
	level := str(b, "level")
	if level != levelNone && level != roleView && level != roleComment {
		return nil, 0, fail("bad_request", "Общий доступ: none, view или comment")
	}
	return change(func(t *txn) error { return s.setGeneral(t, ref, actor, level, info.Target) })
}
