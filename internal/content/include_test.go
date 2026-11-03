package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrmxf/nabu/internal/source"
)

// writeLesson lays out a minimal lesson on disk and returns its dir.
func writeLesson(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The whole point of expanding before conversion: reference-style links in the
// slide resolve against definitions that live in an included file.
func TestIncludeResolvesReferenceLinks(t *testing.T) {
	dir := writeLesson(t, map[string]string{
		"_references.md": "[zapier]: https://zapier.com \"Zapier\"\n[n8n]: https://n8n.io\n",
	})
	src := "Try [Zapier][zapier] and [n8n][n8n].\n\n{{< LMS t=\"include\" from=\"_references.md\" >}}\n"

	out, _, err := NewGoldmarkRenderer().Render(src, RenderContext{Source: source.Disk(dir), BundleDir: ".", LessonDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `href="https://zapier.com"`) {
		t.Errorf("zapier reference did not resolve:\n%s", out)
	}
	if !strings.Contains(out, `href="https://n8n.io"`) {
		t.Errorf("n8n reference did not resolve:\n%s", out)
	}
	// the definitions themselves must not render as visible text
	if strings.Contains(out, "[zapier]:") {
		t.Errorf("link definitions leaked into output:\n%s", out)
	}
}

// A leaf-bundle slide (lesson/<slide>/index.md) must still reach the lesson's
// shared partial, without the author writing ../.
func TestIncludeFallsBackToLessonDir(t *testing.T) {
	lesson := writeLesson(t, map[string]string{
		"_references.md":   "[a]: https://example.com/a\n",
		"my-slide/keep.md": "unused",
	})
	src := "See [A][a].\n\n{{< LMS t=\"include\" from=\"_references.md\" >}}\n"

	out, issues, err := NewGoldmarkRenderer().Render(src, RenderContext{Source: source.Disk(lesson), BundleDir: "my-slide", LessonDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("unexpected issues: %+v", issues)
	}
	if !strings.Contains(out, `href="https://example.com/a"`) {
		t.Errorf("lesson-dir fallback failed:\n%s", out)
	}
}

// `only` narrows the splice to named keys, for a slide that wants a short list.
func TestIncludeOnlyFiltersKeys(t *testing.T) {
	dir := writeLesson(t, map[string]string{
		"_references.md": "[a]: https://example.com/a\n[b]: https://example.com/b\n",
	})
	src := "[A][a] [B][b]\n\n{{< LMS t=\"include\" from=\"_references.md\" only=\"a\" >}}\n"

	out, _, err := NewGoldmarkRenderer().Render(src, RenderContext{Source: source.Disk(dir), BundleDir: ".", LessonDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `href="https://example.com/a"`) {
		t.Errorf("kept key did not resolve:\n%s", out)
	}
	// b was filtered out, so [B][b] stays literal text rather than becoming a link
	if strings.Contains(out, `href="https://example.com/b"`) {
		t.Errorf("filtered key should not have resolved:\n%s", out)
	}
}

// A missing partial must not fail the build — the slide renders without it and
// the author is told.
func TestIncludeMissingFileWarns(t *testing.T) {
	dir := t.TempDir()
	src := "text\n\n{{< LMS t=\"include\" from=\"_nope.md\" >}}\n"

	out, issues, err := NewGoldmarkRenderer().Render(src, RenderContext{Source: source.Disk(dir), BundleDir: ".", LessonDir: "."})
	if err != nil {
		t.Fatalf("a missing include must not be fatal: %v", err)
	}
	if !strings.Contains(out, "text") {
		t.Errorf("slide should still render:\n%s", out)
	}
	if len(issues) != 1 || issues[0].Level != StatusWarnings {
		t.Fatalf("want one warning, got %+v", issues)
	}
	if !strings.Contains(issues[0].Msg, "_nope.md") {
		t.Errorf("warning should name the file, got %q", issues[0].Msg)
	}
}

// A partial is a lesson-scoped convenience, not a way to read arbitrary files at
// seed time. Traversal and absolute paths are refused outright.
func TestIncludeRefusesEscape(t *testing.T) {
	dir := writeLesson(t, map[string]string{"ok.md": "x"})
	outer := filepath.Dir(dir)
	if err := os.WriteFile(filepath.Join(outer, "secret.md"), []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, from := range []string{"../secret.md", "/etc/hostname"} {
		src := "{{< LMS t=\"include\" from=\"" + from + "\" >}}\n"
		out, issues, err := NewGoldmarkRenderer().Render(src, RenderContext{Source: source.Disk(dir), BundleDir: ".", LessonDir: "."})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "SECRET") {
			t.Fatalf("include escaped the lesson with %q:\n%s", from, out)
		}
		if len(issues) == 0 {
			t.Errorf("escaping include %q should report an issue", from)
		}
	}
}

// Included text is spliced verbatim and never rescanned, so a self-referencing
// partial terminates instead of looping.
func TestIncludeDoesNotNest(t *testing.T) {
	dir := writeLesson(t, map[string]string{
		"_loop.md": "inner\n\n{{< LMS t=\"include\" from=\"_loop.md\" >}}\n",
	})
	src := "{{< LMS t=\"include\" from=\"_loop.md\" >}}\n"

	out, _, err := NewGoldmarkRenderer().Render(src, RenderContext{Source: source.Disk(dir), BundleDir: ".", LessonDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "inner") {
		t.Errorf("first level should splice:\n%s", out)
	}
	// the nested one is left as an inert <lms> element, not expanded again
	if strings.Count(out, "inner") != 1 {
		t.Errorf("include should not nest:\n%s", out)
	}
}

// The help lessons document this widget in code fences; those must stay literal.
func TestIncludeInCodeFenceStaysLiteral(t *testing.T) {
	dir := writeLesson(t, map[string]string{"_references.md": "[a]: https://example.com/a\n"})
	src := "```\n{{< LMS t=\"include\" from=\"_references.md\" >}}\n```\n"

	out, issues, err := NewGoldmarkRenderer().Render(src, RenderContext{Source: source.Disk(dir), BundleDir: ".", LessonDir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("a documented example should not be processed: %+v", issues)
	}
	if !strings.Contains(out, `t=&quot;include&quot;`) && !strings.Contains(out, `t="include"`) {
		t.Errorf("fenced example should survive literally:\n%s", out)
	}
}
