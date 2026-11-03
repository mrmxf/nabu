package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init adds people-management schema:
//
//   - `role` (admin|teacher|student) and a stable unique `uid` handle on the
//     built-in `users` collection. `uid` is the import/backup match key so a
//     user's `email` can change without breaking links.
//   - an `enrolments` join collection relating users <-> classes with a
//     per-class role, so admins can assign teachers and students to classes.
//
// See docs/student-management.md.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users collection: %w", err)
		}
		users.Fields.Add(
			&core.SelectField{Name: "role", MaxSelect: 1,
				Values: []string{"admin", "teacher", "student"}},
			// uid: stable admin-assigned handle, unique, decoupled from email.
			&core.TextField{Name: "uid"},
		)
		users.AddIndex("idx_users_uid", true, "uid", "uid != ''")
		if err := app.Save(users); err != nil {
			return fmt.Errorf("update users: %w", err)
		}

		classes, err := app.FindCollectionByNameOrId("classes")
		if err != nil {
			return fmt.Errorf("find classes collection: %w", err)
		}

		// ---- enrolments ----------------------------------------------------
		// Assigns a user to a class with a per-class role. Students see their own
		// rows; teachers and admins can read rosters; only admins (or superusers,
		// who bypass rules) may write.
		enrolments := core.NewBaseCollection("enrolments")
		enrolments.Fields.Add(
			&core.RelationField{Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "class", Required: true, CollectionId: classes.Id, MaxSelect: 1, CascadeDelete: true},
			&core.SelectField{Name: "role_in_class", MaxSelect: 1, Values: []string{"teacher", "student"}},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		enrolments.AddIndex("idx_enrolments_user_class", true, "user, class", "")
		readRule := types.Pointer(`@request.auth.id != "" && (user = @request.auth.id || @request.auth.role = "admin" || @request.auth.role = "teacher")`)
		writeRule := types.Pointer(`@request.auth.role = "admin"`)
		enrolments.ListRule = readRule
		enrolments.ViewRule = readRule
		enrolments.CreateRule = writeRule
		enrolments.UpdateRule = writeRule
		enrolments.DeleteRule = writeRule
		if err := app.Save(enrolments); err != nil {
			return fmt.Errorf("save enrolments: %w", err)
		}

		return nil
	}, func(app core.App) error {
		if enr, err := app.FindCollectionByNameOrId("enrolments"); err == nil {
			if err := app.Delete(enr); err != nil {
				return fmt.Errorf("delete enrolments: %w", err)
			}
		}
		if users, err := app.FindCollectionByNameOrId("users"); err == nil {
			users.Fields.RemoveByName("role")
			users.Fields.RemoveByName("uid")
			if err := app.Save(users); err != nil {
				return fmt.Errorf("revert users: %w", err)
			}
		}
		return nil
	})
}
