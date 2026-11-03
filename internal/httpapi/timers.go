package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// This file backs the teacher-scoped synchronized timer (session_timers). A timer
// is declared in a slide's `timers:` frontmatter (id + scope + label + seconds)
// and shared cohort-wide: the FIRST student to reach a declaring slide creates the
// row here with an absolute `end_time`, and every other student in the same
// Session then reads the same deadline over PocketBase realtime, so all clocks
// agree regardless of skew. Teachers pause/resume/extend/reset by PATCHing the
// record directly (the collection's staff UpdateRule); that path needs no handler.
//
// Only the create path lives server-side: students may not write session_timers
// via the collection API (staff-only rules), so ensure runs in app context, which
// bypasses the CreateRule — exactly like a job submission creating its record.

// timerEnsureBody is the payload for POST /api/session/timer/ensure.
type timerEnsureBody struct {
	SessionID string `json:"session_id"` // the caller's attempt (sessions record) id
	TimerID   string `json:"timer_id"`   // the frontmatter timer id
	Seconds   int    `json:"seconds"`    // authored duration; used only on first create
	Label     string `json:"label"`
}

// timerEnsureHandler returns the shared timer for the caller's cohort + timer id,
// creating and starting it on first touch. The cohort (schedule_session) is
// derived from the caller's own attempt — never trusted from the client — so a
// student can only ever touch their own cohort's timer. An unscheduled/solo
// attempt has no cohort to share, so it gets {shared:false} and the client falls
// back to a private local countdown.
func timerEnsureHandler(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	var body timerEnsureBody
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.SessionID == "" || body.TimerID == "" {
		return e.BadRequestError("session_id and timer_id are required", nil)
	}

	app := e.App
	sess, err := app.FindRecordById("sessions", body.SessionID)
	if err != nil {
		return e.NotFoundError("session not found", err)
	}
	if sess.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("not your session", nil)
	}
	cohort := sess.GetString("schedule_session")
	if cohort == "" {
		// Unscheduled/solo attempt: no cohort to share a timer with.
		return e.JSON(http.StatusOK, map[string]any{"shared": false})
	}

	rec, err := findOrCreateSessionTimer(app, cohort, body.TimerID, body.Label, body.Seconds)
	if err != nil {
		return e.InternalServerError("could not resolve timer", err)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"shared":       true,
		"id":           rec.Id,
		"running":      rec.GetBool("running"),
		"end_time":     rec.GetDateTime("end_time").Time().UTC().Format(time.RFC3339Nano),
		"remaining_ms": rec.GetInt("remaining_ms"),
		"server_now":   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// findOrCreateSessionTimer returns the timer for (cohort, timerID), creating it
// running with an absolute end_time on first use. Idempotent and race-safe (the
// unique (session, timer_id) index makes a lost create re-fetch the winner), so
// re-entering a declaring slide never restarts the clock. Duration comes from the
// first student's authored seconds; later callers inherit whatever was started.
func findOrCreateSessionTimer(app core.App, cohort, timerID, label string, seconds int) (*core.Record, error) {
	params := map[string]any{"s": cohort, "t": timerID}
	filter := "session = {:s} && timer_id = {:t}"
	if existing, err := app.FindFirstRecordByFilter("session_timers", filter, params); err == nil {
		return existing, nil
	}
	col, err := app.FindCollectionByNameOrId("session_timers")
	if err != nil {
		return nil, err
	}
	if seconds <= 0 {
		seconds = 0
	}
	rec := core.NewRecord(col)
	rec.Set("session", cohort)
	rec.Set("timer_id", timerID)
	rec.Set("label", label)
	rec.Set("duration_seconds", seconds)
	rec.Set("running", true)
	rec.Set("end_time", types.NowDateTime().Add(time.Duration(seconds)*time.Second))
	rec.Set("remaining_ms", seconds*1000)
	if err := app.Save(rec); err != nil {
		// Lost a create race: re-fetch the winner.
		if again, e2 := app.FindFirstRecordByFilter("session_timers", filter, params); e2 == nil {
			return again, nil
		}
		return nil, err
	}
	return rec, nil
}
