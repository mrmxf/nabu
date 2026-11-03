package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// This file captures the derived data a student/group worked on — the output of a
// t_mapper (e.g. the table of rows selected by their choice). The client posts a
// snapshot on navigation/exit; the server derives group+lesson from the session
// (never the client) and upserts one row per (session, key_id), storing minified
// JSON. See docs/lesson-choice-values.md.

// captureSetBody is the payload for POST /api/captures/set.
type captureSetBody struct {
	SessionID string `json:"session_id"`
	Items     []struct {
		ID    string          `json:"id"`    // the t_mapper id
		Scope string          `json:"scope"` // session|group (inherited from the driving choice)
		Value json.RawMessage `json:"value"` // the derived data (minified by the client)
	} `json:"items"`
}

// capturesSetHandler upserts the caller's derived-data snapshot, one row per
// (session, mapper id). It overwrites (the capture is the latest snapshot).
func capturesSetHandler(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	var body captureSetBody
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	app := e.App
	if body.SessionID == "" {
		return e.BadRequestError("session_id required", nil)
	}
	sess, ferr := app.FindRecordById("sessions", body.SessionID)
	if ferr != nil {
		return e.NotFoundError("session not found", ferr)
	}
	if sess.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("not your session", nil)
	}
	group := strings.TrimSpace(sess.GetString("group_name")) // optional for captures
	lesson := sess.GetString("lesson")

	col, err := app.FindCollectionByNameOrId("captures")
	if err != nil {
		return e.InternalServerError("captures collection missing", err)
	}
	for _, it := range body.Items {
		if it.ID == "" {
			continue
		}
		rec, _ := app.FindFirstRecordByFilter("captures",
			"session = {:s} && key_id = {:k}",
			map[string]any{"s": body.SessionID, "k": it.ID})
		if rec == nil {
			rec = core.NewRecord(col)
			rec.Set("session", body.SessionID)
			rec.Set("lesson", lesson)
			rec.Set("key_id", it.ID)
		}
		rec.Set("group", group)
		rec.Set("scope", it.Scope)
		rec.Set("value", it.Value)
		if err := app.Save(rec); err != nil {
			return e.InternalServerError("could not save capture", err)
		}
	}
	return e.JSON(http.StatusOK, map[string]any{"ok": true, "count": len(body.Items)})
}
