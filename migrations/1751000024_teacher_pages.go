package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init adds the teacher_pages collection: teacher-only companion pages for a lesson
// (briefing/debrief screens a teacher reads while running the class), authored under a
// lesson's params.teacher_pages map and seeded from the content tree like slides.
//
// It mirrors the slide content fields (body/sim_output HTML, the t_choice/t_agent/
// t_mapper/timers widget configs, notes_label…) so a teacher page renders through the
// same <Slide> component with the same live runtime, plus a `key` (the frontmatter map
// key, e.g. "start"/"end") used in its URL. Unlike slides it is NOT gated by the public
// go_live rule — it is staff-only: only role teacher/admin may list or view it.
func init() {
	m.Register(func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}

		tp := core.NewBaseCollection("teacher_pages")
		tp.Fields.Add(
			&core.RelationField{Name: "lesson", Required: true, CollectionId: lessons.Id, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "key", Required: true},
			&core.SelectField{Name: "kind", MaxSelect: 1,
				Values: []string{"intro", "human", "presentation", "agent", "discussion", "reflection"}},
			&core.TextField{Name: "audience"},
			&core.TextField{Name: "title", Required: true},
			&core.EditorField{Name: "body"},
			&core.TextField{Name: "agent_key"},
			&core.EditorField{Name: "sim_output"},
			&core.JSONField{Name: "t_choice"},
			&core.JSONField{Name: "t_agent"},
			&core.JSONField{Name: "t_mapper"},
			&core.JSONField{Name: "timers"},
			&core.TextField{Name: "notes_label"},
			// publishing status, for admin visibility + CORRUPT detection (not a serve gate).
			&core.TextField{Name: "readiness"},
			&core.SelectField{Name: "content", MaxSelect: 1,
				Values: []string{"OK", "WARNINGS", "ERRORS", "CORRUPT"}},
		)
		tp.AddIndex("idx_teacher_pages_lesson_key", true, "lesson, `key`", "")
		// staff-only: teacher pages are never shown to students.
		staff := types.Pointer(`@request.auth.role = "teacher" || @request.auth.role = "admin"`)
		tp.ListRule = staff
		tp.ViewRule = staff
		if err := app.Save(tp); err != nil {
			return fmt.Errorf("save teacher_pages: %w", err)
		}
		return nil
	}, func(app core.App) error {
		if tp, err := app.FindCollectionByNameOrId("teacher_pages"); err == nil {
			if err := app.Delete(tp); err != nil {
				return fmt.Errorf("delete teacher_pages: %w", err)
			}
		}
		return nil
	})
}
