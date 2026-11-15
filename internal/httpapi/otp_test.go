package httpapi

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// TestEmailSignInCodes: the migration turns codes on with 9-hour logins, a user
// is found by username or email, and a code is minted for the email on record.
func TestEmailSignInCodes(t *testing.T) {
	app := newMigratedApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if !users.OTP.Enabled || users.OTP.Length != 6 || users.OTP.Duration != 600 {
		t.Errorf("otp = %+v, want on, 6 digits, 600 s", users.OTP)
	}
	if users.AuthToken.Duration != 9*60*60 {
		t.Errorf("auth token duration = %d, want 32400", users.AuthToken.Duration)
	}

	u := core.NewRecord(users)
	u.Set("uid", "alice")
	u.Set("name", "Alice")
	u.Set("role", "student")
	u.SetEmail("Alice@nabu.example")
	u.SetRandomPassword()
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"alice", "ALICE", "alice@nabu.example", "Alice@nabu.example"} {
		if got := findByIdentity(app, users, id); got == nil || got.Id != u.Id {
			t.Errorf("findByIdentity(%q) = %v, want alice", id, got)
		}
	}
	if got := findByIdentity(app, users, "bob@nabu.example"); got != nil {
		t.Errorf("unknown email found %v", got.Id)
	}

	otp, pass, err := issueOTP(app, users, u)
	if err != nil || len(pass) != 6 {
		t.Fatalf("issueOTP = %v, %q, %v", otp, pass, err)
	}
	if otp.SentTo() != u.Email() || !otp.ValidatePassword(pass) {
		t.Errorf("otp sent to %q (want the email on record), password valid=%v", otp.SentTo(), otp.ValidatePassword(pass))
	}
	for i := 1; i < maxLiveOTPs; i++ {
		if _, _, err := issueOTP(app, users, u); err != nil {
			t.Fatal(err)
		}
	}
	again, pass, err := issueOTP(app, users, u)
	if err != nil || pass != "" || again == nil {
		t.Errorf("past %d live codes: want the newest re-used and nothing new to send, got pass=%q err=%v", maxLiveOTPs, pass, err)
	}
}
