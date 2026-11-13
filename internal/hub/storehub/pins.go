package storehub

import (
	"context"

	"github.com/ly-solutions/harnessly/internal/filestore"
	"github.com/ly-solutions/harnessly/internal/hub/store"
)

// pinSources is every table of the hub whose row is the only record of a blob in the store: nothing in the store's
// own revisions refers to these blobs, so its collection of garbage must be told of them (filestore.Pins). A table
// that starts to keep a blob this way is added here, or its blobs are deleted a day after they are written.
var pinSources = []string{
	// the page of a chat's snapshot
	`SELECT space_id, '` + filestore.KindFiles + `', sha256 FROM chat_shares`,
	// the answers worked out from a design's revision
	`SELECT space_id, '` + filestore.KindFiles + `', sha256 FROM design_derived`,
	// a chat's attachments, by who may read them and by who put them
	`SELECT COALESCE(m.space_id, ''), '` + filestore.KindUploads + `', u.sha256 FROM upload_refs u JOIN machines m ON m.id = u.machine_id`,
	`SELECT COALESCE(m.space_id, ''), '` + filestore.KindUploads + `', u.sha256 FROM upload_owners u JOIN machines m ON m.id = u.machine_id`,
	// what a machine delivered to the store (the journal of the move)
	`SELECT COALESCE(m.space_id, ''), o.kind, o.sha256 FROM migration_objects o JOIN machines m ON m.id = o.machine_id WHERE o.sha256 <> ''`,
}

// Pins reads the blobs the hub's own tables refer to, by room; legacy is the id of the space whose rooms are bare
// kinds ("" for none).
func Pins(db *store.DB, legacy string) filestore.Pins {
	return func(ctx context.Context) (map[string]map[string]struct{}, error) {
		out := map[string]map[string]struct{}{}
		q := db.S().Q()
		for _, src := range pinSources {
			rows, err := q.Query(ctx, src)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var space, kind, sha string
				if err := rows.Scan(&space, &kind, &sha); err != nil {
					rows.Close()
					return nil, err
				}
				room := RoomName(legacy, space, kind)
				if out[room] == nil {
					out[room] = map[string]struct{}{}
				}
				out[room][sha] = struct{}{}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	}
}
