package nabu

import (
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/demo"
)

// TestLibrarySeedsEmbeddedCourse builds an App the way a course binary does —
// content, config and demo roster all embedded, nothing on disk — runs `seed`
// through it, and checks the course, the app name and the enrolled demo user
// landed in the database.
func TestLibrarySeedsEmbeddedCourse(t *testing.T) {
	t.Cleanup(func() { embedfs.OverrideYAML, demo.Override = nil, nil })
	content := fstest.MapFS{
		"lib-course/_index.md":               {Data: []byte("---\ntitle: Library course\nparams:\n  readiness: production\n---\n")},
		"lib-course/lesson-one/_index.md":    {Data: []byte("---\ntitle: Lesson one\nparams:\n  readiness: production\n---\n")},
		"lib-course/lesson-one/010-intro.md": {Data: []byte("---\ntitle: Intro\nparams:\n  kind: intro\n  readiness: production\n---\nHello from an embedded course.\n")},
		"lib-course/lesson-one/020-human.md": {Data: []byte("---\ntitle: Task\nparams:\n  kind: human\n  readiness: production\n---\nDo the task.\n")},
	}
	roster := []byte("users:\n  - uid: lib-alice\n    email: alice@nabu.example\n    name: Alice\n    role: student\n    courses: [lib-course]\n    password: secret1234\n    verified: true\n")

	app := New(Options{
		DataDir:    filepath.Join(t.TempDir(), "pb_data"),
		Content:    content,
		ConfigYAML: []byte("demo_mode: true\npocketbase:\n  app:\n    name: Library Test\n"),
		DemoRoster: roster,
		KeepLogger: true,
	})
	app.RootCmd.SetArgs([]string{"seed"})
	if err := app.Start(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	course, err := app.FindFirstRecordByData("courses", "slug", "lib-course")
	if err != nil {
		t.Fatalf("embedded course not seeded: %v", err)
	}
	lessons, err := app.FindRecordsByFilter("lessons", "course = {:c}", "", 0, 0, map[string]any{"c": course.Id})
	if err != nil || len(lessons) != 1 {
		t.Fatalf("lessons = %d (%v), want 1", len(lessons), err)
	}
	name, err := app.FindFirstRecordByData("app_config", "key", "app_name")
	if err != nil || name.GetString("value") != "Library Test" {
		t.Fatalf("app_name = %v (%v), want Library Test from the baked config", name, err)
	}
	alice, err := app.FindFirstRecordByData("users", "uid", "lib-alice")
	if err != nil {
		t.Fatalf("demo user not seeded from the baked roster: %v", err)
	}
	if _, err := app.FindFirstRecordByFilter("enrolments", "user = {:u} && course = {:c}",
		map[string]any{"u": alice.Id, "c": course.Id}); err != nil {
		t.Fatalf("demo user not enrolled in the embedded course: %v", err)
	}
	// help comes from the binary when nothing else is given
	if _, err := app.FindFirstRecordByData("courses", "slug", "using-nabu"); err != nil {
		t.Fatalf("built-in help not seeded: %v", err)
	}
}
