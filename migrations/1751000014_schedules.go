package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init adds the lesson-schedule schema:
//
//   - `lesson_schedules`  — one row per lesson, holding the `allow_unscheduled`
//     gate (may a student name their own session, or must they pick a cohort?).
//   - `schedule_sessions` — the named cohort windows under a lesson_schedule,
//     each with an enroll window and a start window. `sched_id` is a STABLE
//     author-supplied key (the YAML anchor / upsert key); `name` is a mutable
//     teacher-editable label. References are by the record id, so a rename never
//     breaks a student's run.
//   - a `schedule_session` relation on the existing `sessions` collection — the
//     id-reference from a student's run of a lesson to the cohort window they
//     joined.
//
// Schedules are seeded from a YAML pre-load (content/lesson-schedules.yaml, see
// internal/content/schedule.go) but are also teacher-editable, so reads are open
// to any signed-in user and writes are gated to teacher/admin by role.
func init() {
	m.Register(func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}

		// readRule: any signed-in user (students need schedules to start a session).
		readRule := types.Pointer(`@request.auth.id != ""`)
		// writeRule: teachers and admins author schedules; superusers bypass rules.
		writeRule := types.Pointer(`@request.auth.role = "teacher" || @request.auth.role = "admin"`)

		// ---- lesson_schedules ---------------------------------------------
		lessonSchedules := core.NewBaseCollection("lesson_schedules")
		lessonSchedules.Fields.Add(
			&core.RelationField{Name: "lesson", Required: true, CollectionId: lessons.Id, MaxSelect: 1, CascadeDelete: true},
			// allow_unscheduled: when true, a student may name their own session
			// instead of picking a scheduled cohort window.
			&core.BoolField{Name: "allow_unscheduled"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		lessonSchedules.AddIndex("idx_lesson_schedules_lesson", true, "lesson", "")
		lessonSchedules.ListRule = readRule
		lessonSchedules.ViewRule = readRule
		lessonSchedules.CreateRule = writeRule
		lessonSchedules.UpdateRule = writeRule
		lessonSchedules.DeleteRule = writeRule
		if err := app.Save(lessonSchedules); err != nil {
			return fmt.Errorf("save lesson_schedules: %w", err)
		}

		// ---- schedule_sessions --------------------------------------------
		scheduleSessions := core.NewBaseCollection("schedule_sessions")
		scheduleSessions.Fields.Add(
			&core.RelationField{Name: "lesson_schedule", Required: true, CollectionId: lessonSchedules.Id, MaxSelect: 1, CascadeDelete: true},
			// sched_id: stable author-supplied key; the YAML anchor and upsert key.
			&core.TextField{Name: "sched_id", Required: true},
			// name: mutable, teacher-editable display label.
			&core.TextField{Name: "name", Required: true},
			&core.DateField{Name: "enroll_from"},
			&core.DateField{Name: "enroll_to"},
			&core.DateField{Name: "start_from"},
			&core.DateField{Name: "start_to"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		scheduleSessions.AddIndex("idx_schedule_sessions_sched", true, "lesson_schedule, sched_id", "")
		scheduleSessions.ListRule = readRule
		scheduleSessions.ViewRule = readRule
		scheduleSessions.CreateRule = writeRule
		scheduleSessions.UpdateRule = writeRule
		scheduleSessions.DeleteRule = writeRule
		if err := app.Save(scheduleSessions); err != nil {
			return fmt.Errorf("save schedule_sessions: %w", err)
		}

		// ---- sessions.schedule_session ------------------------------------
		// The id-reference from a student's run to the cohort window they joined.
		// Optional: unscheduled runs (allow_unscheduled) leave it empty.
		sessions, err := app.FindCollectionByNameOrId("sessions")
		if err != nil {
			return fmt.Errorf("find sessions collection: %w", err)
		}
		sessions.Fields.Add(
			&core.RelationField{Name: "schedule_session", CollectionId: scheduleSessions.Id, MaxSelect: 1},
		)
		if err := app.Save(sessions); err != nil {
			return fmt.Errorf("update sessions: %w", err)
		}

		return nil
	}, func(app core.App) error {
		// down: drop the sessions field first, then the collections (child before
		// parent so the relation is gone before its target).
		if sessions, err := app.FindCollectionByNameOrId("sessions"); err == nil {
			sessions.Fields.RemoveByName("schedule_session")
			if err := app.Save(sessions); err != nil {
				return fmt.Errorf("revert sessions: %w", err)
			}
		}
		for _, name := range []string{"schedule_sessions", "lesson_schedules"} {
			if c, err := app.FindCollectionByNameOrId(name); err == nil {
				if err := app.Delete(c); err != nil {
					return fmt.Errorf("delete %s: %w", name, err)
				}
			}
		}
		return nil
	})
}
