package content

import (
	"strings"
	"testing"
)

// The help tree has a static, guest-viewable route, so its cross-references
// become real URLs.
func TestXrefHelpRootEmitsURL(t *testing.T) {
	ctx := RenderContext{CourseSlug: "using-nabu", LessonSlug: "authoring-lessons", HelpRoot: true}
	for _, tc := range []struct{ in, want string }{
		// bare lesson -> current course
		{`href="help:reference"`, `href="/nabu/help/using-nabu/reference"`},
		// explicit course/lesson
		{`href="help:other/reference"`, `href="/nabu/help/other/reference"`},
		// fragment preserved
		{`href="help:reference#widgets"`, `href="/nabu/help/using-nabu/reference#widgets"`},
	} {
		if got := rewriteXrefLinks(tc.in, ctx); got != tc.want {
			t.Errorf("rewriteXrefLinks(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// An authored course has no static route — its slides are only on screen inside a
// session — so a same-lesson reference becomes an in-player jump instead.
func TestXrefAuthoredCourseEmitsSlideJump(t *testing.T) {
	ctx := RenderContext{CourseSlug: "BTASB", LessonSlug: "04-observability-security"}

	got := rewriteXrefLinks(`href="help:04-observability-security#a4-2-spectre"`, ctx)
	want := `href="#a4-2-spectre" data-nabu-slide="a4-2-spectre"`
	if got != want {
		t.Errorf("same-lesson xref = %q, want %q", got, want)
	}

	// the explicit course/lesson form resolves to the same jump
	got = rewriteXrefLinks(`href="help:BTASB/04-observability-security#a4-2-spectre"`, ctx)
	if got != want {
		t.Errorf("explicit same-lesson xref = %q, want %q", got, want)
	}
}

// What an authored course CANNOT express is left exactly as authored, so
// validateXrefs can find it and explain — better than emitting a link that 404s.
func TestXrefAuthoredCourseLeavesUnresolvable(t *testing.T) {
	ctx := RenderContext{CourseSlug: "BTASB", LessonSlug: "04-observability-security"}
	for _, in := range []string{
		`href="help:05-playpen#a5-1-starbucks"`, // another lesson
		`href="help:other/lesson#slide"`,        // another course
		`href="help:04-observability-security"`, // no slide fragment: nothing to jump to
	} {
		if got := rewriteXrefLinks(in, ctx); got != in {
			t.Errorf("rewriteXrefLinks(%q) = %q, want it left as authored", in, got)
		}
	}
}

// A slide slug is interpolated into two HTML attributes, so it must be escaped.
func TestXrefSlideJumpEscapes(t *testing.T) {
	ctx := RenderContext{CourseSlug: "c", LessonSlug: "l"}
	got := rewriteXrefLinks(`href="help:l#a&quot;><script>"`, ctx)
	if strings.Contains(got, "<script>") {
		t.Errorf("unescaped slug leaked into markup: %q", got)
	}
}

// The two failure modes an authored course can hit must both surface on the
// slide's content status rather than passing silently.
func TestValidateXrefsAuthoredCourse(t *testing.T) {
	courses := []Course{{Slug: "BTASB", Lessons: []Lesson{
		{Slug: "m4", Slides: []Slide{
			{Slug: "one", status: status{Content: StatusOK}},
			{
				Slug:   "goodjump",
				HTML:   `<a href="#one" data-nabu-slide="one">x</a>`,
				status: status{Content: StatusOK},
			},
			{
				Slug:   "badjump",
				HTML:   `<a href="#nope" data-nabu-slide="nope">x</a>`,
				status: status{Content: StatusOK},
			},
			{
				Slug:   "unresolvable",
				HTML:   `<a href="help:m5#thing">x</a>`,
				status: status{Content: StatusOK},
			},
		}},
	}}}
	validateXrefs(courses)
	s := courses[0].Lessons[0].Slides

	if s[1].Content != StatusOK {
		t.Errorf("jump to an existing slide should stay OK, got %s (%s)", s[1].Content, s[1].Issue)
	}
	if s[2].Content != StatusWarnings {
		t.Errorf("jump to a missing slide should warn, got %s", s[2].Content)
	}
	if s[3].Content != StatusWarnings {
		t.Errorf("unresolvable xref should warn, got %s", s[3].Content)
	}
	// the warning has to say WHY, since the authored text looks correct
	if !strings.Contains(s[3].Issue, "same-lesson") {
		t.Errorf("unresolvable xref issue should explain the limitation, got %q", s[3].Issue)
	}
}
