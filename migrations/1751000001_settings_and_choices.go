package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init adds:
//   - a global `settings` key/value collection (public read) for site config such
//     as logos and the session/group name auto-templates.
//   - `choices` and `timers` JSON fields on `slides` so a slide can render a radio
//     selector and one or more countdown segments.
func init() {
	m.Register(func(app core.App) error {
		// ---- app_config (key/value config) ---------------------------------
		// NB: "settings" is a reserved collection name in PocketBase, so the
		// global config table is called app_config.
		config := core.NewBaseCollection("app_config")
		config.Fields.Add(
			&core.TextField{Name: "key", Required: true},
			&core.TextField{Name: "value"},
		)
		config.AddIndex("idx_app_config_key", true, "key", "")
		config.ListRule = types.Pointer("")
		config.ViewRule = types.Pointer("")
		if err := app.Save(config); err != nil {
			return fmt.Errorf("save app_config: %w", err)
		}

		// ---- slides: choices + timers --------------------------------------
		slides, err := app.FindCollectionByNameOrId("slides")
		if err != nil {
			return fmt.Errorf("find slides collection: %w", err)
		}
		slides.Fields.Add(
			// choices: [{"key":"A","label":"…"}, …] -> radio selector
			&core.JSONField{Name: "choices"},
			// timers: [{"label":"Task","seconds":900}, …] -> chained countdowns
			&core.JSONField{Name: "timers"},
		)
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("update slides: %w", err)
		}

		return nil
	}, func(app core.App) error {
		if slides, err := app.FindCollectionByNameOrId("slides"); err == nil {
			slides.Fields.RemoveByName("choices")
			slides.Fields.RemoveByName("timers")
			if err := app.Save(slides); err != nil {
				return fmt.Errorf("revert slides: %w", err)
			}
		}
		if config, err := app.FindCollectionByNameOrId("app_config"); err == nil {
			if err := app.Delete(config); err != nil {
				return fmt.Errorf("delete app_config: %w", err)
			}
		}
		return nil
	})
}
