package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds a `logo` text field to the classes and lessons collections. It is
// authored per class/lesson as params.logo in _index.md and overrides the default
// navbar logo (config logopath -> app_config site_logo) while that class or lesson
// is on screen. Served by basename, like the other logo settings.
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"classes", "lessons"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s collection: %w", name, err)
			}
			col.Fields.Add(&core.TextField{Name: "logo"})
			if err := app.Save(col); err != nil {
				return fmt.Errorf("update %s: %w", name, err)
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"classes", "lessons"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s collection: %w", name, err)
			}
			col.Fields.RemoveByName("logo")
			if err := app.Save(col); err != nil {
				return fmt.Errorf("revert %s: %w", name, err)
			}
		}
		return nil
	})
}
