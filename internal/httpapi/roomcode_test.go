package httpapi

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// TestRoomCodeMatches: a code is 6 digits, differs per course, and is accepted
// in its own step and the next one, never two steps later.
func TestRoomCodeMatches(t *testing.T) {
	now := time.Date(2026, 11, 16, 15, 4, 0, 0, time.UTC)
	step := roomCodeStepAt(now)
	a := roomCodeFor("secret", "courseA", step)
	if len(a) != roomCodeDigits {
		t.Fatalf("code %q is not %d digits", a, roomCodeDigits)
	}
	if a == roomCodeFor("secret", "courseB", step) {
		t.Error("two courses share a code")
	}
	if a == roomCodeFor("other", "courseA", step) {
		t.Error("the secret does not change the code")
	}
	if !roomCodeMatches("secret", "courseA", a, now) {
		t.Error("current code refused")
	}
	if !roomCodeMatches("secret", "courseA", a, now.Add(roomCodeStep)) {
		t.Error("previous step's code refused")
	}
	if roomCodeMatches("secret", "courseA", a, now.Add(2*roomCodeStep)) {
		t.Error("a code two steps old accepted")
	}
	if roomCodeMatches("secret", "courseB", a, now) {
		t.Error("course A's code accepted for course B")
	}
}

// TestRoomCodeLimiter: the sixth try inside the window is blocked; old
// failures age out.
func TestRoomCodeLimiter(t *testing.T) {
	now := time.Now()
	key := "limiter-test"
	for i := 0; i < roomCodeMaxFail; i++ {
		if roomCodeBlocked(key, now) {
			t.Fatalf("blocked after %d failures", i)
		}
		roomCodeFail(key, now)
	}
	if !roomCodeBlocked(key, now) {
		t.Error("not blocked after the limit")
	}
	if roomCodeBlocked(key, now.Add(roomCodeFailWin+time.Second)) {
		t.Error("still blocked after the window")
	}
}

// TestOpenCourses: enrollable Sessions are grouped by course; closed ones and
// the disabled switch are respected.
func TestOpenCourses(t *testing.T) {
	app := newMigratedApp(t)
	save := func(collection string, set map[string]any) *core.Record {
		t.Helper()
		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatal(err)
		}
		r := core.NewRecord(col)
		for k, v := range set {
			r.Set(k, v)
		}
		if err := app.Save(r); err != nil {
			t.Fatalf("save %s: %v", collection, err)
		}
		return r
	}
	course := save("courses", map[string]any{"title": "C", "slug": "c"})
	lesson := save("lessons", map[string]any{"course": course.Id, "title": "L", "slug": "l"})
	sched := save("lesson_schedules", map[string]any{"lesson": lesson.Id})
	save("schedule_sessions", map[string]any{"lesson_schedule": sched.Id, "sched_id": "open", "name": "Open"})
	save("schedule_sessions", map[string]any{"lesson_schedule": sched.Id, "sched_id": "past", "name": "Past",
		"enroll_from": "2020-01-01 09:00:00.000Z", "enroll_to": "2020-01-01 10:00:00.000Z"})

	got, err := openCourses(app)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CourseID != course.Id || len(got[0].Sessions) != 1 || got[0].Sessions[0] != "L / Open" {
		t.Errorf("openCourses = %+v, want one course with the open Session", got)
	}
	if roomCodeEnabled(app) {
		t.Error("room codes on with no app_config row")
	}
	save("app_config", map[string]any{"key": "room_code_login", "value": "true"})
	if !roomCodeEnabled(app) {
		t.Error("room codes off with the row set to true")
	}
	if s, err := roomCodeSecret(app); err != nil || s == "" {
		t.Errorf("secret = %q, %v", s, err)
	}
}
