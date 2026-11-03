package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// This file backs student-driven group formation. A group belongs to one
// (lesson, Session) — the Session being a schedule_sessions cohort window — and
// is keyed by (lesson, session, tag) so two Sessions never share a group (see
// docs/glossary.md). Its collection create rule is staff-only so students can't
// set max_depth/paused; this endpoint gives students a controlled way to "start a
// new group": the server creates it (in app context, bypassing the rule) with the
// default quota. It is idempotent — creating a group that already exists just
// returns it, so "start a new group" and "join an existing one" converge on the
// same (lesson, session, tag). A shared group REQUIRES a Session: unscheduled runs
// are solo (handled at job time by resolveGroup), so this endpoint rejects an
// empty session_id.

// groupCreateBody is the payload for POST /api/group/create.
type groupCreateBody struct {
	LessonID  string `json:"lesson_id"`
	SessionID string `json:"session_id"` // schedule_sessions id (the Session)
	Tag       string `json:"tag"`
}

// groupCreateHandler creates (or returns) the group for a (lesson, session, tag)
// and responds with its id/tag/lesson/session so the client can select it.
func groupCreateHandler(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	var body groupCreateBody
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	tag := normalizeTag(body.Tag)
	if body.LessonID == "" || tag == "" {
		return e.BadRequestError("lesson_id and a non-empty group name are required", nil)
	}
	// A shared group belongs to a Session; unscheduled runs are solo (never
	// created here). Require the session so two Sessions can't share a group.
	if body.SessionID == "" {
		return e.BadRequestError("a scheduled session is required to form a shared group", nil)
	}
	// the lesson must exist (a group is scoped to it).
	if _, err := e.App.FindRecordById("lessons", body.LessonID); err != nil {
		return e.BadRequestError("unknown lesson", err)
	}
	// the session must exist and belong to this lesson.
	sess, err := e.App.FindRecordById("schedule_sessions", body.SessionID)
	if err != nil {
		return e.BadRequestError("unknown session", err)
	}
	if !sessionBelongsToLesson(e.App, sess, body.LessonID) {
		return e.BadRequestError("session does not belong to this lesson", nil)
	}
	g, err := findOrCreateGroup(e.App, body.LessonID, body.SessionID, tag)
	if err != nil {
		return e.InternalServerError("could not create group", err)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"id":      g.Id,
		"tag":     g.GetString("tag"),
		"lesson":  g.GetString("lesson"),
		"session": g.GetString("session"),
	})
}

// groupStateHandler returns the caller's group id and its live server-authoritative
// state (currently images_remaining) for an attempt. The group is resolved from the
// caller's own `sessions` attempt — never from the client — mirroring resolveGroup,
// so a student can only read their own group. The image widget calls this to
// show "images left" and then subscribes to the groups record for live updates
// (decrements and teacher overrides).
func groupStateHandler(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	sessionID := e.Request.URL.Query().Get("session_id")
	if sessionID == "" {
		return e.BadRequestError("session_id required", nil)
	}
	sess, err := e.App.FindRecordById("sessions", sessionID)
	if err != nil {
		return e.NotFoundError("session not found", err)
	}
	if sess.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("not your session", nil)
	}
	g, err := resolveGroup(e.App, sess)
	if err != nil {
		return e.InternalServerError("could not resolve group", err)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"group_id":         g.Id,
		"images_remaining": g.GetInt("images_remaining"),
	})
}

// sessionBelongsToLesson checks a schedule_sessions record resolves (via its
// lesson_schedule) to the given lesson, so a client can't attach a group to a
// session from another lesson.
func sessionBelongsToLesson(app core.App, sess *core.Record, lessonID string) bool {
	sched, err := app.FindRecordById("lesson_schedules", sess.GetString("lesson_schedule"))
	if err != nil {
		return false
	}
	return sched.GetString("lesson") == lessonID
}
