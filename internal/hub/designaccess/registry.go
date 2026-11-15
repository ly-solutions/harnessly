package designaccess

import (
	"context"
)

// The guest links of a design are entered in the hub's one registry of guest links, resource_guest_links (kind
// "design", id "<machine>/<project>"), under their own id and token hash (stage 3). The registry says
// whether a token is alive; design_share_links keeps what only a design's link has and is written as before, so an
// older revision of the hub reads its links as it always did. Every write of a link ends with enterLinks in the same
// transaction: one statement is the only way a link reaches the registry, so the two cannot tell different stories.
//
// There is no migration: the links that were made before are entered by ReconcileLinks when the hub starts, with the
// token hashes they have, so the addresses people hold keep opening. Going back is safe: a revision without this never
// reads the entries, and `DELETE FROM resource_guest_links WHERE kind = 'design'` takes them out.

const registryKind = "design"

const enterLinksSQL = `INSERT INTO resource_guest_links (link_id, kind, id, space_id, token_hash, rights, pinned_version, title, opens, last_opened_at,
		created_by, created_at, expires_at, revoked_at)
	SELECT l.id, 'design', l.machine_id || '/' || l.project_id, m.space_id, l.token_hash, l.rights,
		CASE WHEN l.version = 'pinned' THEN COALESCE(l.rev, '') ELSE '' END, l.title, l.opens, l.last_opened_at,
		l.created_by, l.created_at, l.expires_at, l.revoked_at
	FROM design_share_links l JOIN machines m ON m.id = l.machine_id WHERE `

const enterLinksUpsert = ` ON CONFLICT (link_id) DO UPDATE SET id = excluded.id, space_id = excluded.space_id, token_hash = excluded.token_hash,
		rights = excluded.rights, pinned_version = excluded.pinned_version, title = excluded.title, opens = excluded.opens,
		last_opened_at = excluded.last_opened_at, expires_at = excluded.expires_at, revoked_at = excluded.revoked_at`

// enterLink writes one link's present state into the registry.
func (t *txn) enterLink(linkID string) error {
	_, err := t.q.Exec(t.ctx, enterLinksSQL+`l.id = ?`+enterLinksUpsert, linkID)
	return err
}

// enterProjectLinks does it for every link of a project, on whichever machines hold it.
func (t *txn) enterProjectLinks(id string) error {
	_, err := t.q.Exec(t.ctx, enterLinksSQL+`l.project_id = ?`+enterLinksUpsert, id)
	return err
}

// leaveRegistry takes the links of a project that is being deleted out of the registry.
func (t *txn) leaveRegistry(machine, id string) error {
	_, err := t.q.Exec(t.ctx, `DELETE FROM resource_guest_links WHERE kind = ? AND id = ?`, registryKind, machine+"/"+id)
	return err
}

// ReconcileLinks makes the registry agree with the links of the designs: it enters what an older revision of the hub
// made or changed while it ran, and drops the entries whose link is no more (a machine that was removed takes its
// links with it). The hub does it when it starts, before it answers anybody, so an address a person holds never
// answers "not found" for a moment; it changes nothing when the two already agree, and two replicas doing it at once
// write the same rows.
func (s *Service) ReconcileLinks(ctx context.Context) {
	q := s.db.S().Q()
	_, err := q.Exec(ctx, enterLinksSQL+`1 = 1`+enterLinksUpsert)
	if err == nil {
		_, err = q.Exec(ctx, `DELETE FROM resource_guest_links WHERE kind = ? AND NOT EXISTS (SELECT 1 FROM design_share_links l WHERE l.id = resource_guest_links.link_id)`, registryKind)
	}
	if err != nil {
		s.log.Warn("design links: the registry of guest links was not reconciled", "error", err)
	}
}
