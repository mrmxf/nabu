package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"
	"github.com/pocketbase/pocketbase/tools/routine"
	"github.com/pocketbase/pocketbase/tools/security"
)

// otpRequestBody is POST /api/auth/otp: a username (uid) or an email.
type otpRequestBody struct {
	Identity string `json:"identity"`
}

// maxLiveOTPs is how many unexpired codes one account may hold; past it the
// newest is re-sent rather than a new one minted (as PocketBase does).
const maxLiveOTPs = 10

// otpRequestHandler emails a sign-in code to the account named by a username or
// an email. The code always goes to the email ON RECORD, never to the typed
// text. The reply is {otpId} whether or not the account exists, so the endpoint
// does not reveal who is on the roster; the browser then calls PocketBase's
// own POST /api/collections/users/auth-with-otp with that id and the code,
// which limits guesses (5 per account per 3 minutes).
//
// PocketBase's built-in request-otp takes an email only; this one exists so a
// student can type the username they know.
func otpRequestHandler(e *core.RequestEvent) error {
	users, err := e.App.FindCollectionByNameOrId("users")
	if err != nil {
		return e.InternalServerError("", err)
	}
	if !users.OTP.Enabled {
		return e.ForbiddenError("sign-in codes are not enabled", nil)
	}
	var body otpRequestBody
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	identity := strings.TrimSpace(body.Identity)
	if identity == "" || len(identity) > 255 {
		return e.BadRequestError("enter your username or email", nil)
	}

	user := findByIdentity(e.App, users, identity)
	if user == nil || user.Email() == "" {
		// same shape as a real reply: a random id that matches no code.
		slog.Info("sign-in code not sent: no account with an email", "identity_kind", identityKind(identity))
		return e.JSON(http.StatusOK, map[string]string{"otpId": core.GenerateDefaultRandomId()})
	}

	otp, fresh, err := issueOTP(e.App, users, user)
	if err != nil {
		return e.InternalServerError("could not create a sign-in code", err)
	}
	if fresh != "" {
		app := e.App
		routine.FireAndForget(func() {
			if err := mails.SendRecordOTP(app, user, otp.Id, fresh); err != nil {
				slog.Warn("sign-in code not sent", "uid", user.GetString("uid"), "err", err)
				_ = app.Delete(otp)
			}
		})
	}
	slog.Info("sign-in code requested", "uid", user.GetString("uid"), "new", fresh != "")
	return e.JSON(http.StatusOK, map[string]string{"otpId": otp.Id})
}

// findByIdentity finds a user by email (case-insensitive) or by uid.
func findByIdentity(app core.App, users *core.Collection, identity string) *core.Record {
	if strings.Contains(identity, "@") {
		if r, err := app.FindAuthRecordByEmail(users, identity); err == nil {
			return r
		}
		if r, err := app.FindFirstRecordByFilter(users, "email:lower = {:e}", map[string]any{"e": strings.ToLower(identity)}); err == nil {
			return r
		}
		return nil
	}
	if r, err := app.FindFirstRecordByData(users, "uid", identity); err == nil {
		return r
	}
	if r, err := app.FindFirstRecordByData(users, "uid", strings.ToLower(identity)); err == nil {
		return r
	}
	return nil
}

// issueOTP creates a code for user and returns it with its plain password, or,
// when the account already holds maxLiveOTPs live codes, returns the newest one
// and an empty password (nothing new to send).
func issueOTP(app core.App, users *core.Collection, user *core.Record) (*core.OTP, string, error) {
	otps, err := app.FindAllOTPsByRecord(user)
	if err != nil {
		return nil, "", err
	}
	live := 0
	for _, o := range otps {
		if !o.HasExpired(users.OTP.DurationTime()) {
			live++
		}
	}
	if live >= maxLiveOTPs && len(otps) > 0 {
		return otps[0], "", nil // DESC by created
	}
	pass := security.RandomStringWithAlphabet(users.OTP.Length, "1234567890")
	otp := core.NewOTP(app)
	otp.SetCollectionRef(users.Id)
	otp.SetRecordRef(user.Id)
	otp.SetSentTo(user.Email())
	otp.SetPassword(pass)
	if err := app.Save(otp); err != nil {
		return nil, "", errors.Join(errors.New("save otp"), err)
	}
	return otp, pass, nil
}

// identityKind says what kind of identity was typed, for logs that must not
// record the identity itself.
func identityKind(s string) string {
	if strings.Contains(s, "@") {
		return "email"
	}
	return "username"
}
