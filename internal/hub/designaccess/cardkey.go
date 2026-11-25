package designaccess

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"

	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// Without a file store each machine keeps its own copy of a project, and two copies may share one slug: a project of
// the person's own and one shared with them, or two shared by different owners. A card of the library is therefore
// named by the copy it stands for, not by the slug alone: its card key is an opaque digest of the holder machine and
// the project. It reveals neither the machine's id nor any address or path, and the hub resolves it back only among the
// live machines of the person's space, where the person's own grant is checked again.

const cardKeyInfo = "harnessly-design-card/v1"

var cardRe = regexp.MustCompile(`^[0-9a-f]{24}$`)

func cardShape(card string) bool { return cardRe.MatchString(card) }

// cardKey names the project of one holder machine.
func cardKey(holder, project string) string {
	sum := sha256.Sum256([]byte(cardKeyInfo + "\x00" + holder + "\x00" + project))
	return hex.EncodeToString(sum[:12])
}

// holderByCard is the live machine of the space whose copy of project the card names. A card that names no copy, or
// one in another space, is not found.
func (s *Service) holderByCard(ctx context.Context, anchor store.Machine, project, card string) (string, error) {
	if !cardShape(card) {
		return "", fail("not_found", "Проект не найден")
	}
	rows, err := s.db.S().Q().Query(ctx, `SELECT p.machine_id FROM design_projects p JOIN machines m ON m.id=p.machine_id
		WHERE m.space_id=? AND p.project_id=? AND m.revoked_at IS NULL AND m.disabled_at IS NULL`, anchor.SpaceID, project)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		if cardKey(id, project) == card {
			return id, nil
		}
	}
	if err := rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return "", fail("not_found", "Проект не найден")
}

// withoutCard is a query without its card parameter: a keyed address carries the card in the key instead.
func withoutCard(q url.Values, raw string) string {
	if !q.Has("card") {
		return raw
	}
	q.Del("card")
	return q.Encode()
}
