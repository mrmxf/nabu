package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init registers the initial Secret Agents schema:
//
//	content:    classes -> lessons -> slides   (authored, seeded, public-read)
//	execution:  sessions                        (a student's run of a lesson, owner-scoped)
//	audit:      activity                        (login/session events, superuser-only)
//
// A slide is the atomic unit of display — one screen a student sees in a lesson.
// The built-in `users` auth collection provides student and admin identity.
//
// Content collections carry two orthogonal publishing axes (see internal/content):
//   - readiness (authored): DRAFT | production | deprecated | a go-live timestamp
//   - content (ingester-set): OK | WARNINGS | ERRORS | CORRUPT
//   - go_live (ingester-derived): the publish gate used by the list/view rules
//
// so students only ever see live, healthy content while superusers (the /_/
// dashboard) see everything.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users collection: %w", err)
		}

		// liveRule gates public content to items that are published now and not
		// broken. Superusers bypass all rules, so the dashboard still sees drafts.
		liveRule := types.Pointer(`go_live != "" && go_live <= @now && content != "ERRORS" && content != "CORRUPT"`)

		// addStatusFields adds the shared publishing axes to a content collection.
		addStatusFields := func(c *core.Collection) {
			c.Fields.Add(
				&core.TextField{Name: "readiness"},
				&core.SelectField{Name: "content", MaxSelect: 1,
					Values: []string{"OK", "WARNINGS", "ERRORS", "CORRUPT"}},
				&core.DateField{Name: "go_live"},
			)
		}

		// ---- classes -------------------------------------------------------
		classes := core.NewBaseCollection("classes")
		classes.Fields.Add(
			&core.TextField{Name: "title", Required: true},
			&core.TextField{Name: "slug", Required: true},
			&core.NumberField{Name: "order", OnlyInt: true},
			&core.EditorField{Name: "summary"},
			&core.TextField{Name: "number"},
		)
		addStatusFields(classes)
		classes.AddIndex("idx_classes_slug", true, "slug", "")
		classes.ListRule = liveRule
		classes.ViewRule = liveRule
		if err := app.Save(classes); err != nil {
			return fmt.Errorf("save classes: %w", err)
		}

		// ---- lessons -------------------------------------------------------
		lessons := core.NewBaseCollection("lessons")
		lessons.Fields.Add(
			&core.RelationField{Name: "class", Required: true, CollectionId: classes.Id, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "title", Required: true},
			&core.TextField{Name: "slug", Required: true},
			&core.NumberField{Name: "order", OnlyInt: true},
			&core.EditorField{Name: "summary"},
			&core.TextField{Name: "number"},
		)
		addStatusFields(lessons)
		lessons.AddIndex("idx_lessons_class_slug", true, "class, slug", "")
		lessons.ListRule = liveRule
		lessons.ViewRule = liveRule
		if err := app.Save(lessons); err != nil {
			return fmt.Errorf("save lessons: %w", err)
		}

		// ---- slides --------------------------------------------------------
		slides := core.NewBaseCollection("slides")
		slides.Fields.Add(
			&core.RelationField{Name: "lesson", Required: true, CollectionId: lessons.Id, MaxSelect: 1, CascadeDelete: true},
			&core.NumberField{Name: "order", OnlyInt: true},
			&core.TextField{Name: "slug"},
			&core.TextField{Name: "number"},
			// kind is not Required: a CORRUPT slide (unparseable frontmatter) still
			// needs to be recorded so admins can see it — it is never served to
			// students because its go_live gate is empty.
			&core.SelectField{Name: "kind", MaxSelect: 1,
				Values: []string{"intro", "human", "presentation", "agent", "discussion", "reflection"}},
			&core.TextField{Name: "title", Required: true},
			// body holds rendered HTML (goldmark/hugo), not Markdown.
			&core.EditorField{Name: "body"},
			// agent_key names the simulated agent for kind=="agent" (hunter|hattie|pixie).
			&core.TextField{Name: "agent_key"},
			// sim_output is the rendered HTML canned agent result on agent screens.
			&core.EditorField{Name: "sim_output"},
		)
		addStatusFields(slides)
		slides.AddIndex("idx_slides_lesson_order", false, "lesson, `order`", "")
		slides.ListRule = liveRule
		slides.ViewRule = liveRule
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("save slides: %w", err)
		}

		// ---- sessions ------------------------------------------------------
		// A student's execution instance of a lesson. Owner-scoped: a student
		// only ever sees and edits their own sessions.
		owner := types.Pointer("@request.auth.id != \"\" && user = @request.auth.id")
		sessions := core.NewBaseCollection("sessions")
		sessions.Fields.Add(
			&core.RelationField{Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "class", Required: true, CollectionId: classes.Id, MaxSelect: 1},
			&core.RelationField{Name: "lesson", Required: true, CollectionId: lessons.Id, MaxSelect: 1},
			// current_slide indexes the live, ordered slide set for this lesson.
			&core.NumberField{Name: "current_slide", OnlyInt: true},
			// optional reporting metadata captured on the dashboard.
			&core.TextField{Name: "session_name"},
			&core.TextField{Name: "group_name"},
			// arbitrary per-lesson working state so a student can resume mid-task.
			&core.JSONField{Name: "work_state"},
			&core.SelectField{Name: "status", MaxSelect: 1, Values: []string{"active", "complete"}},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		sessions.AddIndex("idx_sessions_user", false, "user", "")
		sessions.ListRule = owner
		sessions.ViewRule = owner
		sessions.CreateRule = types.Pointer("@request.auth.id != \"\"")
		sessions.UpdateRule = owner
		sessions.DeleteRule = owner
		if err := app.Save(sessions); err != nil {
			return fmt.Errorf("save sessions: %w", err)
		}

		// ---- activity ------------------------------------------------------
		// Login/session audit trail, written by the Go auth hook (superuser
		// context). Superuser-only rules (nil) keep it out of the student API;
		// admins query it via the dashboard for "who was active" reporting.
		activity := core.NewBaseCollection("activity")
		activity.Fields.Add(
			&core.RelationField{Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1, CascadeDelete: true},
			&core.SelectField{Name: "event", Required: true, MaxSelect: 1,
				Values: []string{"login", "logout", "session_start", "session_resume"}},
			&core.RelationField{Name: "session", CollectionId: sessions.Id, MaxSelect: 1},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		activity.AddIndex("idx_activity_created", false, "created", "")
		if err := app.Save(activity); err != nil {
			return fmt.Errorf("save activity: %w", err)
		}

		return nil
	}, func(app core.App) error {
		// down: delete in reverse dependency order.
		for _, name := range []string{"activity", "sessions", "slides", "lessons", "classes"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue // already gone
			}
			if err := app.Delete(c); err != nil {
				return fmt.Errorf("delete %s: %w", name, err)
			}
		}
		return nil
	})
}
