package seed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mrmxf/nabu/embedfs"
)

func md(title, extra, body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("---\ntitle: " + title + "\nparams:\n" + extra + "---\n" + body)}
}

// buildCourse is an embedded course with one live lesson (one live and one
// draft slide, an image, a teacher page), a draft lesson and a scheduled one.
func buildCourse() fstest.MapFS {
	return fstest.MapFS{
		"c/_index.md":             {Data: []byte("---\ntitle: C\nparams:\n  readiness: production\n  guest_access: true\n---\n")},
		"c/live/_index.md":        {Data: []byte("---\ntitle: Live\nparams:\n  readiness: production\n  teacher_pages:\n    start: teacher/start.md\n---\n")},
		"c/live/010-a.md":         md("A", "  readiness: production\n  kind: intro\n", "![p](pic.svg) [m](media:clip.mp4)\n"),
		"c/live/020-draft.md":     md("Draft", "  readiness: DRAFT\n  kind: intro\n", "not yet\n"),
		"c/live/pic.svg":          {Data: []byte("<svg/>")},
		"c/live/teacher/start.md": md("Teacher", "  readiness: production\n  kind: intro\n", "SECRET TEACHER NOTES\n"),
		"c/draft/_index.md":       {Data: []byte("---\ntitle: Draft lesson\nparams:\n  readiness: DRAFT\n---\n")},
		"c/draft/010-x.md":        md("X", "  readiness: production\n  kind: intro\n", "x\n"),
		"c/later/_index.md":       {Data: []byte("---\ntitle: Later\nparams:\n  readiness: 2099-09-01@09:00:00T+0100\n---\n")},
		"c/later/010-y.md":        md("Y", "  readiness: production\n  kind: intro\n", "y\n"),
	}
}

func TestBuildPublishesOnlyLiveContent(t *testing.T) {
	embedfs.OverrideFile = ""
	tmp := t.TempDir()
	web := filepath.Join(tmp, "web")
	if err := os.MkdirAll(filepath.Join(web, "_app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>app</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "dist")
	src := Sources{Content: buildCourse(), Help: fstest.MapFS{}}
	build := func() error {
		return Build(Paths{Assets: filepath.Join(tmp, "no-assets")}, src, nil, BuildOptions{Out: out, Public: web})
	}
	if err := build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, f := range []string{"index.html", BuildMarker, "content/index.json", "content/c/live/lesson.json", "content/c/live/a/pic.svg"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	for _, f := range []string{"content/c/draft/lesson.json", "content/c/later/lesson.json"} {
		if _, err := os.Stat(filepath.Join(out, f)); err == nil {
			t.Errorf("%s published but not live", f)
		}
	}

	raw, err := os.ReadFile(filepath.Join(out, "content/c/live/lesson.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET TEACHER NOTES") {
		t.Error("a teacher page was published")
	}
	var doc StaticLessonDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Course.GuestAccess || len(doc.Slides) != 1 || doc.Slides[0].ID != "c/live/a" {
		t.Fatalf("lesson doc = %+v", doc)
	}
	body := doc.Slides[0].Body
	if !strings.Contains(body, `/content/c/live/a/pic.svg`) || !strings.Contains(body, `/content/c/live/media/clip.mp4`) {
		t.Errorf("slide body not rewritten: %s", body)
	}

	var idx StaticIndex
	raw, _ = os.ReadFile(filepath.Join(out, "content/index.json"))
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Courses) != 1 || len(idx.Courses[0].Lessons) != 1 || idx.Courses[0].Lessons[0].Slug != "live" {
		t.Errorf("index = %+v", idx)
	}

	// a rebuild clears its own previous output…
	if err := build(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	// …but never a directory that is not a nabu build.
	other := filepath.Join(tmp, "precious")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "thesis.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Build(Paths{}, src, nil, BuildOptions{Out: other, Public: web})
	if err == nil || !strings.Contains(err.Error(), "not a previous nabu build") {
		t.Fatalf("build into a foreign dir: err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(other, "thesis.md")); err != nil {
		t.Fatal("foreign dir was touched")
	}
}

func TestBuildNeedsWebApp(t *testing.T) {
	err := Build(Paths{}, Sources{Content: buildCourse()}, nil,
		BuildOptions{Out: filepath.Join(t.TempDir(), "dist"), Public: filepath.Join(t.TempDir(), "missing")})
	if err == nil || !strings.Contains(err.Error(), "no built web app") {
		t.Fatalf("err = %v, want a missing-web-app error with a fix", err)
	}
}
