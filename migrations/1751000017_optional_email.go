package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init makes the users `email` field optional at the schema level so a student
// can self-sign-up with just a username (uid) + password. Whether the signup
// FORM asks for an email is a separate, config-driven choice
// (signup.require_email in nabu-config.yaml -> the signup_require_email setting);
// this migration only removes the hard schema requirement so an account without
// an email is valid. `uid` is already an identity field, so email-less accounts
// can still sign in.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users collection: %w", err)
		}
		f, ok := users.Fields.GetByName("email").(*core.EmailField)
		if !ok {
			return fmt.Errorf("users.email is not an EmailField")
		}
		f.Required = false
		if err := app.Save(users); err != nil {
			return fmt.Errorf("make users.email optional: %w", err)
		}
		return nil
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return nil // already gone
		}
		if f, ok := users.Fields.GetByName("email").(*core.EmailField); ok {
			f.Required = true
			if err := app.Save(users); err != nil {
				return fmt.Errorf("restore users.email requirement: %w", err)
			}
		}
		return nil
	})
}
