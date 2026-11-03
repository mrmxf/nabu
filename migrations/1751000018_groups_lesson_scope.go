package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init re-scopes the `groups` collection from class to LESSON. A group (its sXgY
// tag) is the unit agent-job rate limiting is scoped to (max_depth, paused).
// Originally a group belonged to a class with a globally-unique tag, so one
// `s7g1` row spanned every lesson in the class and shared a single quota bucket.
//
// The rest of the model already scopes group data by (group tag, lesson) — the
// decisions and captures collections store group + lesson — so class-scoping was
// the odd one out. This makes a group's identity and quota per (lesson, tag): the
// same tag in two lessons is two independent groups, each with its own cap.
//
// Non-destructive: existing rows keep their tag and get an empty lesson, so they
// become inert (they match no real lesson and are recreated per-lesson on demand
// by resolveGroup). A reseed gives a clean slate in dev.
func init() {
	m.Register(func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}
		groups, err := app.FindCollectionByNameOrId("groups")
		if err != nil {
			return fmt.Errorf("find groups collection: %w", err)
		}
		// class -> lesson. lesson is optional so the solo fallback tag and any
		// legacy rows stay valid.
		groups.Fields.RemoveByName("class")
		groups.Fields.Add(
			&core.RelationField{Name: "lesson", CollectionId: lessons.Id, MaxSelect: 1},
		)
		// identity is now (lesson, tag), not the tag alone.
		groups.RemoveIndex("idx_groups_tag")
		groups.AddIndex("idx_groups_lesson_tag", true, "lesson, tag", "")
		if err := app.Save(groups); err != nil {
			return fmt.Errorf("save groups: %w", err)
		}
		return nil
	}, func(app core.App) error {
		classes, err := app.FindCollectionByNameOrId("classes")
		if err != nil {
			return nil // classes gone; nothing to restore against
		}
		groups, err := app.FindCollectionByNameOrId("groups")
		if err != nil {
			return nil // already gone
		}
		groups.Fields.RemoveByName("lesson")
		groups.Fields.Add(
			&core.RelationField{Name: "class", CollectionId: classes.Id, MaxSelect: 1},
		)
		groups.RemoveIndex("idx_groups_lesson_tag")
		groups.AddIndex("idx_groups_tag", true, "tag", "")
		if err := app.Save(groups); err != nil {
			return fmt.Errorf("revert groups: %w", err)
		}
		return nil
	})
}
