package designaccess

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// General access («Общий доступ → Все, у кого есть ссылка», HRN-1791, docs/hub/design-access.md): the one link of a
// project that opens it for anyone, without an account, Google's general access. It is a live guest link like the
// others (the guest reads the store, the owner's machine is not asked), marked general and never listed among them.
// Switched off it is revoked and kept: switched on again it opens at the same address, so a link already handed out
// works again. Its maker is whoever set it last: a guest link opens while its maker may share the project.

const generalTitle = "Все, у кого есть ссылка"

// generalRow is the project's general link, on or off; false when it never had one.
func (t *txn) generalRow(id string) (shareRow, bool, error) {
	scope, args := t.linkScope(id)
	r, err := scanShare(t.q.QueryRow(t.ctx, `SELECT `+shareCols+` FROM design_share_links l WHERE `+scope+` AND l.general = 1
		ORDER BY l.created_at DESC, l.id LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return shareRow{}, false, nil
	}
	return r, err == nil, err
}

// generalOf is what the dialog shows: the level ("none", "view", "comment") and the address while it is on.
func (s *Service) generalOf(t *txn, project string) (map[string]any, error) {
	off := map[string]any{"level": levelNone, "url": nil}
	r, ok, err := t.generalRow(project)
	if err != nil || !ok || r.Revoked != nil || (r.Expires != nil && !r.Expires.After(t.now)) {
		return off, err
	}
	if may, err := t.makerMayShare(project, normEmail(r.CreatedBy)); err != nil || !may {
		return off, err
	}
	out := map[string]any{"level": r.Rights, "url": nil, "opens": r.Opens}
	if token, ok, err := t.shareAddress(project, r.ID); err != nil {
		return nil, err
	} else if ok {
		out["url"] = s.shareURLOf(token)
	}
	return out, nil
}

// setGeneral switches the general link to level; on, it answers the token, kept or new.
func (t *txn) setGeneral(id, actor, level string) (string, error) {
	if _, err := t.lockProject(id); err != nil {
		return "", err
	}
	r, ok, err := t.generalRow(id)
	if err != nil {
		return "", err
	}
	if level == levelNone {
		if !ok || r.Revoked != nil {
			return "", nil
		}
		return "", t.revokeShare(id, actor, r.ID)
	}
	if ok {
		if token, kept, err := t.shareAddress(id, r.ID); err != nil {
			return "", err
		} else if kept {
			scope, args := t.linkScope(id, level, normEmail(actor), r.ID)
			if _, err := t.q.Exec(t.ctx, `UPDATE design_share_links SET revoked_at = NULL, expires_at = NULL, rights = ?, created_by = ?
				WHERE id = ? AND `+scope, args...); err != nil {
				return "", err
			}
			if err := t.enterLink(r.ID); err != nil {
				return "", err
			}
			return token, t.writeLog(id, actor, "share.general", r.ID, map[string]any{"rights": level})
		}
		// The hub's key changed: the old address cannot be copied again, a new one replaces it.
		if r.Revoked == nil {
			if err := t.revokeShare(id, actor, r.ID); err != nil {
				return "", err
			}
		}
	}
	row, token, err := t.createShare(id, actor, shareInput{Title: generalTitle, Rights: level, TTL: ttl{none: true}, Version: "latest"})
	if err != nil {
		return "", err
	}
	scope, args := t.linkScope(id, row.ID)
	if _, err := t.q.Exec(t.ctx, `UPDATE design_share_links SET general = 1 WHERE id = ? AND `+scope, args...); err != nil {
		return "", err
	}
	return token, t.writeLog(id, actor, "share.general", row.ID, map[string]any{"rights": level})
}

// generalRoute is PUT …/general {level}: who may make a link may set it; the apps' checks stop it as any live link.
func (s *Service) generalRoute(r *http.Request, m store.Machine, email, project string) (any, int, error) {
	ctx := r.Context()
	if s.shareURL == "" {
		return nil, 0, fail("unavailable", "Ссылки без входа на этом хабе не настроены")
	}
	b, err := readBody(r)
	if err != nil {
		return nil, 0, err
	}
	level, _ := b["level"].(string)
	if level != levelNone && level != roleView && level != roleComment {
		return nil, 0, fail("bad_request", "Общий доступ: none, view или comment")
	}
	if level != levelNone {
		if err := s.shareGate(ctx, m, project); err != nil {
			return nil, 0, err
		}
	}
	if err := s.write(ctx, m, func(t *txn) error {
		if ok, e := t.makerMayShare(project, email); e != nil || !ok {
			if e != nil {
				return e
			}
			return fail("forbidden", deniedManageAny)
		}
		_, err := t.setGeneral(project, email, level)
		return err
	}); err != nil {
		return nil, 0, err
	}
	v, err := s.viewOf(ctx, m, email, project)
	return v, 200, err
}
