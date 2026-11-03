package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init lets students sign in with their username OR their email. `uid` is the
// stable, unique per-student handle (e.g. s7g1-alice, added in
// 1751000003_roles_and_enrolments.go) and already carries a unique index, so it
// qualifies as a PocketBase identity field. Adding it to the users collection's
// passwordAuth.identityFields means authWithPassword(identity, password) matches
// either the email or the uid.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users collection: %w", err)
		}
		users.PasswordAuth.IdentityFields = []string{"email", "uid"}
		if err := app.Save(users); err != nil {
			return fmt.Errorf("update users identity fields: %w", err)
		}
		return nil
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return nil // already gone
		}
		users.PasswordAuth.IdentityFields = []string{"email"}
		if err := app.Save(users); err != nil {
			return fmt.Errorf("revert users identity fields: %w", err)
		}
		return nil
	})
}
