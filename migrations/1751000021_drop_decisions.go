package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init drops the legacy `decisions` collection. Group-scoped choice values now
// live in the group's collaborative Yjs document (the `choices` Y.Map, persisted
// as base64 updates in `ydoc_updates` — see 1751000019_ydoc_updates.go and
// internal/httpapi/ydoc.go), so `decisions` is unused. It also carried an
// open read rule (any signed-in user could list every group's picks), so removing
// it closes that surface. Idempotent: a no-op when the collection is already
// absent (a fresh DB creates it in 1751000010 and drops it here in the same run).
func init() {
	m.Register(func(app core.App) error {
		decisions, err := app.FindCollectionByNameOrId("decisions")
		if err != nil {
			return nil // already absent
		}
		return app.Delete(decisions)
	}, func(app core.App) error {
		// down: do not recreate the legacy collection.
		return nil
	})
}
