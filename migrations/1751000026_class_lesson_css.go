package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds a `css` text field to the classes and lessons collections. It is
// authored per class/lesson as params.css in _index.md and names a stylesheet in
// that bundle (conventionally _class.css / _lesson.css), copied into the public
// tree at seed like a logo. The player links them after nabu.css, class first
// then lesson, giving the authoring cascade:
//
//	nabu.css (base tokens) -> _class.css (course theme) -> _lesson.css (override)
//
// Holds the resolved public URL, not the authored filename, so the frontend can
// use it as an href directly.
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"classes", "lessons"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s collection: %w", name, err)
			}
			col.Fields.Add(&core.TextField{Name: "css"})
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
			col.Fields.RemoveByName("css")
			if err := app.Save(col); err != nil {
				return fmt.Errorf("revert %s: %w", name, err)
			}
		}
		return nil
	})
}
