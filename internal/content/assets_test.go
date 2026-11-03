package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recSink records every Copy call and returns the served URL the real sink would.
type recSink struct{ copied map[string]string } // publicRel -> url

func (s *recSink) Copy(srcPath, publicRel string) (string, error) {
	if s.copied == nil {
		s.copied = map[string]string{}
	}
	url := "/" + publicRel
	s.copied[publicRel] = url
	return url, nil
}

// TestBundleAssetCopy verifies that seeding copies (a) markdown images, (b) markdown
// asset links (PDF etc.), and (c) course/lesson frontmatter logos out of the content
// bundle, and that the resolved logo URLs are the served paths.
func TestBundleAssetCopy(t *testing.T) {
	root := t.TempDir()
	cls := filepath.Join(root, "testcourse")
	les := filepath.Join(cls, "testlesson")
	mustMkdir(t, les)

	write(t, filepath.Join(cls, "brand.svg"), "<svg/>")
	write(t, filepath.Join(cls, "_index.md"), "---\ntitle: C\nparams:\n  logo: brand.svg\n---\n")
	write(t, filepath.Join(les, "lessonlogo.svg"), "<svg/>")
	write(t, filepath.Join(les, "brief.pdf"), "%PDF fake")
	write(t, filepath.Join(les, "pic.png"), "PNG")
	write(t, filepath.Join(les, "_index.md"), "---\ntitle: L\nparams:\n  logo: lessonlogo.svg\n---\n")
	write(t, filepath.Join(les, "10-intro.md"),
		"---\ntitle: Intro\nparams:\n  kind: presentation\n---\n# Intro\n[brief](brief.pdf) ![pic](pic.png)\n")

	sink := &recSink{}
	courses, err := Load(root, NewGoldmarkRenderer(), Options{Assets: sink})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(courses) != 1 || len(courses[0].Lessons) != 1 {
		t.Fatalf("unexpected tree: %+v", courses)
	}
	c := courses[0]
	l := c.Lessons[0]

	// (c) logos resolved to their served bundle URLs.
	if c.Logo != "/content/testcourse/brand.svg" {
		t.Errorf("course logo = %q, want /content/testcourse/brand.svg", c.Logo)
	}
	if l.Logo != "/content/testcourse/testlesson/lessonlogo.svg" {
		t.Errorf("lesson logo = %q, want /content/testcourse/testlesson/lessonlogo.svg", l.Logo)
	}

	// (a) image + (b) pdf link copied under the slide's public dir.
	wantCopied := []string{
		"content/testcourse/brand.svg",
		"content/testcourse/testlesson/lessonlogo.svg",
		"content/testcourse/testlesson/intro/pic.png",
		"content/testcourse/testlesson/intro/brief.pdf",
	}
	for _, w := range wantCopied {
		if _, ok := sink.copied[w]; !ok {
			t.Errorf("expected copy of %q; copied=%v", w, keys(sink.copied))
		}
	}

	// the slide HTML should point href/src at the served URLs.
	html := l.Slides[0].HTML
	if !strings.Contains(html, "/content/testcourse/testlesson/intro/brief.pdf") {
		t.Errorf("pdf href not rewritten in HTML: %s", html)
	}
	if !strings.Contains(html, "/content/testcourse/testlesson/intro/pic.png") {
		t.Errorf("image src not rewritten in HTML: %s", html)
	}
}

// TestBundleEscapeNotCopied guards the traversal bug: a slide link that climbs
// above the page bundle (like the help docs' `../../../internal/x.go` repo
// cross-references) must be left as authored, never copied — so nothing is
// written outside the intended public dir.
func TestBundleEscapeNotCopied(t *testing.T) {
	root := t.TempDir()
	les := filepath.Join(root, "c", "l")
	mustMkdir(t, les)
	// a real file one level above the bundle
	write(t, filepath.Join(root, "c", "secret.go"), "package x")
	write(t, filepath.Join(root, "c", "_index.md"), "---\ntitle: C\n---\n")
	write(t, filepath.Join(les, "_index.md"), "---\ntitle: L\n---\n")
	write(t, filepath.Join(les, "10-s.md"),
		"---\ntitle: S\nparams:\n  kind: presentation\n---\n[up](../secret.go) ![up](../secret.go)\n")

	sink := &recSink{}
	courses, err := Load(root, NewGoldmarkRenderer(), Options{Assets: sink})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.copied) != 0 {
		t.Fatalf("escaping refs must not be copied; copied=%v", keys(sink.copied))
	}
	html := courses[0].Lessons[0].Slides[0].HTML
	if !strings.Contains(html, "../secret.go") {
		t.Errorf("escaping ref should be left as authored; html=%s", html)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}
func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
