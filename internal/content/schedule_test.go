package content

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadSchedules parses the corrected YAML shape (sessions as a sibling of
// lesson/allow-unscheduled) and checks id defaulting and field mapping.
func TestLoadSchedules(t *testing.T) {
	yml := `
lesson-schedules:
  - lesson: secret-agents
    allow-unscheduled: false
    sessions:
      - id: rise2026-testing
        name: RISE 2026 (testing)
        enroll:
          from: 2026-07-20 00:00:00
          to: 2026-07-27 23:59:59
        start:
          from: 2026-07-20 00:00:00
          to: 2026-07-27 23:59:59
      - name: Autumn Cohort
        enroll:
          from: 2026-09-01 00:00:00
          to: 2026-09-30 23:59:59
        start:
          from: 2026-09-01 00:00:00
          to: 2026-09-30 23:59:59
  - lesson: another-lesson
    allow-unscheduled: true
    sessions: []
`
	dir := t.TempDir()
	path := filepath.Join(dir, "lesson-schedules.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}

	schedules, err := LoadSchedules(path)
	if err != nil {
		t.Fatalf("LoadSchedules: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("got %d schedules, want 2", len(schedules))
	}

	sa := schedules[0]
	if sa.Lesson != "secret-agents" || sa.AllowUnscheduled {
		t.Errorf("first schedule = %+v, want lesson=secret-agents allow-unscheduled=false", sa)
	}
	if len(sa.Sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sa.Sessions))
	}
	if sa.Sessions[0].ID != "rise2026-testing" {
		t.Errorf("session[0].ID = %q, want rise2026-testing", sa.Sessions[0].ID)
	}
	if sa.Sessions[0].Enroll.From != "2026-07-20 00:00:00" {
		t.Errorf("session[0].Enroll.From = %q", sa.Sessions[0].Enroll.From)
	}
	// id omitted => defaults to slug(name)
	if got := sa.Sessions[1].ID; got != "autumn-cohort" {
		t.Errorf("session[1].ID = %q, want autumn-cohort (slug of name)", got)
	}
	if !schedules[1].AllowUnscheduled {
		t.Errorf("second schedule allow-unscheduled = false, want true")
	}
}

// TestLoadSchedulesMissingFile confirms an absent file is not an error.
func TestLoadSchedulesMissingFile(t *testing.T) {
	schedules, err := LoadSchedules(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if schedules != nil {
		t.Errorf("got %v, want nil for missing file", schedules)
	}
}
