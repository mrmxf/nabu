// Package hooks wires PocketBase event hooks for the Nabu server.
package hooks

import (
	"fmt"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/serverctl"
)

// BindActivity records a row in the `activity` collection on each successful
// user authentication. This gives admins a queryable audit trail — e.g.
// "on date X between Y and Z, who was active" — independent of PocketBase's
// internal auth logs. Failures are logged but never block the login.
func BindActivity(app core.App) {
	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if err := writeActivity(e.App, e.Record.Id, "login"); err != nil {
			slog.Warn("could not record login activity", "user", e.Record.Id, "err", err)
		}
		return e.Next()
	})
}

// BindSessionActivity records a "session_start" row when a student creates a
// new session, so the activity trail captures both logins and session starts.
func BindSessionActivity(app core.App) {
	app.OnRecordCreateRequest("sessions").BindFunc(func(e *core.RecordRequestEvent) error {
		// let the create proceed first so the record has a persisted id.
		if err := e.Next(); err != nil {
			return err
		}
		userID := e.Record.GetString("user")
		if err := writeSessionActivity(e.App, userID, e.Record.Id); err != nil {
			slog.Warn("could not record session_start activity", "session", e.Record.Id, "err", err)
		}
		serverctl.IncSessionsServed()
		return nil
	})
}

// writeSessionActivity saves a session_start event linked to the session.
func writeSessionActivity(app core.App, userID, sessionID string) error {
	col, err := app.FindCollectionByNameOrId("activity")
	if err != nil {
		return fmt.Errorf("find activity collection: %w", err)
	}
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("event", "session_start")
	rec.Set("session", sessionID)
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("save activity: %w", err)
	}
	return nil
}

// writeActivity saves an activity event. It runs in superuser context (direct
// DB save), so it bypasses the collection's superuser-only API rules.
func writeActivity(app core.App, userID, event string) error {
	col, err := app.FindCollectionByNameOrId("activity")
	if err != nil {
		return fmt.Errorf("find activity collection: %w", err)
	}
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("event", event)
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("save activity: %w", err)
	}
	return nil
}
