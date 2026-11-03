package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds a persona `audience` field to lessons and slides. Authored in
// frontmatter (params.audience: student|teacher|admin|all); a slide inherits its
// lesson's audience unless it sets its own (resolved at ingest, see
// internal/content). Plain text so an empty value can mean "inherit".
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"lessons", "slides"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s collection: %w", name, err)
			}
			col.Fields.Add(&core.TextField{Name: "audience"})
			if err := app.Save(col); err != nil {
				return fmt.Errorf("add audience to %s: %w", name, err)
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"lessons", "slides"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			col.Fields.RemoveByName("audience")
			if err := app.Save(col); err != nil {
				return fmt.Errorf("remove audience from %s: %w", name, err)
			}
		}
		return nil
	})
}
