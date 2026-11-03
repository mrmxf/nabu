package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds a `t_quiz` JSON field to slides and teacher_pages: a list of named
// self-check question groups, each with its own `id`, placed in the body by an
// {{< LMS t="quiz" id="…" >}} widget. Mirrors `t_choice` — bulky config in
// frontmatter, the shortcode carrying only placement.
//
//	t_quiz: [{"id":"m1-basics","scope":"student","questions":[…]}]
//
// This stores the quiz DEFINITION only. Student answers are not persisted yet;
// when they are, they get their own collection keyed
// (session, user|group, quiz id, question id) — a tuple the authored ids here
// already determine, so no content has to change. See docs/quiz-persistence.md.
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"slides", "teacher_pages"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s collection: %w", name, err)
			}
			col.Fields.Add(&core.JSONField{Name: "t_quiz"})
			if err := app.Save(col); err != nil {
				return fmt.Errorf("update %s: %w", name, err)
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"slides", "teacher_pages"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return fmt.Errorf("find %s collection: %w", name, err)
			}
			col.Fields.RemoveByName("t_quiz")
			if err := app.Save(col); err != nil {
				return fmt.Errorf("revert %s: %w", name, err)
			}
		}
		return nil
	})
}
