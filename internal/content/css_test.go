package content

import (
	"os"
	"path/filepath"
	"testing"
)

// cssSink records what was copied into the public tree.
type cssSink struct{ copied map[string]string }

func (s *cssSink) Copy(srcPath, publicRel string) (string, error) {
	if s.copied == nil {
		s.copied = map[string]string{}
	}
	s.copied[publicRel] = srcPath
	return "/" + publicRel, nil
}

// writeTree lays out a small content tree on disk.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The authoring cascade is nabu.css -> _course.css -> _lesson.css. The engine's
// half of that is resolving BOTH stylesheets to served URLs and copying them
// into the public tree; the player links them in that order.
func TestCourseAndLessonCSSResolved(t *testing.T) {
	root := writeTree(t, map[string]string{
		"mycourse/_index.md":            "---\ntitle: My course\nparams:\n  css: _course.css\n---\n",
		"mycourse/_course.css":          ".slide .body { --lime: hsl(80 45% 49%); }\n",
		"mycourse/lesson-a/_index.md":   "---\ntitle: Lesson A\nparams:\n  css: _lesson.css\n---\n",
		"mycourse/lesson-a/_lesson.css": ".slide .body { --lime: red; }\n",
		"mycourse/lesson-a/10-slide.md": "---\ntitle: A slide\n---\nbody\n",
		// a lesson with no stylesheet must simply have none — not inherit a path
		"mycourse/lesson-b/_index.md":   "---\ntitle: Lesson B\n---\n",
		"mycourse/lesson-b/10-slide.md": "---\ntitle: B slide\n---\nbody\n",
	})
	sink := &cssSink{}
	courses, err := Load(root, NewGoldmarkRenderer(), Options{Assets: sink})
	if err != nil {
		t.Fatal(err)
	}
	if len(courses) != 1 {
		t.Fatalf("courses = %d, want 1", len(courses))
	}
	c := courses[0]
	if got, want := c.CSS, "/content/mycourse/_course.css"; got != want {
		t.Errorf("course CSS = %q, want %q", got, want)
	}
	byslug := map[string]Lesson{}
	for _, l := range c.Lessons {
		byslug[l.Slug] = l
	}
	if got, want := byslug["lesson-a"].CSS, "/content/mycourse/lesson-a/_lesson.css"; got != want {
		t.Errorf("lesson CSS = %q, want %q", got, want)
	}
	if got := byslug["lesson-b"].CSS; got != "" {
		t.Errorf("a lesson with no css must have none, got %q", got)
	}
	// both stylesheets must actually be published, or the <link> 404s
	for _, want := range []string{"content/mycourse/_course.css", "content/mycourse/lesson-a/_lesson.css"} {
		if _, ok := sink.copied[want]; !ok {
			t.Errorf("stylesheet not copied to the public tree: %s", want)
		}
	}
}

// A control file is never content. This is the rule that stops _course.css,
// _references.md and a _drafts/ folder from turning into lessons and slides —
// and it had already bitten this repo once, when an _original-art/ folder beside
// the lessons was ingested as a second lesson.
func TestControlFilesAreNotContent(t *testing.T) {
	root := writeTree(t, map[string]string{
		"mycourse/_index.md":               "---\ntitle: My course\n---\n",
		"mycourse/_course.css":             "x",
		"mycourse/_original-art/_index.md": "---\ntitle: Not a lesson\n---\n",
		"mycourse/_original-art/10-art.md": "---\ntitle: Not a slide\n---\n",
		"mycourse/lesson-a/_index.md":      "---\ntitle: Lesson A\n---\n",
		"mycourse/lesson-a/_references.md": "[a]: https://example.com\n",
		"mycourse/lesson-a/_drafts/x.md":   "---\ntitle: Draft\n---\n",
		"mycourse/lesson-a/10-real.md":     "---\ntitle: Real slide\n---\nbody\n",
		"_scratch/_index.md":               "---\ntitle: Not a course\n---\n",
	})
	courses, err := Load(root, NewGoldmarkRenderer(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(courses) != 1 || courses[0].Slug != "mycourse" {
		t.Fatalf("a _-prefixed folder became a course: %+v", courses)
	}
	if n := len(courses[0].Lessons); n != 1 {
		t.Fatalf("lessons = %d, want 1 (a _-prefixed folder became a lesson)", n)
	}
	l := courses[0].Lessons[0]
	if n := len(l.Slides); n != 1 {
		t.Fatalf("slides = %d, want 1: %+v", n, l.Slides)
	}
	if l.Slides[0].Slug != "real" {
		t.Errorf("wrong slide survived: %q", l.Slides[0].Slug)
	}
}
