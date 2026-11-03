package content

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mrmxf/nabu/internal/source"
)

// TestMediaRefs: media:<asset> becomes /content/<course>/<lesson>/media/<asset>
// in src, href and poster; an escaping ref warns and is left alone; and neither
// media: nor lms: is mistaken for a bundle file (no "not found" warnings).
func TestMediaRefs(t *testing.T) {
	src := "![clip](media:intro.mp4)\n\n[film](media:films/long.mp4#t=10)\n\n" +
		"<video src=\"media:a.mp4\" poster=\"media:a.jpg\"></video>\n\n" +
		"[bad](media:../escape.mp4)\n\n[res](lms://resources/brief.pdf)\n"
	ctx := RenderContext{
		Source:    source.Embed(fstest.MapFS{"c/l/010-s.md": {Data: []byte("x")}}, "content"),
		BundleDir: "c/l", LessonDir: "c/l", PublicRel: "content/wx/face/s",
		CourseSlug: "wx", LessonSlug: "face", Assets: &recSink{},
	}
	out, issues, err := NewGoldmarkRenderer().Render(src, ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`src="/content/wx/face/media/intro.mp4"`,
		`href="/content/wx/face/media/films/long.mp4#t=10"`,
		`src="/content/wx/face/media/a.mp4"`,
		`poster="/content/wx/face/media/a.jpg"`,
		`href="media:../escape.mp4"`,
		`href="lms://resources/brief.pdf"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Msg, "bad media reference: media:../escape.mp4") {
		t.Errorf("issues = %+v, want exactly the bad media ref", issues)
	}
}
