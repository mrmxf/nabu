package httpapi

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	// side-effect: registers the Secret Agents migrations so a fresh test app
	// bootstraps the full schema (courses/lessons/sessions/groups/...).
	_ "github.com/mrmxf/nabu/migrations"
)

// newMigratedApp spins up a throwaway, fully-migrated PocketBase app backed by a
// temp data dir — the same Bootstrap + RunAllMigrations the server does — so
// DB-coupled helpers can be exercised against the real schema.
func newMigratedApp(t *testing.T) core.App {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return app
}

// TestResolveGroupLessonScoped: a group's identity includes the lesson, so the
// same group tag in the same Session but two different lessons resolves to two
// independent group rows, while the same (lesson, session, tag) is reused.
func TestResolveGroupLessonScoped(t *testing.T) {
	app := newMigratedApp(t)

	save := func(rec *core.Record) *core.Record {
		t.Helper()
		if err := app.Save(rec); err != nil {
			t.Fatalf("save %s: %v", rec.Collection().Name, err)
		}
		return rec
	}
	newRec := func(collection string) *core.Record {
		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatalf("find %s: %v", collection, err)
		}
		return core.NewRecord(col)
	}

	course := newRec("courses")
	course.Set("title", "C")
	course.Set("slug", "c")
	save(course)

	mkLesson := func(slug string) *core.Record {
		l := newRec("lessons")
		l.Set("course", course.Id)
		l.Set("title", slug)
		l.Set("slug", slug)
		return save(l)
	}
	lessonA := mkLesson("lesson-a")
	lessonB := mkLesson("lesson-b")

	// each lesson gets a schedule + one Session (a shared group needs a Session).
	mkScheduleSession := func(lesson *core.Record) *core.Record {
		sched := newRec("lesson_schedules")
		sched.Set("lesson", lesson.Id)
		save(sched)
		ss := newRec("schedule_sessions")
		ss.Set("lesson_schedule", sched.Id)
		ss.Set("sched_id", "s")
		ss.Set("name", "S")
		return save(ss)
	}
	sessionA := mkScheduleSession(lessonA)
	sessionB := mkScheduleSession(lessonB)

	user := newRec("users")
	user.Set("email", "student@nabu.example")
	user.Set("uid", "u1")
	user.SetPassword("secret1234")
	save(user)

	mkSession := func(lesson, scheduleSession *core.Record) *core.Record {
		s := newRec("sessions")
		s.Set("user", user.Id)
		s.Set("course", course.Id)
		s.Set("lesson", lesson.Id)
		s.Set("schedule_session", scheduleSession.Id)
		s.Set("group_name", "s7g1") // same tag in both lessons
		s.Set("status", "active")
		return save(s)
	}
	sessA := mkSession(lessonA, sessionA)
	sessB := mkSession(lessonB, sessionB)

	groupA, err := resolveGroup(app, sessA)
	if err != nil {
		t.Fatalf("resolveGroup A: %v", err)
	}
	groupB, err := resolveGroup(app, sessB)
	if err != nil {
		t.Fatalf("resolveGroup B: %v", err)
	}

	if groupA.Id == groupB.Id {
		t.Fatalf("same tag in different lessons resolved to one group %q; want two", groupA.Id)
	}
	if got := groupA.GetString("lesson"); got != lessonA.Id {
		t.Errorf("group A lesson = %q, want %q", got, lessonA.Id)
	}
	if got := groupB.GetString("lesson"); got != lessonB.Id {
		t.Errorf("group B lesson = %q, want %q", got, lessonB.Id)
	}

	// same (lesson, session, tag) reuses the existing row, not a duplicate.
	if again, err := resolveGroup(app, sessA); err != nil || again.Id != groupA.Id {
		t.Errorf("same (lesson,session,tag) should reuse group; got %v (err %v)", again, err)
	}

	rows, err := app.FindRecordsByFilter("groups", "tag = 's7g1'", "", 0, 0)
	if err != nil {
		t.Fatalf("count groups: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d s7g1 group rows, want 2 (one per lesson)", len(rows))
	}
}

// TestResolveGroupSessionScoped: a group's identity is (lesson, session, tag), so
// the same tag in two different Sessions of one lesson resolves to two independent
// groups, while an attempt with no Session (unscheduled) is solo (its own group).
func TestResolveGroupSessionScoped(t *testing.T) {
	app := newMigratedApp(t)

	save := func(rec *core.Record) *core.Record {
		t.Helper()
		if err := app.Save(rec); err != nil {
			t.Fatalf("save %s: %v", rec.Collection().Name, err)
		}
		return rec
	}
	newRec := func(collection string) *core.Record {
		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatalf("find %s: %v", collection, err)
		}
		return core.NewRecord(col)
	}

	course := newRec("courses")
	course.Set("title", "C")
	course.Set("slug", "c")
	save(course)

	lesson := newRec("lessons")
	lesson.Set("course", course.Id)
	lesson.Set("title", "L")
	lesson.Set("slug", "l")
	save(lesson)

	schedule := newRec("lesson_schedules")
	schedule.Set("lesson", lesson.Id)
	save(schedule)

	mkSession := func(schedID string) *core.Record {
		s := newRec("schedule_sessions")
		s.Set("lesson_schedule", schedule.Id)
		s.Set("sched_id", schedID)
		s.Set("name", schedID)
		return save(s)
	}
	sessTue := mkSession("tue")
	sessThu := mkSession("thu")

	user := newRec("users")
	user.Set("email", "s@nabu.example")
	user.Set("uid", "u1")
	user.SetPassword("secret1234")
	save(user)

	// an attempt (the `sessions` record) with a given Session + group tag.
	mkAttempt := func(scheduleSession, tag string) *core.Record {
		a := newRec("sessions")
		a.Set("user", user.Id)
		a.Set("course", course.Id)
		a.Set("lesson", lesson.Id)
		a.Set("group_name", tag)
		a.Set("schedule_session", scheduleSession)
		a.Set("status", "active")
		return save(a)
	}

	// same (lesson, Session, tag) → one shared group.
	gTue1, err := resolveGroup(app, mkAttempt(sessTue.Id, "g1"))
	if err != nil {
		t.Fatalf("resolveGroup tue g1: %v", err)
	}
	gTue2, err := resolveGroup(app, mkAttempt(sessTue.Id, "g1"))
	if err != nil {
		t.Fatalf("resolveGroup tue g1 again: %v", err)
	}
	if gTue1.Id != gTue2.Id {
		t.Errorf("same (lesson,session,tag) should reuse the group; got %s vs %s", gTue1.Id, gTue2.Id)
	}

	// same tag, DIFFERENT Session → independent group (the isolation fix).
	gThu, err := resolveGroup(app, mkAttempt(sessThu.Id, "g1"))
	if err != nil {
		t.Fatalf("resolveGroup thu g1: %v", err)
	}
	if gThu.Id == gTue1.Id {
		t.Errorf("tag g1 in two Sessions must be two groups; got one %s", gThu.Id)
	}
	if gThu.GetString("session") != sessThu.Id {
		t.Errorf("group session = %q, want %q", gThu.GetString("session"), sessThu.Id)
	}

	// no Session (unscheduled) → solo, unique per attempt, session empty.
	a1 := mkAttempt("", "g1")
	a2 := mkAttempt("", "g1")
	solo1, err := resolveGroup(app, a1)
	if err != nil {
		t.Fatalf("resolveGroup solo a1: %v", err)
	}
	solo2, err := resolveGroup(app, a2)
	if err != nil {
		t.Fatalf("resolveGroup solo a2: %v", err)
	}
	if solo1.Id == solo2.Id {
		t.Errorf("unscheduled attempts must be solo (distinct groups); got one %s", solo1.Id)
	}
	if solo1.GetString("tag") != "solo-"+a1.Id || solo1.GetString("session") != "" {
		t.Errorf("solo group = {tag:%q session:%q}, want {tag:solo-%s session:''}",
			solo1.GetString("tag"), solo1.GetString("session"), a1.Id)
	}
}

// TestResolveTTL: authored timeouts parse and clamp to [minTTL, maxTTL] (max=120
// with AGENT_MAX_TTL unset).
func TestResolveTTL(t *testing.T) {
	cases := map[string]int{
		"":        defaultTTLSeconds, // unset -> default
		"30s":     30,
		"1m":      60,
		"90":      90,  // bare seconds
		"2s":      5,   // below min -> clamped up
		"9999s":   120, // above max -> clamped down
		"garbage": defaultTTLSeconds,
	}
	for in, want := range cases {
		if got := resolveTTL(in); got != want {
			t.Errorf("resolveTTL(%q) = %d, want %d", in, got, want)
		}
	}
}
