// Package httpapi registers Nabu's custom HTTP endpoints for user
// roster export/import, full backup/restore, and course rosters. These are the
// first custom routes in the app; everything else is PocketBase's auto REST API
// or the SPA static handler. See docs/student-management.md.
package httpapi

import (
	"io"
	"net/url"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/roster"
	"github.com/mrmxf/nabu/internal/students"
)

// Register wires the custom routes onto the serve event's router. Call it inside
// the OnServe hook before the SPA catch-all.
func Register(se *core.ServeEvent) {
	se.Router.GET("/api/users/export", exportHandler).BindFunc(requireStaff)
	se.Router.POST("/api/users/import", importHandler).BindFunc(requireAdmin)
	se.Router.GET("/api/users/backup", backupHandler).BindFunc(requireAdmin)
	se.Router.POST("/api/users/restore", restoreHandler).BindFunc(requireAdmin)
	se.Router.GET("/api/courses/{slug}/roster", rosterHandler).BindFunc(requireStaff)
	// demo-account status — PUBLIC (the login page reads it before sign-in) to warn
	// when demo accounts linger while demo_mode is off. Exposes only demo uids.
	se.Router.GET("/api/demo/status", demoStatusHandler)
	// classroom fast login — PUBLIC by necessity (the caller is signed out) and
	// gated on the app_config `classroom_fast_login` flag, which is off unless a
	// teacher turns it on for a lesson. `status` reports nothing while the flag
	// is off; `login` signs a student in by name alone, creating their account on
	// first use. See internal/httpapi/classroom.go for the security tradeoff.
	se.Router.GET("/api/classroom/status", classroomStatusHandler)
	se.Router.POST("/api/classroom/login", classroomLoginHandler)
	// agent server health — readable by any signed-in student (serverstatus widget).
	se.Router.GET("/api/agent/health", agentHealthHandler).BindFunc(requireAuth)
	// job submit — any signed-in student dispatches a job for their own session.
	se.Router.POST("/api/job/submit", jobSubmitHandler).BindFunc(requireAuth)
	// group formation — a signed-in student starts (or joins) a lesson group. The
	// server sets the quota; students never create groups via the collection API.
	se.Router.POST("/api/group/create", groupCreateHandler).BindFunc(requireAuth)
	// live per-group state for the caller's attempt (e.g. images_remaining); the
	// widget then subscribes to the groups record for realtime updates.
	se.Router.GET("/api/group/state", groupStateHandler).BindFunc(requireAuth)
	// group collaborative document (Yjs) — a student pushes/pulls updates for
	// their own group's room; room (lesson+group) is derived from the session
	// (see internal/httpapi/ydoc.go). Realtime fan-out rides the ydoc_updates
	// collection subscription.
	se.Router.POST("/api/ydoc/push", ydocPushHandler).BindFunc(requireAuth)
	se.Router.GET("/api/ydoc/pull", ydocPullHandler).BindFunc(requireAuth)
	// teacher-scoped synchronized timer — the first student to reach a slide that
	// declares a `timers:` (scope teacher) entry creates the shared countdown for
	// their cohort; everyone else reads the same end_time and subscribes to the
	// session_timers record for realtime pause/extend/reset (see timers.go).
	se.Router.POST("/api/session/timer/ensure", timerEnsureHandler).BindFunc(requireAuth)
	// captures — a student snapshots the derived data (t_mapper output) their
	// session worked on, on navigation/exit (see internal/httpapi/captures.go).
	se.Router.POST("/api/captures/set", capturesSetHandler).BindFunc(requireAuth)
	// lesson resource store (S3) — staff only. Bucket/prefix come from the lesson
	// record; keys are server-derived; nabu holds the only credential.
	se.Router.POST("/api/resources/presign", resourcePresignHandler).BindFunc(requireStaff)
	se.Router.POST("/api/resources/confirm", resourceConfirmHandler).BindFunc(requireStaff)
	se.Router.POST("/api/resources/import-url", resourceImportURLHandler).BindFunc(requireStaff)
	se.Router.GET("/api/resources", resourceListHandler).BindFunc(requireStaff)
	// course-global resources folder (bucket-root resources/) — read-only course
	// assets, so any authenticated user (incl. students) may list it.
	se.Router.GET("/api/resources/global", resourceGlobalListHandler).BindFunc(requireAuth)
	se.Router.GET("/api/resources/usage", resourceUsageHandler).BindFunc(requireStaff)
	se.Router.POST("/api/resources/delete", resourceDeleteHandler).BindFunc(requireStaff)
	se.Router.POST("/api/resources/rename", resourceRenameHandler).BindFunc(requireStaff)
	// multipart direct upload for large files (>~part size): create -> sign each
	// part (browser PUTs direct to S3) -> complete, or abort on failure.
	se.Router.POST("/api/resources/multipart/create", resourceMultipartCreateHandler).BindFunc(requireStaff)
	se.Router.POST("/api/resources/multipart/sign", resourceMultipartSignHandler).BindFunc(requireStaff)
	se.Router.POST("/api/resources/multipart/complete", resourceMultipartCompleteHandler).BindFunc(requireStaff)
	se.Router.POST("/api/resources/multipart/abort", resourceMultipartAbortHandler).BindFunc(requireStaff)
	// student resource store — a student's own work + their group's shared work.
	// The bucket, lesson and group are derived from the caller's session record.
	se.Router.GET("/api/student/resources", studentListHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/presign", studentPresignHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/confirm", studentConfirmHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/import-url", studentImportURLHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/delete", studentDeleteHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/rename", studentRenameHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/multipart/create", studentMultipartCreateHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/multipart/sign", studentMultipartSignHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/multipart/complete", studentMultipartCompleteHandler).BindFunc(requireAuth)
	se.Router.POST("/api/student/resources/multipart/abort", studentMultipartAbortHandler).BindFunc(requireAuth)
}

// ---- guards ----------------------------------------------------------------

func isAdmin(e *core.RequestEvent) bool {
	return e.HasSuperuserAuth() || (e.Auth != nil && e.Auth.GetString("role") == "admin")
}

func isTeacher(e *core.RequestEvent) bool {
	return e.Auth != nil && e.Auth.GetString("role") == "teacher"
}

func requireAdmin(e *core.RequestEvent) error {
	if isAdmin(e) {
		return e.Next()
	}
	return apis.NewForbiddenError("admin only", nil)
}

func requireStaff(e *core.RequestEvent) error {
	if isAdmin(e) || isTeacher(e) {
		return e.Next()
	}
	return apis.NewForbiddenError("admin or teacher only", nil)
}

// ---- handlers --------------------------------------------------------------

// courseParam reads the `course` query parameter, falling back to the
// deprecated pre-rename `class` spelling so older scripts keep working.
func courseParam(q url.Values) string {
	if c := q.Get("course"); c != "" {
		return c
	}
	return q.Get("class")
}

func exportHandler(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	format := students.Format(q.Get("format"))
	if format == "" {
		format = students.FormatCSV
	}
	filter := students.ExportFilter{Role: q.Get("role"), Course: courseParam(q)}
	// A teacher is scoped to the courses they teach; an admin is unrestricted.
	if !isAdmin(e) {
		scope, err := students.TeacherCourses(e.App, e.Auth.Id)
		if err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		filter.TeacherScope = scope
	}
	recs, err := students.Export(e.App, filter, q.Get("positions") != "")
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	data, err := students.EncodeRoster(recs, format)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	return e.Blob(200, contentType(format), data)
}

func importHandler(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	format := students.Format(q.Get("format"))
	if format == "" {
		format = students.FormatFromContentType(e.Request.Header.Get("Content-Type"))
	}
	if format == "" {
		format = students.FormatCSV
	}
	body, err := io.ReadAll(e.Request.Body)
	if err != nil {
		return apis.NewBadRequestError("could not read body", nil)
	}
	recs, err := students.DecodeRoster(body, format)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	mode := students.Mode(q.Get("mode"))
	if mode == "" {
		mode = students.ModeMerge
	}
	sum, err := students.Apply(e.App, recs, mode, courseParam(q))
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	return e.JSON(200, sum)
}

func backupHandler(e *core.RequestEvent) error {
	format := students.Format(e.Request.URL.Query().Get("format"))
	if format == "" {
		format = students.FormatYAML
	}
	b, err := students.Backup(e.App)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	data, err := students.EncodeBundle(b, format)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	return e.Blob(200, contentType(format), data)
}

func restoreHandler(e *core.RequestEvent) error {
	format := students.Format(e.Request.URL.Query().Get("format"))
	if format == "" {
		format = students.FormatFromContentType(e.Request.Header.Get("Content-Type"))
	}
	if format == "" {
		format = students.FormatYAML
	}
	body, err := io.ReadAll(e.Request.Body)
	if err != nil {
		return apis.NewBadRequestError("could not read body", nil)
	}
	bundle, err := students.DecodeBundle(body, format)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	sum, err := students.Restore(e.App, bundle)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	return e.JSON(200, sum)
}

func rosterHandler(e *core.RequestEvent) error {
	slug := e.Request.PathValue("slug")
	// teachers may only view their own course's roster.
	if !isAdmin(e) {
		scope, err := students.TeacherCourses(e.App, e.Auth.Id)
		if err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		if !contains(scope, slug) {
			return apis.NewForbiddenError("not your course", nil)
		}
	}
	r, err := roster.CourseRoster(e.App, slug)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	return e.JSON(200, r)
}

func contentType(f students.Format) string {
	switch f {
	case students.FormatCSV:
		return "text/csv; charset=utf-8"
	case students.FormatJSON:
		return "application/json; charset=utf-8"
	default:
		return "application/yaml; charset=utf-8"
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
