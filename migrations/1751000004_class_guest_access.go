package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds `guest_access` to `classes`: a boolean (default false) that marks a
// class as viewable by signed-out guests. Nabu's own help class sets it true so
// the read-only help lessons at /nabu/help are reachable without login. Content
// is already public-read via the liveRule; this flag drives which classes the
// guest UI offers (see nabu/web guest help viewer).
func init() {
	m.Register(func(app core.App) error {
		classes, err := app.FindCollectionByNameOrId("classes")
		if err != nil {
			return fmt.Errorf("find classes collection: %w", err)
		}
		classes.Fields.Add(
			&core.BoolField{Name: "guest_access"},
		)
		if err := app.Save(classes); err != nil {
			return fmt.Errorf("update classes: %w", err)
		}
		return nil
	}, func(app core.App) error {
		if classes, err := app.FindCollectionByNameOrId("classes"); err == nil {
			classes.Fields.RemoveByName("guest_access")
			if err := app.Save(classes); err != nil {
				return fmt.Errorf("revert classes: %w", err)
			}
		}
		return nil
	})
}
