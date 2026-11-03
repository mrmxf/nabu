package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds a `t_mapper` JSON field to the slides collection: the derived-data
// specs authored under a slide's `t_mapper` frontmatter (id, from, source, root,
// columns). A {{< LMS t="tabulate" from="…" >}} renders the value they derive.
// See docs/lesson-choice-values.md.
func init() {
	m.Register(func(app core.App) error {
		slides, err := app.FindCollectionByNameOrId("slides")
		if err != nil {
			return fmt.Errorf("find slides collection: %w", err)
		}
		slides.Fields.Add(&core.JSONField{Name: "t_mapper"})
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("update slides: %w", err)
		}
		return nil
	}, func(app core.App) error {
		slides, err := app.FindCollectionByNameOrId("slides")
		if err != nil {
			return fmt.Errorf("find slides collection: %w", err)
		}
		slides.Fields.RemoveByName("t_mapper")
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("revert slides: %w", err)
		}
		return nil
	})
}
