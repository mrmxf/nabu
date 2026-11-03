package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds two optional text fields to `slides`:
//   - choice_key: lets slides share a stored radio selection (the two human
//     screens use the same key so the topic carries across both).
//   - notes_label: when set, the slide shows a group-notes textarea with this
//     label (may contain HTML and the {{.GROUPNAME}} placeholder).
func init() {
	m.Register(func(app core.App) error {
		slides, err := app.FindCollectionByNameOrId("slides")
		if err != nil {
			return fmt.Errorf("find slides collection: %w", err)
		}
		slides.Fields.Add(
			&core.TextField{Name: "choice_key"},
			&core.TextField{Name: "notes_label"},
		)
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("update slides: %w", err)
		}
		return nil
	}, func(app core.App) error {
		if slides, err := app.FindCollectionByNameOrId("slides"); err == nil {
			slides.Fields.RemoveByName("choice_key")
			slides.Fields.RemoveByName("notes_label")
			if err := app.Save(slides); err != nil {
				return fmt.Errorf("revert slides: %w", err)
			}
		}
		return nil
	})
}
