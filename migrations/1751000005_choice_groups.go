package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init replaces the flat `choices` + `choice_key` slide fields with a single
// `t_choice` JSON field: a list of named choice groups, each with its own `id`
// and `options`. A slide places a group in its body with an inline
// {{< LMS t="choice" id="…" >}} widget, so option lists stay in frontmatter
// (small diffs) while the shortcode controls placement. The group `id` also
// subsumes the old choice_key — two slides sharing an id share the saved pick.
//
//	t_choice: [{"id":"topic","legend":"Pick one","options":[{"key":"A","label":"…"}]}]
func init() {
	m.Register(func(app core.App) error {
		slides, err := app.FindCollectionByNameOrId("slides")
		if err != nil {
			return fmt.Errorf("find slides collection: %w", err)
		}
		slides.Fields.RemoveByName("choices")
		slides.Fields.RemoveByName("choice_key")
		slides.Fields.Add(&core.JSONField{Name: "t_choice"})
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("update slides: %w", err)
		}
		return nil
	}, func(app core.App) error {
		slides, err := app.FindCollectionByNameOrId("slides")
		if err != nil {
			return fmt.Errorf("find slides collection: %w", err)
		}
		slides.Fields.RemoveByName("t_choice")
		slides.Fields.Add(
			&core.JSONField{Name: "choices"},
			&core.TextField{Name: "choice_key"},
		)
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("revert slides: %w", err)
		}
		return nil
	})
}
