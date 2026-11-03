package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init adds the `session_timers` schema: one synchronized countdown per
// (schedule_session, timer_id). A teacher-scoped timer is declared in a slide's
// `timers:` frontmatter (id + scope + label + seconds); the FIRST student to
// reach a declaring slide creates the row (server-side, via
// POST /api/session/timer/ensure) and every other student in the same cohort
// then reads the same absolute `end_time` over realtime, so all clocks agree.
//
// The write rules are staff-only: teachers pause/resume/extend/reset from the
// dashboard timers-card by PATCHing the record directly. The student create path
// runs in app context and bypasses the CreateRule (like a job submission).
//
//   - end_time     — absolute deadline; authoritative while `running`.
//   - remaining_ms — frozen remaining time while paused.
//   - duration_seconds — the authored default, used by Reset.
func init() {
	m.Register(func(app core.App) error {
		scheduleSessions, err := app.FindCollectionByNameOrId("schedule_sessions")
		if err != nil {
			return fmt.Errorf("find schedule_sessions collection: %w", err)
		}

		// readRule: any signed-in user (every student in the cohort reads the timer).
		readRule := types.Pointer(`@request.auth.id != ""`)
		// writeRule: teachers and admins control timers; superusers bypass rules.
		// The student create path writes in app context and is not subject to this.
		writeRule := types.Pointer(`@request.auth.role = "teacher" || @request.auth.role = "admin"`)

		timers := core.NewBaseCollection("session_timers")
		timers.Fields.Add(
			&core.RelationField{Name: "session", Required: true, CollectionId: scheduleSessions.Id, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "timer_id", Required: true},
			&core.TextField{Name: "label"},
			&core.NumberField{Name: "duration_seconds", OnlyInt: true},
			&core.BoolField{Name: "running"},
			&core.DateField{Name: "end_time"},
			&core.NumberField{Name: "remaining_ms", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		// Unique per (cohort, timer id): the race guard that makes "first student
		// wins" deterministic — a second concurrent create hits this constraint.
		timers.AddIndex("idx_session_timers_session_timer", true, "session, timer_id", "")
		timers.ListRule = readRule
		timers.ViewRule = readRule
		timers.CreateRule = writeRule
		timers.UpdateRule = writeRule
		timers.DeleteRule = writeRule
		if err := app.Save(timers); err != nil {
			return fmt.Errorf("save session_timers: %w", err)
		}

		return nil
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("session_timers"); err == nil {
			if err := app.Delete(c); err != nil {
				return fmt.Errorf("delete session_timers: %w", err)
			}
		}
		return nil
	})
}
