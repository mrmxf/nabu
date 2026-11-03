package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init creates the `ydoc_updates` collection: the sync log for the per-group
// collaborative Yjs document (see docs/lesson-choice-values.md and the group
// collaboration plan). Each row is one Yjs binary update, base64-encoded, tagged
// with the room it belongs to. A room is a `${lesson}:${group}` pair, so every
// member of a group working a lesson shares one CRDT document holding their
// notes, choices, and group-editable agent configs.
//
// The collection is append-only: a client emits an update, POSTs it to
// /api/ydoc/push (which derives the room from the caller's session, so a student
// can only write their own room), the row is created, and PocketBase realtime
// fans it out to every group member's provider, which applies it to its Y.Doc.
// On load a client GETs /api/ydoc/pull and merges every row for its room.
//
// Reads are open to any signed-in user (updates are low-sensitivity and clients
// subscribe via realtime); writes are server-only (no client create/update rule).
func init() {
	m.Register(func(app core.App) error {
		ydoc := core.NewBaseCollection("ydoc_updates")
		ydoc.Fields.Add(
			&core.TextField{Name: "room", Required: true},  // `${lesson}:${group}`
			&core.TextField{Name: "update", Required: true}, // base64 Yjs update (Y.encodeStateAsUpdate / doc update)
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		// Rooms are queried and swept by name; ordering the log is by created.
		ydoc.AddIndex("idx_ydoc_room", false, "room", "")

		// Signed-in users may read (realtime); writes are server-only.
		read := "@request.auth.id != \"\""
		ydoc.ListRule = &read
		ydoc.ViewRule = &read

		if err := app.Save(ydoc); err != nil {
			return fmt.Errorf("save ydoc_updates collection: %w", err)
		}
		return nil
	}, func(app core.App) error {
		ydoc, err := app.FindCollectionByNameOrId("ydoc_updates")
		if err != nil {
			return nil // already gone
		}
		return app.Delete(ydoc)
	})
}
