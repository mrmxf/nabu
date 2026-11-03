package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init creates the `decisions` collection: the shared store for group-scoped
// choice values (see docs/lesson-choice-values.md). A record is one group's
// current pick for a choice id in a lesson: keyed uniquely by (group, lesson,
// key_id) so a pick upserts, last-write-wins. `value` holds the resolved
// {key,label,slug,scope} so `{{< LMS t="print" >}}` can render it anywhere.
//
// Reads are open to any signed-in user (picks are low-sensitivity, and clients
// subscribe via realtime); writes are server-only (via /api/decisions/set, which
// derives group+lesson from the caller's session), so no client create/update
// rule is set.
func init() {
	m.Register(func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}

		decisions := core.NewBaseCollection("decisions")
		decisions.Fields.Add(
			&core.TextField{Name: "group", Required: true},                                              // the session's group_name
			&core.RelationField{Name: "lesson", Required: true, CollectionId: lessons.Id, MaxSelect: 1}, // the lesson
			&core.TextField{Name: "key_id", Required: true},                                             // the t_choice id
			&core.JSONField{Name: "value"},                                                              // resolved {key,label,slug,scope}
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		// Upsert key: one row per (group, lesson, choice id).
		decisions.AddIndex("idx_decisions_group_lesson_key", true, "`group`, lesson, key_id", "")

		// Signed-in users may read (realtime); writes are server-only.
		read := "@request.auth.id != \"\""
		decisions.ListRule = &read
		decisions.ViewRule = &read

		if err := app.Save(decisions); err != nil {
			return fmt.Errorf("save decisions collection: %w", err)
		}
		return nil
	}, func(app core.App) error {
		decisions, err := app.FindCollectionByNameOrId("decisions")
		if err != nil {
			return nil // already gone
		}
		return app.Delete(decisions)
	})
}
