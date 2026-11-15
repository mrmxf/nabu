package httpapi

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// TestSetClassroomFastLogin: the toggle returns the previous value, so
// `nabu classroom` can restore it on exit.
func TestSetClassroomFastLogin(t *testing.T) {
	app := newMigratedApp(t)
	if _, err := SetClassroomFastLogin(app, true); err == nil {
		t.Error("want an error when the app_config row is missing (unseeded database)")
	}
	col, err := app.FindCollectionByNameOrId("app_config")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("key", "classroom_fast_login")
	rec.Set("value", "false")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	prev, err := SetClassroomFastLogin(app, true)
	if err != nil || prev {
		t.Fatalf("switch on: prev=%v err=%v, want false, nil", prev, err)
	}
	if !classroomEnabled(app) {
		t.Fatal("fast login not on")
	}
	if prev, err = SetClassroomFastLogin(app, prev); err != nil || !prev {
		t.Fatalf("restore: prev=%v err=%v, want true, nil", prev, err)
	}
	if classroomEnabled(app) {
		t.Error("fast login not restored to off")
	}
	open, err := OpenSessions(app)
	if err != nil || len(open) != 0 {
		t.Errorf("OpenSessions on an empty schedule = %v, %v", open, err)
	}
}
