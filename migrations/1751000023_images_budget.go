package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds the per-group image budget used by pixie's "make an image" mode.
//
//	lessons.images_budget    the authored starting number of images a group may
//	                         make in this lesson (from the lesson _index.md header).
//	groups.images_remaining  the live per-group counter, seeded from images_budget
//	                         at group creation, decremented server-side each time
//	                         the group makes an image, and blocked at 0.
//
// The counter lives on the group record (not the group's Yjs doc) so the server
// is the authority: it is decremented in a transaction at job submit (concurrent
// image submits can't both pass), and teachers can override it live per group via
// the staff-only groups UpdateRule — neither is possible for the opaque, student-
// owned Yjs document.
func init() {
	m.Register(func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}
		lessons.Fields.Add(&core.NumberField{Name: "images_budget", OnlyInt: true})
		if err := app.Save(lessons); err != nil {
			return fmt.Errorf("save lessons: %w", err)
		}

		groups, err := app.FindCollectionByNameOrId("groups")
		if err != nil {
			return fmt.Errorf("find groups collection: %w", err)
		}
		groups.Fields.Add(&core.NumberField{Name: "images_remaining", OnlyInt: true})
		if err := app.Save(groups); err != nil {
			return fmt.Errorf("save groups: %w", err)
		}
		return nil
	}, func(app core.App) error {
		if groups, err := app.FindCollectionByNameOrId("groups"); err == nil {
			groups.Fields.RemoveByName("images_remaining")
			if err := app.Save(groups); err != nil {
				return fmt.Errorf("revert groups: %w", err)
			}
		}
		if lessons, err := app.FindCollectionByNameOrId("lessons"); err == nil {
			lessons.Fields.RemoveByName("images_budget")
			if err := app.Save(lessons); err != nil {
				return fmt.Errorf("revert lessons: %w", err)
			}
		}
		return nil
	})
}
