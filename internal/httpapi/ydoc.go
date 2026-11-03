package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// This file backs the per-group collaborative Yjs document (see the group
// collaboration plan). A room is a `${lesson}:${group}` pair; the server derives
// it from the caller's session — never from the client — so a student can only
// read/write their own group's document. The stored form is an append-only log
// of base64 Yjs updates in the `ydoc_updates` collection; PocketBase realtime
// fans new rows out to every group member, whose Yjs provider applies them.

// ydocRoom builds the room name for a (lesson, session, group). Including the
// Session is what keeps two Sessions of one lesson from sharing a room, so group
// state (notes/choices/agent-config) never leaks across offerings. A solo attempt
// has an empty session and a `solo-<attemptId>` group, giving it a stable private
// room. Keep this the single source of truth so the provider agrees on the key.
func ydocRoom(lesson, session, group string) string {
	return lesson + ":" + session + ":" + group
}

// ydocPushBody is the payload for POST /api/ydoc/push: one base64 Yjs update.
type ydocPushBody struct {
	SessionID string `json:"session_id"`
	Update    string `json:"update"` // base64-encoded Yjs update
}

// ydocPushHandler appends one Yjs update to the caller's room. The room is
// derived from the session (group+lesson), so a student can only contribute to
// their own group's document. The created row is broadcast by PB realtime.
func ydocPushHandler(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	var body ydocPushBody
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.SessionID == "" || body.Update == "" {
		return e.BadRequestError("session_id and update are required", nil)
	}

	app := e.App
	group, lesson, session, herr := resolveSessionGroup(e, body.SessionID)
	if herr != nil {
		return herr
	}

	col, err := app.FindCollectionByNameOrId("ydoc_updates")
	if err != nil {
		return e.InternalServerError("ydoc_updates collection missing", err)
	}
	rec := core.NewRecord(col)
	rec.Set("room", ydocRoom(lesson, session, group))
	rec.Set("update", body.Update)
	if err := app.Save(rec); err != nil {
		return e.InternalServerError("could not save ydoc update", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// ydocPullHandler returns every stored update for the caller's room, oldest
// first, so the client can rebuild its Y.Doc on load. The Yjs merge is
// order-independent, but oldest-first keeps a later snapshot row (from
// compaction) applying before the deltas that followed it.
func ydocPullHandler(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	group, lesson, session, herr := resolveSessionGroup(e, e.Request.URL.Query().Get("session_id"))
	if herr != nil {
		return herr
	}
	room := ydocRoom(lesson, session, group)
	recs, err := e.App.FindRecordsByFilter("ydoc_updates",
		"room = {:r}", "created", 0, 0,
		map[string]any{"r": room})
	if err != nil {
		return e.InternalServerError("could not list ydoc updates", err)
	}
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.GetString("update"))
	}
	// Return the (server-derived) room so the client can filter the realtime
	// stream to its own group and never apply another group's updates.
	return e.JSON(http.StatusOK, map[string]any{"room": room, "updates": out})
}

// resolveSessionGroup loads an attempt (the `sessions` record) that must belong
// to the caller and returns its (normalized group tag, lesson id, Session id).
// The group tag never comes from the client, so a student can only touch their
// own group. The Session id (schedule_session) scopes the Yjs room so two
// Sessions of one lesson never share a room (see docs/glossary.md); it is empty
// for an unscheduled/solo attempt.
func resolveSessionGroup(e *core.RequestEvent, sessionID string) (group, lesson, session string, err error) {
	if sessionID == "" {
		return "", "", "", e.BadRequestError("session_id required", nil)
	}
	sess, ferr := e.App.FindRecordById("sessions", sessionID)
	if ferr != nil {
		return "", "", "", e.NotFoundError("session not found", ferr)
	}
	if sess.GetString("user") != e.Auth.Id {
		return "", "", "", e.ForbiddenError("not your session", nil)
	}
	lesson = sess.GetString("lesson")
	session = sess.GetString("schedule_session")
	group = normalizeTag(sess.GetString("group_name"))
	// Mirror resolveGroup: an unscheduled attempt or one with no group name is
	// SOLO — a private, per-attempt room (session empty) so the student's own
	// notes/choices still persist, but nothing is ever shared.
	if session == "" || group == "" {
		return "solo-" + sess.Id, lesson, "", nil
	}
	return group, lesson, session, nil
}
