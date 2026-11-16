package seed

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	_ "github.com/mrmxf/nabu/migrations"
)

// TestApplySignupRule: closing signup removes the users create rule (only
// superusers and server code create accounts); opening it restores the
// student-only rule; repeating either is a no-op.
func TestApplySignupRule(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	rule := func() *string {
		t.Helper()
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			t.Fatal(err)
		}
		return users.CreateRule
	}
	for _, step := range []struct {
		enabled bool
		want    *string
	}{{false, nil}, {false, nil}, {true, ptr(signupCreateRule)}, {true, ptr(signupCreateRule)}} {
		if err := applySignupRule(app, step.enabled); err != nil {
			t.Fatal(err)
		}
		got := rule()
		if (got == nil) != (step.want == nil) || (got != nil && *got != *step.want) {
			t.Errorf("enabled=%v: create rule = %v, want %v", step.enabled, got, step.want)
		}
	}
}

func ptr(s string) *string { return &s }
