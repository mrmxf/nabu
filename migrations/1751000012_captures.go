package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init creates the `captures` collection: a snapshot of the derived data a
// student/group actually worked on (the output of a `t_mapper`), so a teacher can
// inspect it later even if the source data file changes, and so it can be reused
// (e.g. by AI agents) without recompute. One row per (session, key_id); the value
// is minified JSON. Written server-side on navigation/exit (see
// internal/httpapi/captures.go); read by the owning student or staff.
func init() {
	m.Register(func(app core.App) error {
		sessions, err := app.FindCollectionByNameOrId("sessions")
		if err != nil {
			return fmt.Errorf("find sessions collection: %w", err)
		}
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}

		captures := core.NewBaseCollection("captures")
		captures.Fields.Add(
			&core.RelationField{Name: "session", Required: true, CollectionId: sessions.Id, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "group"},
			&core.RelationField{Name: "lesson", Required: true, CollectionId: lessons.Id, MaxSelect: 1},
			&core.TextField{Name: "key_id", Required: true}, // the t_mapper id
			&core.TextField{Name: "scope"},
			&core.JSONField{Name: "value"}, // minified derived data
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		captures.AddIndex("idx_captures_session_key", true, "session, key_id", "")

		// The owning student or staff may read; writes are server-only.
		read := `session.user = @request.auth.id || @request.auth.role = "teacher" || @request.auth.role = "admin"`
		captures.ListRule = &read
		captures.ViewRule = &read

		if err := app.Save(captures); err != nil {
			return fmt.Errorf("save captures collection: %w", err)
		}
		return nil
	}, func(app core.App) error {
		captures, err := app.FindCollectionByNameOrId("captures")
		if err != nil {
			return nil // already gone
		}
		return app.Delete(captures)
	})
}
