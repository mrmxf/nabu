package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/mrmxf/nabu/internal/students"
)

// This file backs "classroomFastLogin" — a deliberate stopgap for live lessons.
// The normal path (sign in with a password, or sign up with username + name +
// email + password, then pick course/lesson/session/group) costs a classroom far
// too much time. While the app_config `classroom_fast_login` flag is on AND a
// Session (a schedule_sessions cohort window) is inside its enrolment window, a
// student types ONLY their name: the account is created on first use and they
// are signed straight in, landing on the group chooser (/classroom/group).
//
// The uid namespace is `<name-slug>-<session-slug>`, so two students called
// "sam" in two different Sessions get two independent accounts and never
// collide. A returning student typing the same name in the same Session gets
// their existing account back (same uid) — no duplicate, work preserved.
//
// TRADEOFF — with no password, anyone who can reach the site can join the open
// Session under any name, including a classmate's. That is inherent to
// "username only". The mitigations, all enforced here on the server:
//   - OFF by default; it must be switched on deliberately for the lesson;
//   - it only works while a Session is enrollable, so it closes itself;
//   - it can never authenticate staff: a matched record whose role is not
//     "student" is refused, and the `-<session-slug>` suffix keeps the
//     fast-login namespace disjoint from real, password-protected accounts.
//
// See docs/classroom-fast-login.md.

// classroomSession is one currently-enrollable Session, flattened with the
// lesson/course it belongs to so a signed-out login page can label it without
// reading the (authed-only) schedule collections itself.
type classroomSession struct {
	// ID is the schedule_sessions record id (the `session_id` the client posts back).
	ID string `json:"id"`
	// Name is the teacher-facing Session label, Slug its uid-safe form — the
	// suffix that scopes every account created for this Session.
	Name string `json:"name"`
	Slug string `json:"slug"`
	// CourseID/LessonID are what the group page needs to create the attempt;
	// Course/Lesson are their titles, for the picker label.
	CourseID string `json:"course_id"`
	Course   string `json:"course"`
	LessonID string `json:"lesson_id"`
	Lesson   string `json:"lesson"`
}

// classroomStatus is the public payload the login page reads to decide whether
// to show the name-only form, and which Sessions to offer.
type classroomStatus struct {
	Enabled  bool               `json:"enabled"`
	Sessions []classroomSession `json:"sessions"`
}

// classroomLoginBody is the payload for POST /api/classroom/login.
type classroomLoginBody struct {
	Username  string `json:"username"`
	SessionID string `json:"session_id"` // schedule_sessions id (the Session)
}

// classroomStatusHandler answers GET /api/classroom/status. It is public
// (unauthenticated) because the login page — shown before sign-in — consumes it.
// With the flag off it reports nothing at all, so a normal deployment leaks no
// schedule data to anonymous callers.
func classroomStatusHandler(e *core.RequestEvent) error {
	out := classroomStatus{Enabled: classroomEnabled(e.App), Sessions: []classroomSession{}}
	if !out.Enabled {
		return e.JSON(http.StatusOK, out)
	}
	sessions, err := enrollableSessions(e.App)
	if err != nil {
		return e.InternalServerError("could not read the schedule", err)
	}
	out.Sessions = sessions
	return e.JSON(http.StatusOK, out)
}

// classroomLoginHandler answers POST /api/classroom/login: it resolves (or
// creates) the student account for (name, Session) and mints an auth token for
// it without any password check. Public by necessity — the caller is signed out.
func classroomLoginHandler(e *core.RequestEvent) error {
	if !classroomEnabled(e.App) {
		return e.ForbiddenError("classroom fast login is not enabled", nil)
	}
	var body classroomLoginBody
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	handle := students.Slugify(body.Username)
	if handle == "" {
		return e.BadRequestError("please enter your name", nil)
	}

	// The Session must be one that is open RIGHT NOW. Checking against the live
	// enrollable set (rather than trusting the posted id) is what implements
	// "no open session => no login", and stops a stale browser tab enrolling
	// into a window that has since closed.
	sessions, err := enrollableSessions(e.App)
	if err != nil {
		return e.InternalServerError("could not read the schedule", err)
	}
	var session *classroomSession
	for i := range sessions {
		if sessions[i].ID == body.SessionID {
			session = &sessions[i]
			break
		}
	}
	if session == nil {
		return e.BadRequestError("no course is open for that session right now", nil)
	}

	uid := handle + "-" + session.Slug
	user, created, err := findOrCreateClassroomUser(e.App, uid, handle)
	if err != nil {
		return e.InternalServerError("could not sign you in", err)
	}
	if user == nil {
		// The uid is taken by a non-student (teacher/admin). Never mint a token
		// for a privileged account on a password-less path.
		return e.ForbiddenError("that name is reserved — use the staff sign-in", nil)
	}
	// Put them on the course roster so the teacher's views and exports see them.
	// Best-effort: a roster gap must not block a student from joining the lesson.
	if err := ensureEnrolment(e.App, user.Id, session.CourseID); err != nil {
		slog.Warn("classroom fast login: could not enrol student in course",
			"uid", uid, "course", session.CourseID, "err", err)
	}

	slog.Info("classroom fast login",
		"uid", uid, "session", session.ID, "lesson", session.LessonID, "created", created)

	// Mint the standard auth response — same {token, record} body as
	// authWithPassword, so the SPA stores it identically and the existing
	// OnRecordAuthRequest("users") hook still records the login in `activity`.
	return apis.RecordAuthResponse(e, user, "password", nil)
}

// findOrCreateClassroomUser returns the student account for uid, creating it on
// first use. It returns (nil, false, nil) — not an error — when the uid exists
// but belongs to a non-student, so the caller can answer 403 rather than 500.
//
// A created account has NO email (optional since migration 1751000017) and a
// random password nobody ever sees: PocketBase forces the password field on an
// auth collection to be required with a minimum length, so a literally blank
// password cannot be stored. The student is never asked for one, and the side
// effect is that the account is reachable only through this endpoint.
func findOrCreateClassroomUser(app core.App, uid, name string) (*core.Record, bool, error) {
	if existing, _ := app.FindFirstRecordByData("users", "uid", uid); existing != nil {
		if existing.GetString("role") != "student" {
			return nil, false, nil
		}
		return existing, false, nil
	}
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, false, fmt.Errorf("find users collection: %w", err)
	}
	pw, err := randomPassword()
	if err != nil {
		return nil, false, err
	}
	// Saving in app context bypasses the users CreateRule (see migration
	// 1751000016_user_signup.go), which exists to stop a self-signup granting
	// itself a staff role — a role we set ourselves here.
	user := core.NewRecord(col)
	user.Set("uid", uid)
	user.Set("name", name)
	user.Set("role", "student")
	user.Set("verified", true)
	user.SetPassword(pw)
	if err := app.Save(user); err != nil {
		// Lost a create race with a classmate typing the same name at the same
		// moment: re-fetch the winner rather than failing the sign-in.
		if again, _ := app.FindFirstRecordByData("users", "uid", uid); again != nil {
			if again.GetString("role") != "student" {
				return nil, false, nil
			}
			return again, false, nil
		}
		return nil, false, fmt.Errorf("save user: %w", err)
	}
	return user, true, nil
}

// ensureEnrolment idempotently puts a user on a course roster as a student,
// mirroring students.reconcileEnrolments. The (user, course) unique index means a
// lost race is harmless — the row is already there.
func ensureEnrolment(app core.App, userID, courseID string) error {
	if courseID == "" {
		return nil
	}
	existing, _ := app.FindFirstRecordByFilter("enrolments", "user = {:u} && course = {:c}",
		map[string]any{"u": userID, "c": courseID})
	if existing != nil {
		return nil
	}
	col, err := app.FindCollectionByNameOrId("enrolments")
	if err != nil {
		return fmt.Errorf("find enrolments collection: %w", err)
	}
	en := core.NewRecord(col)
	en.Set("user", userID)
	en.Set("course", courseID)
	en.Set("role_in_course", "student")
	if err := app.Save(en); err != nil {
		return fmt.Errorf("enrol: %w", err)
	}
	return nil
}

// enrollableSessions returns every Session whose enrolment window contains NOW,
// newest first, flattened with the lesson and course it belongs to. An unset
// enroll_from/enroll_to bound is open-ended on that side — the same rule the
// student picker applies client-side (see nabu/web/src/routes/+page.svelte).
//
// This is the single server-side source of truth for "which Sessions are open",
// used both to render the login page and to authorize a fast login. It runs on
// the server because schedule_sessions is readable only by signed-in users and
// the caller here is, by definition, signed out.
func enrollableSessions(app core.App) ([]classroomSession, error) {
	recs, err := app.FindRecordsByFilter("schedule_sessions", "", "-start_from", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list schedule_sessions: %w", err)
	}
	now := types.NowDateTime()
	out := []classroomSession{}
	// Lessons and courses repeat across Sessions; cache so a full schedule costs
	// a couple of reads, not one per row.
	lessons := map[string]*core.Record{}
	courses := map[string]*core.Record{}

	for _, rec := range recs {
		if !windowContains(rec.GetDateTime("enroll_from"), rec.GetDateTime("enroll_to"), now) {
			continue
		}
		lesson := resolveSessionLesson(app, rec, lessons)
		if lesson == nil {
			// A Session whose lesson has gone is unusable — skip rather than
			// offering a login that cannot start an attempt.
			slog.Warn("classroom: enrollable session has no resolvable lesson", "session", rec.Id)
			continue
		}
		courseID := lesson.GetString("course")
		course := courses[courseID]
		if course == nil {
			course, _ = app.FindRecordById("courses", courseID)
			if course != nil {
				courses[courseID] = course
			}
		}
		cs := classroomSession{
			ID:       rec.Id,
			Name:     rec.GetString("name"),
			Slug:     students.Slugify(rec.GetString("name")),
			CourseID: courseID,
			LessonID: lesson.Id,
			Lesson:   lesson.GetString("title"),
		}
		if course != nil {
			cs.Course = course.GetString("title")
		}
		// A Session with an unusable name would produce the uid suffix "" and
		// silently merge two cohorts into one namespace — fall back to its id.
		if cs.Slug == "" {
			cs.Slug = rec.Id
		}
		out = append(out, cs)
	}
	return out, nil
}

// resolveSessionLesson walks schedule_sessions -> lesson_schedules -> lessons,
// memoising the lesson by its lesson_schedules id.
func resolveSessionLesson(app core.App, session *core.Record, cache map[string]*core.Record) *core.Record {
	schedID := session.GetString("lesson_schedule")
	if schedID == "" {
		return nil
	}
	if hit, ok := cache[schedID]; ok {
		return hit
	}
	var lesson *core.Record
	if sched, err := app.FindRecordById("lesson_schedules", schedID); err == nil {
		lesson, _ = app.FindRecordById("lessons", sched.GetString("lesson"))
	}
	cache[schedID] = lesson
	return lesson
}

// windowContains reports whether now falls inside [from, to]. A zero bound is
// open-ended on that side, so a Session with neither bound is always open.
func windowContains(from, to, now types.DateTime) bool {
	if !from.IsZero() && now.Before(from) {
		return false
	}
	if !to.IsZero() && now.After(to) {
		return false
	}
	return true
}

// classroomEnabled reads the app_config `classroom_fast_login` row as a bool.
// Unlike demo_mode it defaults to FALSE: a missing or unreadable row must never
// open a password-less sign-in path.
func classroomEnabled(app core.App) bool {
	rec, err := app.FindFirstRecordByData("app_config", "key", "classroom_fast_login")
	if err != nil || rec == nil {
		return false
	}
	return rec.GetString("value") == "true"
}

// randomPassword returns a 32-hex-char password for a fast-login account. It is
// never shown to anyone: it exists only because PocketBase requires auth records
// to carry a password, and its unguessability is what keeps these accounts off
// the normal password sign-in form.
func randomPassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return hex.EncodeToString(b), nil
}
