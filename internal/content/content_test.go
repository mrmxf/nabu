package content

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mrmxf/nabu/internal/source"
)

// demoTree is a small, stable fixture course (testdata/course) shaped like a real
// lesson: intro, a human slide with a timer and choice, notes, and an agent slide.
const demoTree = "testdata/course"

func loadDemo(t *testing.T) []Course {
	t.Helper()
	courses, err := Load(source.Disk(demoTree), NewGoldmarkRenderer(), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return courses
}

func TestLoadDemoStructure(t *testing.T) {
	courses := loadDemo(t)
	if len(courses) != 1 {
		t.Fatalf("courses = %d, want 1", len(courses))
	}
	c := courses[0]
	if c.Slug != "fixture-course" {
		t.Errorf("course slug = %q, want fixture-course", c.Slug)
	}
	if len(c.Lessons) != 1 {
		t.Fatalf("lessons = %d, want 1", len(c.Lessons))
	}
	l := c.Lessons[0]
	if l.Slug != "fixture-lesson" {
		t.Errorf("lesson slug = %q, want fixture-lesson", l.Slug)
	}
	// Lower bound (not exact) so authoring new slides doesn't break the suite; the
	// specific slug/ordering assertions below are the real structural checks.
	if len(l.Slides) < 4 {
		t.Fatalf("slides = %d, want >= 4", len(l.Slides))
	}
	// slides must be ordered by their NN- prefix.
	if !sort.SliceIsSorted(l.Slides, func(i, j int) bool { return l.Slides[i].Order < l.Slides[j].Order }) {
		t.Error("slides not sorted by order")
	}
	// filename -> slug derivation, incl. two distinct human slides.
	if got := l.Slides[1].Slug; got != "human-task" {
		t.Errorf("slide[1].Slug = %q, want human-task", got)
	}
	if got := l.Slides[2].Slug; got != "human-task-notes" {
		t.Errorf("slide[2].Slug = %q, want human-task-notes", got)
	}
}

func TestWidgetFrontmatterMapping(t *testing.T) {
	l := loadDemo(t)[0].Lessons[0]
	s := l.Slides[1] // 020-human-task
	if len(s.Timers) != 1 || s.Timers[0].Seconds != 900 || s.Timers[0].Label != "Task" {
		t.Errorf("timers = %+v, want one Task/900", s.Timers)
	}
	if len(s.ChoiceGroups) != 1 || s.ChoiceGroups[0].ID != "human-topic" {
		t.Fatalf("choice groups = %+v, want one id=human-topic", s.ChoiceGroups)
	}
	if opts := s.ChoiceGroups[0].Options; len(opts) != 4 || opts[0].Key != "A" {
		t.Errorf("choice options = %+v, want 4 starting A", opts)
	}
	// an unspecified style defaults to "radio".
	if got := s.ChoiceGroups[0].Style; got != "radio" {
		t.Errorf("choice style = %q, want radio (default)", got)
	}
	if s.Kind != "human" {
		t.Errorf("kind = %q, want human", s.Kind)
	}
}

// TestChoiceStyleDropdown verifies a t_choice group with an explicit
// style: dropdown parses through as a dropdown (e.g. a later slide re-showing an
// earlier pick as a compact selector). Driven by a fixture rather than a demo
// slide so it stays meaningful regardless of how the showcase lesson is authored;
// normalization of empty/bad styles is covered by TestNormalizeChoiceStyles.
func TestChoiceStyleDropdown(t *testing.T) {
	raw := []byte(`---
title: Re-show the pick
params:
  kind: presentation
  t_choice:
    - id: human-topic
      style: dropdown
      scope: group
      default_value_key: slug
      options:
        - { key: A, label: "Dinosaurs", slug: "dinosaurs" }
        - { key: B, label: "Empires",   slug: "empires" }
---
Your topic: {{< LMS t="choice" id="human-topic" >}}
`)
	fm, _, err := parseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	gs := fm.Params.ChoiceGroups
	if len(gs) != 1 || gs[0].ID != "human-topic" {
		t.Fatalf("choice groups = %+v, want one id=human-topic", gs)
	}
	if got := gs[0].Style; got != "dropdown" {
		t.Errorf("choice style = %q, want dropdown", got)
	}
}

func TestNormalizeChoiceStyles(t *testing.T) {
	gs := []ChoiceGroup{{Style: ""}, {Style: "dropdown"}, {Style: "bogus"}}
	got := normalizeChoiceStyles(gs)
	if got != StatusWarnings {
		t.Errorf("status = %q, want WARNINGS for a bad style", got)
	}
	want := []string{"radio", "dropdown", "radio"}
	for i, w := range want {
		if gs[i].Style != w {
			t.Errorf("group %d style = %q, want %q", i, gs[i].Style, w)
		}
	}
	// a clean set stays OK.
	if s := normalizeChoiceStyles([]ChoiceGroup{{Style: "radio"}}); s != StatusOK {
		t.Errorf("status = %q, want OK for a clean set", s)
	}
}

// TestScopedTimerFrontmatter checks a teacher-scoped timer parses with its id and
// scope carried through (so the client can pick SyncedTimer over CountdownTimer),
// and that validateTimers accepts it while rejecting a bad scope, a missing id,
// and a non-positive duration.
func TestScopedTimerFrontmatter(t *testing.T) {
	raw := []byte(`---
title: Timed task
params:
  kind: presentation
  timers:
    - id: agent-task-timer
      scope: teacher
      label: Configure & run agents
      seconds: 2400
---
Do the task.
`)
	fm, _, err := parseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	ts := fm.Params.Timers
	if len(ts) != 1 || ts[0].ID != "agent-task-timer" || ts[0].Scope != "teacher" || ts[0].Seconds != 2400 {
		t.Fatalf("timers = %+v, want one teacher-scoped agent-task-timer/2400", ts)
	}
	if st, _ := validateTimers(ts); st != StatusOK {
		t.Errorf("validateTimers(good) = %q, want OK", st)
	}

	// A legacy local segment (no id/scope) stays valid.
	if st, _ := validateTimers([]Timer{{Label: "Task", Seconds: 900}}); st != StatusOK {
		t.Errorf("validateTimers(local segment) = %q, want OK", st)
	}
	// Bad or incomplete scoped timers each error.
	bad := []struct {
		name string
		in   []Timer
	}{
		{"unknown scope", []Timer{{ID: "x", Scope: "group", Seconds: 60}}},
		{"missing id", []Timer{{Scope: "teacher", Seconds: 60}}},
		{"non-positive duration", []Timer{{ID: "x", Scope: "teacher", Seconds: 0}}},
	}
	for _, c := range bad {
		if st, _ := validateTimers(c.in); st != StatusErrors {
			t.Errorf("validateTimers(%s) = %q, want ERRORS", c.name, st)
		}
	}
}

func TestNumberingResolution(t *testing.T) {
	l := loadDemo(t)[0].Lessons[0]
	// slide[2] is the 3rd slide: CC.L.S -> fixture-course.1.3
	if got := l.Slides[2].Number; got != "fixture-course.1.3" {
		t.Errorf("slide[2].Number = %q, want fixture-course.1.3", got)
	}

	// token length x1..x4 => number/slug/title/number+title
	ctx := numberingCtx{Slide: levelInfo{Number: 3, Slug: "hunter", Title: "Hunter Agent"}}
	cases := map[string]string{
		"S":    "3",
		"SS":   "hunter",
		"SSS":  "Hunter Agent",
		"SSSS": "3 Hunter Agent",
	}
	for pat, want := range cases {
		if got := resolveNumbering(pat, ctx); got != want {
			t.Errorf("resolveNumbering(%q) = %q, want %q", pat, got, want)
		}
	}
}

func TestReadinessGate(t *testing.T) {
	if _, gl := resolveReadiness("production"); gl == nil {
		t.Error("production should be live (non-nil go_live)")
	}
	if _, gl := resolveReadiness("DRAFT"); gl != nil {
		t.Error("DRAFT should not be live")
	}
	if _, gl := resolveReadiness("deprecated"); gl != nil {
		t.Error("deprecated should not be live")
	}
	if rd, gl := resolveReadiness("2026-09-01@09:00:00T+0100"); gl == nil || rd == "" {
		t.Errorf("scheduled timestamp should parse to a go_live, got rd=%q gl=%v", rd, gl)
	}
	if rd, gl := resolveReadiness(""); rd != "DRAFT" || gl != nil {
		t.Errorf("empty readiness should default to DRAFT/not-live, got rd=%q gl=%v", rd, gl)
	}
}

func TestGFMTableRenders(t *testing.T) {
	l := loadDemo(t)[0].Lessons[0]
	var agent *Slide
	for i := range l.Slides {
		// slug derives from the filename (040-agent-proto.md -> agent-proto).
		if l.Slides[i].Slug == "agent-proto" {
			agent = &l.Slides[i]
		}
	}
	if agent == nil {
		t.Fatal("agent-proto slide not found")
	}
	if !strings.Contains(agent.SimHTML, "<table") {
		t.Errorf("agent sim_output did not render a GFM table:\n%s", agent.SimHTML)
	}
}

func TestLMSShortcode(t *testing.T) {
	out, issues, err := preprocessLMS(`{{< LMS t="timer" duration="15:00" start="onshow" >}}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("valid timer produced issues: %+v", issues)
	}
	if !strings.Contains(out, `<lms param='`) || !strings.Contains(out, `</lms>`) {
		t.Errorf("output missing <lms> element: %s", out)
	}
	if !strings.Contains(out, `&#34;t&#34;:&#34;timer&#34;`) {
		t.Errorf("JSON not HTML-escaped in attribute: %s", out)
	}

	// unknown widget => WARNINGS; missing required arg => ERRORS
	if _, iss, _ := preprocessLMS(`{{< LMS t="nope" >}}`, nil, nil); worstOf(iss) != StatusWarnings {
		t.Errorf("unknown widget should warn, got %v", worstOf(iss))
	}
	if _, iss, _ := preprocessLMS(`{{< LMS t="timer" >}}`, nil, nil); worstOf(iss) != StatusErrors {
		t.Errorf("missing required arg should error, got %v", worstOf(iss))
	}
}

// TestLMSInlineShortcode covers shortcodes embedded in a line of text (an inline
// `print` in a sentence/heading, with spaces around `=`), not just ones that
// occupy a whole line. These previously passed through raw.
func TestLMSInlineShortcode(t *testing.T) {
	// inline within bold text, with spaces around '='.
	in := `You group's topic is **{{< LMS t = "print" from = "human-topic" >}}**`
	out, issues, err := preprocessLMS(in, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("valid inline print produced issues: %+v", issues)
	}
	if strings.Contains(out, "{{<") {
		t.Errorf("inline shortcode left unconverted: %s", out)
	}
	if !strings.Contains(out, `<lms param='`) || !strings.Contains(out, `</lms>`) {
		t.Errorf("output missing <lms> element: %s", out)
	}
	if !strings.Contains(out, "You group's topic is **") || !strings.Contains(out, "**") {
		t.Errorf("surrounding text not preserved: %s", out)
	}

	// two inline shortcodes on one line are both converted.
	two := `a {{<LMS t="print" from="group">}} b {{<LMS t="print" from="session">}} c`
	out2, _, _ := preprocessLMS(two, nil, nil)
	if n := strings.Count(out2, "<lms param='"); n != 2 {
		t.Errorf("expected 2 <lms> elements, got %d: %s", n, out2)
	}
	if strings.Contains(out2, "{{<") {
		t.Errorf("an inline shortcode was left unconverted: %s", out2)
	}
}

// TestLMSCodeExamplesLeftLiteral covers shortcodes written as DOCUMENTATION inside
// code fences or inline code spans (as the help lessons do): they must be left
// verbatim and must NOT be validated (so an intentionally-incomplete example like
// `{{< LMS t="agent" >}}` doesn't stamp the slide with errors).
func TestLMSCodeExamplesLeftLiteral(t *testing.T) {
	// inline code span with a required-arg-missing example.
	inline := "Place an `{{< LMS t=\"agent\" >}}` widget in the body."
	out, issues, err := preprocessLMS(inline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("code-span example should not validate; got issues %+v", issues)
	}
	if !strings.Contains(out, "`{{< LMS t=\"agent\" >}}`") {
		t.Errorf("inline code example was not left literal: %s", out)
	}
	if strings.Contains(out, "<lms") {
		t.Errorf("inline code example was converted: %s", out)
	}

	// fenced code block with own-line examples (incl. trailing comments).
	fenced := "```\n{{< LMS t=\"agent\" id=\"hunter\" >}}   # dispatch\n{{< LMS t=\"bogus\" >}}\n```\n"
	out2, issues2, _ := preprocessLMS(fenced, nil, nil)
	if len(issues2) != 0 {
		t.Errorf("fenced examples should not validate; got issues %+v", issues2)
	}
	if strings.Contains(out2, "<lms") {
		t.Errorf("fenced example was converted: %s", out2)
	}

	// a real inline widget OUTSIDE code still converts even next to a code span.
	mixed := "Topic `code` is **{{< LMS t=\"print\" from=\"human-topic\" >}}**"
	out3, issues3, _ := preprocessLMS(mixed, nil, nil)
	if len(issues3) != 0 {
		t.Errorf("valid mixed print produced issues: %+v", issues3)
	}
	if !strings.Contains(out3, "<lms param='") || !strings.Contains(out3, "`code`") {
		t.Errorf("expected converted widget and preserved code span: %s", out3)
	}
}

func TestLMSChoiceRef(t *testing.T) {
	// a choice referencing a declared t_choice id is clean.
	if _, iss, _ := preprocessLMS(`{{< LMS t="choice" id="topic" >}}`, []string{"topic"}, nil); len(iss) != 0 {
		t.Errorf("declared choice id should be clean, got %+v", iss)
	}
	// referencing an id with no matching group warns (renders nothing).
	if _, iss, _ := preprocessLMS(`{{< LMS t="choice" id="ghost" >}}`, []string{"topic"}, nil); worstOf(iss) != StatusWarnings {
		t.Errorf("undeclared choice id should warn, got %v", worstOf(iss))
	}
	// a choice with no id at all is a schema error (id is required).
	if _, iss, _ := preprocessLMS(`{{< LMS t="choice" >}}`, nil, nil); worstOf(iss) != StatusErrors {
		t.Errorf("choice missing id should error, got %v", worstOf(iss))
	}
}

func TestLMSAttributeEscapesAdversarialValues(t *testing.T) {
	out := renderLMS(map[string]string{"t": "note", "text": `it's <b>"bad" & wrong</b>`})
	for _, raw := range []string{`'`, `"`, `<`, `>`} {
		// the attribute is single-quoted; none of these may appear raw inside it.
		if strings.Contains(strings.TrimPrefix(strings.TrimSuffix(out, `'></lms>`), `<lms param='`), raw) {
			t.Errorf("raw %q leaked into attribute: %s", raw, out)
		}
	}
}

func TestWidgetCatalogueTypes(t *testing.T) {
	types, err := widgetTypes()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, x := range types {
		got[x] = true
	}
	for _, want := range []string{"timer", "figure", "note", "choice", "browser"} {
		if !got[want] {
			t.Errorf("catalogue missing widget %q", want)
		}
	}
}

// TestHelpAudiencesResolved loads the real help tree and asserts inheritance filled
// every lesson/slide with a valid audience, and nothing is broken.
func TestHelpAudiencesResolved(t *testing.T) {
	courses, err := Load(source.Disk("../../help"), NewGoldmarkRenderer(), Options{})
	if err != nil {
		t.Fatalf("Load help: %v", err)
	}
	for _, c := range courses {
		for _, l := range c.Lessons {
			if !validAudiences[l.Audience] {
				t.Errorf("lesson %s/%s audience %q not resolved to a valid persona", c.Slug, l.Slug, l.Audience)
			}
			for _, s := range l.Slides {
				if !validAudiences[s.Audience] {
					t.Errorf("slide %s/%s/%s audience %q not resolved", c.Slug, l.Slug, s.Slug, s.Audience)
				}
				if s.Content == StatusErrors || s.Content == StatusCorrupt {
					t.Errorf("slide %s/%s/%s content=%s", c.Slug, l.Slug, s.Slug, s.Content)
				}
			}
		}
	}
}

// TestProblemsReportsCorruptWithReason: a lesson with invalid frontmatter (a
// duplicate key — valid to lenient parsers, rejected by Go's yaml.v3) is surfaced
// as a CORRUPT Problem carrying a human-readable reason, so seed/CI can bark and
// fail instead of silently hiding it.
func TestProblemsReportsCorruptWithReason(t *testing.T) {
	tmp := t.TempDir()
	mk := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(tmp, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("tc/_index.md", "---\ntitle: TC\nparams:\n  readiness: production\n---\n")
	mk("tc/bad/_index.md", "---\ntitle: Bad\nparams:\n  summary: one\n  summary: two\n---\n")

	courses, err := Load(source.Disk(tmp), NewGoldmarkRenderer(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var bad *Problem
	for i := range Problems(courses) {
		if p := Problems(courses)[i]; strings.Contains(p.Path, "bad") {
			bad = &p
		}
	}
	if bad == nil {
		t.Fatal("corrupt lesson not reported by Problems()")
	}
	if bad.Status != StatusCorrupt {
		t.Errorf("status = %s, want CORRUPT", bad.Status)
	}
	if !strings.Contains(bad.Issue, "frontmatter") {
		t.Errorf("issue missing a reason: %q", bad.Issue)
	}
	t.Logf("reported: path=%s status=%s issue=%q", bad.Path, bad.Status, bad.Issue)
}

// TestValidateXrefs checks that a `help:` cross-reference to a missing lesson/slide
// is flagged, while a valid one is left clean.
func TestValidateXrefs(t *testing.T) {
	courses := []Course{{Slug: "help", Lessons: []Lesson{
		{Slug: "a", Slides: []Slide{{Slug: "one"}, {Slug: "two"}}},
		{Slug: "b", Slides: []Slide{
			{Slug: "good", HTML: `<a href="/nabu/help/help/a#one">x</a>`, status: status{Content: StatusOK}},
			{Slug: "badlesson", HTML: `<a href="/nabu/help/help/zzz">x</a>`, status: status{Content: StatusOK}},
			{Slug: "badslide", HTML: `<a href="/nabu/help/help/a#nope">x</a>`, status: status{Content: StatusOK}},
		}},
	}}}
	validateXrefs(courses)
	b := courses[0].Lessons[1].Slides
	if b[0].Content != StatusOK {
		t.Errorf("valid xref should stay OK, got %s", b[0].Content)
	}
	if b[1].Content != StatusWarnings {
		t.Errorf("dangling lesson xref should warn, got %s", b[1].Content)
	}
	if b[2].Content != StatusWarnings {
		t.Errorf("dangling slide xref should warn, got %s", b[2].Content)
	}
}
