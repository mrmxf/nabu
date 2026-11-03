package migrations

import (
	"fmt"
	"regexp"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init renames the content container "class" to "course" throughout the schema:
//
//   - the `classes` collection becomes `courses`
//   - every `class` relation field becomes `course`, and `role_in_class` becomes
//     `role_in_course` (enrolments)
//   - every API rule and index that names them is rewritten to match, in the
//     same save as the field rename, so PocketBase never sees an index or rule
//     pointing at a column that no longer exists
//   - the app_config keys `class_logo` and `class_numbering` become
//     `course_logo` and `course_numbering`
//
// It introspects every collection rather than listing them, so a field added by
// any earlier migration is covered. Only whole words are rewritten, with `_` and
// punctuation as separators: `idx_lessons_class_slug` and `@collection.classes`
// change, `classroom_fast_login` does not. The down migration is the exact
// inverse; no `course` schema names existed before this migration.
func init() {
	m.Register(func(app core.App) error {
		return renameClassCourse(app, map[string]string{"class": "course", "classes": "courses"},
			"classes", "courses", map[string]string{"class_logo": "course_logo", "class_numbering": "course_numbering"})
	}, func(app core.App) error {
		return renameClassCourse(app, map[string]string{"course": "class", "courses": "classes"},
			"courses", "classes", map[string]string{"course_logo": "class_logo", "course_numbering": "class_numbering"})
	})
}

// schemaWordRe matches one alphanumeric word; `_` and punctuation separate words.
var schemaWordRe = regexp.MustCompile(`[A-Za-z0-9]+`)

// renameWords rewrites each whole word of s found in words.
func renameWords(s string, words map[string]string) string {
	return schemaWordRe.ReplaceAllStringFunc(s, func(w string) string {
		if to, ok := words[w]; ok {
			return to
		}
		return w
	})
}

// renameClassCourse applies one direction of the rename: the container
// collection fromCol -> toCol, every field/rule/index word in words, and the
// app_config keys in settings.
func renameClassCourse(app core.App, words map[string]string, fromCol, toCol string, settings map[string]string) error {
	container, err := app.FindCollectionByNameOrId(fromCol)
	if err != nil {
		return fmt.Errorf("find %s collection: %w", fromCol, err)
	}
	container.Name = toCol
	renameCollectionSchema(container, words)
	if err := app.Save(container); err != nil {
		return fmt.Errorf("rename collection %s -> %s: %w", fromCol, toCol, err)
	}

	cols, err := app.FindAllCollections()
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	for _, col := range cols {
		if col.Id == container.Id || col.System {
			continue
		}
		if !renameCollectionSchema(col, words) {
			continue
		}
		if err := app.Save(col); err != nil {
			return fmt.Errorf("rename fields in %s: %w", col.Name, err)
		}
	}

	for from, to := range settings {
		if err := renameSetting(app, from, to); err != nil {
			return err
		}
	}
	return nil
}

// renameCollectionSchema rewrites col's field names, rules and indexes in place
// and reports whether anything changed.
func renameCollectionSchema(col *core.Collection, words map[string]string) bool {
	changed := false
	for _, f := range col.Fields {
		if to := renameWords(f.GetName(), words); to != f.GetName() {
			f.SetName(to)
			changed = true
		}
	}
	rules := []**string{&col.ListRule, &col.ViewRule, &col.CreateRule, &col.UpdateRule, &col.DeleteRule}
	if col.IsAuth() {
		rules = append(rules, &col.AuthRule, &col.ManageRule)
	}
	for _, r := range rules {
		if *r == nil {
			continue
		}
		if to := renameWords(**r, words); to != **r {
			*r = &to
			changed = true
		}
	}
	for i, idx := range col.Indexes {
		if to := renameWords(idx, words); to != idx {
			col.Indexes[i] = to
			changed = true
		}
	}
	return changed
}

// renameSetting moves the app_config row keyed from to the key to. A missing row
// is fine (the setting was never seeded). If both exist, the old row is dropped:
// the new key was written by a newer seed and wins.
func renameSetting(app core.App, from, to string) error {
	old, err := app.FindFirstRecordByData("app_config", "key", from)
	if err != nil || old == nil {
		return nil
	}
	if existing, _ := app.FindFirstRecordByData("app_config", "key", to); existing != nil {
		if err := app.Delete(old); err != nil {
			return fmt.Errorf("drop superseded app_config %q: %w", from, err)
		}
		return nil
	}
	old.Set("key", to)
	if err := app.Save(old); err != nil {
		return fmt.Errorf("rename app_config %q -> %q: %w", from, to, err)
	}
	return nil
}
