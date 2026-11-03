package content

import (
	"strings"
	"testing"
)

// render one slide body through the real pipeline (no filesystem needed).
func renderBody(t *testing.T, src string) string {
	t.Helper()
	out, _, err := NewGoldmarkRenderer().Render(src, RenderContext{})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A paired container must wrap its Markdown, and that Markdown must still be
// converted — the blank-line rule is what makes the inner content render.
func TestDefineWrapsMarkdown(t *testing.T) {
	out := renderBody(t, `{{< LMS t="define" tone="warn" header="Watch out" >}}

A **brittle** workflow.

{{< /LMS >}}`)

	for _, want := range []string{
		`<aside class="lms-define lms-tone-warn">`,
		`<span class="lms-define-title">Watch out</span>`,
		"<strong>brittle</strong>",
		"</aside>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// An author writes a tone, never a colour — that is what makes a course
// re-themeable from its palette.
func TestToneEmitsClassNotColour(t *testing.T) {
	out := renderBody(t, `{{< LMS t="define" tone="ok" >}}

x

{{< /LMS >}}`)
	if !strings.Contains(out, "lms-tone-ok") {
		t.Errorf("tone class missing:\n%s", out)
	}
	if strings.Contains(out, "hsl(") || strings.Contains(out, "#") {
		t.Errorf("a widget must not emit a colour:\n%s", out)
	}
	// an unknown tone degrades to no tone class rather than a dead one
	out = renderBody(t, `{{< LMS t="define" tone="chartreuse" >}}

x

{{< /LMS >}}`)
	if strings.Contains(out, "lms-tone-chartreuse") {
		t.Errorf("unknown tone should not emit a class:\n%s", out)
	}
}

// Containers nest because every one closes as </aside>, so opens and closes pair
// up in document order.
func TestGridAndStepsNest(t *testing.T) {
	out := renderBody(t, `{{< LMS t="proscons" >}}

{{< LMS t="col" title="Pros" tone="ok" >}}

fast

{{< /LMS >}}

{{< LMS t="col" title="Cons" tone="attention" >}}

fragile

{{< /LMS >}}

{{< /LMS >}}`)

	if !strings.Contains(out, `class="lms-grid lms-proscons"`) {
		t.Errorf("proscons wrapper missing:\n%s", out)
	}
	if got := strings.Count(out, "<aside"); got != 3 {
		t.Errorf("opens = %d, want 3:\n%s", got, out)
	}
	if got := strings.Count(out, "</aside>"); got != 3 {
		t.Errorf("closes = %d, want 3:\n%s", got, out)
	}
}

// `start` seeds the CSS counter so a procedure can continue across slides.
func TestStepsStartSeedsCounter(t *testing.T) {
	out := renderBody(t, `{{< LMS t="steps" start="4" >}}

{{< LMS t="step" title="Next" >}}

do it

{{< /LMS >}}

{{< /LMS >}}`)
	if !strings.Contains(out, "counter-reset:lms-step 3") {
		t.Errorf("counter not seeded:\n%s", out)
	}
}

// The grid's inline custom properties are the one place authored values reach a
// style attribute, so junk must be dropped rather than stamped through.
func TestGridRejectsBadLengths(t *testing.T) {
	out := renderBody(t, `{{< LMS t="grid" cols="99" min="18rem; background:url(x)" >}}

x

{{< /LMS >}}`)
	if strings.Contains(out, "background") {
		t.Errorf("a bad length must not reach the style attribute:\n%s", out)
	}
	if strings.Contains(out, "--lms-grid-cols:99") {
		t.Errorf("out-of-range column count should be ignored:\n%s", out)
	}
}

func TestVideoResolvesToNocookieEmbed(t *testing.T) {
	for _, src := range []string{
		"https://www.youtube.com/watch?v=E0hrcDO3Noc",
		"https://youtu.be/E0hrcDO3Noc",
		"https://www.youtube.com/embed/E0hrcDO3Noc?si=2XQ2I8QcU7bcbhfx",
		"E0hrcDO3Noc",
	} {
		out := renderBody(t, `{{< LMS t="video" src="`+src+`" >}}`)
		if !strings.Contains(out, "https://www.youtube-nocookie.com/embed/E0hrcDO3Noc") {
			t.Errorf("src %q did not resolve to a nocookie embed:\n%s", src, out)
		}
		// nothing from the authored URL is carried through
		if strings.Contains(out, "si=") {
			t.Errorf("tracking params leaked from %q:\n%s", src, out)
		}
	}
}

// An <iframe src> from slide text is where authored content becomes executable
// third-party code. Anything not a recognised video renders nothing at all.
func TestVideoRejectsArbitrarySources(t *testing.T) {
	for _, src := range []string{
		"https://evil.example.com/embed/x",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"//evil.example.com/x",
		"",
	} {
		out := renderBody(t, `{{< LMS t="video" src="`+src+`" >}}`)
		if strings.Contains(out, "<iframe") {
			t.Errorf("src %q must not produce an iframe:\n%s", src, out)
		}
	}
}

func TestToolCardOnlyLinksHTTP(t *testing.T) {
	out := renderBody(t, `{{< LMS t="toolcard" name="Zapier" url="https://zapier.com" cost="Free tier" >}}

Automation.

{{< /LMS >}}`)
	if !strings.Contains(out, `href="https://zapier.com"`) {
		t.Errorf("http(s) url should link:\n%s", out)
	}
	if !strings.Contains(out, `rel="noopener noreferrer"`) {
		t.Errorf("external link needs rel=noopener:\n%s", out)
	}

	out = renderBody(t, `{{< LMS t="toolcard" name="Bad" url="javascript:alert(1)" >}}

x

{{< /LMS >}}`)
	if strings.Contains(out, "javascript:") {
		t.Errorf("non-http url must not become an href:\n%s", out)
	}
	if !strings.Contains(out, "Bad") {
		t.Errorf("the name should still render:\n%s", out)
	}
}

// Every authored string reaches an HTML attribute or text node, so all of them
// must be escaped.
func TestWidgetsEscapeAuthoredText(t *testing.T) {
	for _, src := range []string{
		`{{< LMS t="banner" text="<script>alert(1)</script>" >}}`,
		`{{< LMS t="itemtype" kind="assignment" >}}`,
		`{{< LMS t="vocab" term="<img src=x onerror=alert(1)>" pos="Noun" >}}

d

{{< /LMS >}}`,
		`{{< LMS t="toolcard" name="<script>x</script>" focus="<b>no</b>" >}}

d

{{< /LMS >}}`,
	} {
		out := renderBody(t, src)
		// The check is for an unescaped TAG. Escaped text may legitimately contain
		// the word "onerror" — `&lt;img src=x onerror=…&gt;` is inert, and is the
		// correct result.
		for _, bad := range []string{"<script", "<img", "<b>"} {
			if strings.Contains(out, bad) {
				t.Errorf("unescaped %q leaked from %s:\n%s", bad, src, out)
			}
		}
	}
}

func TestBannerUsesToneVariables(t *testing.T) {
	out := renderBody(t, `{{< LMS t="banner" text="Part 2" from="attention" to="info" >}}`)
	if !strings.Contains(out, "--lms-banner-from:var(--attention)") {
		t.Errorf("banner should reference the tone variable:\n%s", out)
	}
	if !strings.Contains(out, "Part 2") {
		t.Errorf("banner text missing:\n%s", out)
	}
}

func TestItemTypeBadge(t *testing.T) {
	out := renderBody(t, `{{< LMS t="itemtype" kind="quiz" >}}`)
	if !strings.Contains(out, `class="lms-itemtype lms-itemtype-quiz"`) || !strings.Contains(out, "Quiz") {
		t.Errorf("badge markup wrong:\n%s", out)
	}
}

// Every presentational widget must be in the catalogue, or authors get a
// spurious "unknown widget" warning on a widget that works.
func TestPresentationalWidgetsAreCatalogued(t *testing.T) {
	types, err := widgetTypes()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, x := range types {
		have[x] = true
	}
	for _, want := range []string{
		"define", "steps", "step", "grid", "proscons", "col",
		"vocab", "toolcard", "banner", "itemtype", "video", "include",
	} {
		if !have[want] {
			t.Errorf("catalogue missing widget %q", want)
		}
	}
}

// A catalogued widget that renders server-side must not also be emitted as an
// <lms> element for the client to hydrate — that would mount nothing.
func TestPresentationalWidgetsRenderServerSide(t *testing.T) {
	for _, src := range []string{
		`{{< LMS t="define" >}}`,
		`{{< LMS t="banner" text="x" >}}`,
		`{{< LMS t="itemtype" kind="page" >}}`,
		`{{< LMS t="video" src="E0hrcDO3Noc" >}}`,
	} {
		if out := renderBody(t, src); strings.Contains(out, "<lms ") {
			t.Errorf("%s should render server-side, got:\n%s", src, out)
		}
	}
}
