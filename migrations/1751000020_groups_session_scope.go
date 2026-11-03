package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init scopes a `groups` identity to the SESSION as well as the lesson. A Session
// (the `schedule_sessions` cohort window) is what a group belongs to: the same
// group tag `g1` in a Tuesday session and a Thursday session of the same lesson
// must be two independent groups with independent shared state (notes/choices/
// agent-config, carried by the Yjs room `${lesson}:${session}:${group}`). See
// docs/glossary.md.
//
// Identity moves from (lesson, tag) to (lesson, session, tag). `session` is a
// relation to `schedule_sessions` and is OPTIONAL: an unscheduled/solo run has no
// session, so its solo tag (`solo-<attemptId>`) stays unique on tag alone.
//
// Destructive by design: existing group rows are cleared, because a group holds
// no durable student-visible state (that's the model rule — no group state
// persists across sessions), and old rows carry no session. A reseed/first-use
// recreates them per (lesson, session, tag) via resolveGroup.
func init() {
	m.Register(func(app core.App) error {
		sched, err := app.FindCollectionByNameOrId("schedule_sessions")
		if err != nil {
			return fmt.Errorf("find schedule_sessions collection: %w", err)
		}
		groups, err := app.FindCollectionByNameOrId("groups")
		if err != nil {
			return fmt.Errorf("find groups collection: %w", err)
		}

		// clear old (lesson, tag)-keyed rows — they predate session-scoping and
		// hold no durable value.
		old, err := app.FindRecordsByFilter("groups", "id != ''", "", 0, 0)
		if err != nil {
			return fmt.Errorf("list groups: %w", err)
		}
		for _, g := range old {
			if err := app.Delete(g); err != nil {
				return fmt.Errorf("clear group %s: %w", g.Id, err)
			}
		}

		groups.Fields.Add(
			&core.RelationField{Name: "session", CollectionId: sched.Id, MaxSelect: 1},
		)
		// identity is now (lesson, session, tag).
		groups.RemoveIndex("idx_groups_lesson_tag")
		groups.AddIndex("idx_groups_lesson_session_tag", true, "lesson, session, tag", "")
		if err := app.Save(groups); err != nil {
			return fmt.Errorf("save groups: %w", err)
		}
		return nil
	}, func(app core.App) error {
		groups, err := app.FindCollectionByNameOrId("groups")
		if err != nil {
			return nil // already gone
		}
		groups.RemoveIndex("idx_groups_lesson_session_tag")
		groups.Fields.RemoveByName("session")
		groups.AddIndex("idx_groups_lesson_tag", true, "lesson, tag", "")
		if err := app.Save(groups); err != nil {
			return fmt.Errorf("revert groups: %w", err)
		}
		return nil
	})
}
