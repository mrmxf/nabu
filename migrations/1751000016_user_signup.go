package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init makes public self-signup safe. The users collection shipped with an empty
// ("" = allow everyone) create rule, which let any caller create a user AND set
// any field on it — including role="admin", a privilege-escalation hole.
//
// This tightens the create rule so a public signup may only mint an ordinary
// student account and must supply a username (uid), matching the signup page:
//   - role MUST be "student" (no self-granted teacher/admin)
//   - uid MUST be non-empty (the username students also sign in with)
//
// Superusers bypass collection rules, so the seeder and the admin roster import
// (which set teacher/admin roles) are unaffected.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users collection: %w", err)
		}
		users.CreateRule = types.Pointer(`@request.body.role = "student" && @request.body.uid != ""`)
		if err := app.Save(users); err != nil {
			return fmt.Errorf("update users create rule: %w", err)
		}
		return nil
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return nil // already gone
		}
		// restore the original "allow everyone" create rule.
		users.CreateRule = types.Pointer("")
		if err := app.Save(users); err != nil {
			return fmt.Errorf("revert users create rule: %w", err)
		}
		return nil
	})
}
