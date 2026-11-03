package httpapi

import (
	"log/slog"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/demo"
	"github.com/mrmxf/nabu/internal/students"
)

// demoStatus is the public payload the login page reads to decide whether to
// show its "demo accounts still present" warning banner.
type demoStatus struct {
	// DemoMode is the app_config `demo_mode` flag (seeded from nabu-config.yaml).
	DemoMode bool `json:"demo_mode"`
	// Present lists the uids of demo-roster accounts that currently exist in the
	// users collection — regardless of DemoMode. Non-empty while DemoMode is false
	// means a demo seed left accounts behind that should be cleaned up.
	Present []string `json:"present"`
	// Logins lists the demo accounts a visitor can sign in as, for the login
	// page's hint — only while DemoMode is on. Each carries the identities and the
	// literal demo password the roster itself publishes; roster entries without a
	// literal password (and superusers) are never listed.
	Logins []demoLogin `json:"logins"`
}

// demoLogin is one well-known demo sign-in shown on the login page.
type demoLogin struct {
	Email    string `json:"email,omitempty"`
	Uid      string `json:"uid,omitempty"`
	Password string `json:"password"`
}

// demoStatusHandler answers GET /api/demo/status. It is public (unauthenticated)
// because the login page — shown before sign-in — consumes it. It reveals only
// which well-known demo uids exist, never any other user data.
func demoStatusHandler(e *core.RequestEvent) error {
	out := demoStatus{DemoMode: demoModeSetting(e.App), Present: []string{}, Logins: []demoLogin{}}

	raw, err := demo.RosterYAML()
	if err != nil {
		slog.Warn("could not read demo roster", "err", err)
		return apis.NewApiError(500, "could not read demo roster", nil)
	}
	recs, err := students.DecodeRoster(raw, students.FormatYAML)
	if err != nil {
		return apis.NewApiError(500, "could not read demo roster", nil)
	}
	for _, r := range recs {
		// Superusers live in _superusers, not `users`, and their password comes
		// from Infisical (not well-known), so a lingering one is not a credential
		// leak to warn about here — only the demo `users` matter.
		if r.Uid == "" || strings.EqualFold(r.Role, students.RoleSuperuser) {
			continue
		}
		if out.DemoMode && r.Password != "" {
			out.Logins = append(out.Logins, demoLogin{Email: r.Email, Uid: r.Uid, Password: r.Password})
		}
		if u, _ := e.App.FindFirstRecordByData("users", "uid", r.Uid); u != nil {
			out.Present = append(out.Present, r.Uid)
		}
	}
	return e.JSON(200, out)
}

// demoModeSetting reads the app_config `demo_mode` row as a bool, defaulting to
// true (the Phase 2 default) when the row is absent or unparseable.
func demoModeSetting(app core.App) bool {
	rec, err := app.FindFirstRecordByData("app_config", "key", "demo_mode")
	if err != nil || rec == nil {
		return true
	}
	return rec.GetString("value") != "false"
}
