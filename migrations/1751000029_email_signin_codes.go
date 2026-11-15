package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Email sign-in codes for users, and logins that last a teaching day.
//
//   - otp: on, a 6-digit code valid for 10 minutes, with a plain template. A
//     code goes only to an account that already exists (imported from a roster),
//     so the roster is the allow-list. Nabu's /api/auth/otp accepts a username
//     or an email; the code always goes to the email on record.
//   - authToken.duration: 9 hours (32400 s), down from PocketBase's 5 days.
//
// Down restores PocketBase's defaults: otp off (3 min, 8 digits, its own
// template) and 5-day tokens.
const (
	signinCodeSeconds = 600
	signinCodeLength  = 6
	signinTokenSecs   = 9 * 60 * 60
)

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users collection: %w", err)
		}
		users.OTP.Enabled = true
		users.OTP.Duration = signinCodeSeconds
		users.OTP.Length = signinCodeLength
		users.OTP.EmailTemplate = core.EmailTemplate{
			Subject: "Your sign-in code for " + core.EmailPlaceholderAppName,
			Body: `<p>Hello,</p>
<p>Your sign-in code for ` + core.EmailPlaceholderAppName + ` is:</p>
<p style="font-size:1.6em;letter-spacing:0.2em"><strong>` + core.EmailPlaceholderOTP + `</strong></p>
<p>It works for 10 minutes. If you did not ask to sign in, ignore this email.</p>`,
		}
		users.AuthToken.Duration = signinTokenSecs
		if err := app.Save(users); err != nil {
			return fmt.Errorf("enable email sign-in codes: %w", err)
		}
		return nil
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return nil // already gone
		}
		users.OTP.Enabled = false
		users.OTP.Duration = 180
		users.OTP.Length = 8
		users.OTP.EmailTemplate = core.EmailTemplate{
			Subject: "OTP for " + core.EmailPlaceholderAppName,
			Body: `<p>Hello,</p>
<p>Your one-time password is: <strong>` + core.EmailPlaceholderOTP + `</strong></p>
<p><i>If you didn't ask for the one-time password, you can ignore this email.</i></p>
<p>
  Thanks,<br/>
  ` + core.EmailPlaceholderAppName + ` team
</p>`,
		}
		users.AuthToken.Duration = 432000
		if err := app.Save(users); err != nil {
			return fmt.Errorf("restore PocketBase's sign-in defaults: %w", err)
		}
		return nil
	})
}
