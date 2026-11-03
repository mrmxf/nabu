package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mrmxf/nabu/internal/source"
)

// embeddedCourse is a one-slide course whose slide references a missing image,
// an existing image and a missing include, so each message's location shows.
func embeddedCourse() fstest.MapFS {
	return fstest.MapFS{
		"c/_index.md":   {Data: []byte("---\ntitle: C\nparams:\n  readiness: production\n---\n")},
		"c/l/_index.md": {Data: []byte("---\ntitle: L\nparams:\n  readiness: production\n---\n")},
		"c/l/face.png":  {Data: []byte("png")},
		"c/l/010-s.md":  {Data: []byte("---\ntitle: S\nparams:\n  kind: intro\n  readiness: production\n---\n![ok](face.png) ![gone](missing.png)\n\n{{< LMS t=\"include\" from=\"_nope.md\" >}}\n")},
	}
}

// TestEmbeddedMessagesNameEmbedLocation: a tree baked into the binary is named
// embed://… in lesson sources and content issues, never a temp or disk path.
func TestEmbeddedMessagesNameEmbedLocation(t *testing.T) {
	sink := &recSink{}
	courses, err := Load(source.Embed(embeddedCourse(), "content"), NewGoldmarkRenderer(), Options{Assets: sink})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	l := courses[0].Lessons[0]
	if l.Source != "embed://content/c/l" {
		t.Errorf("lesson source = %q, want embed://content/c/l", l.Source)
	}
	issue := l.Slides[0].Issue
	for _, want := range []string{"image not found: embed://content/c/l/missing.png", "embed://content/c/l/_nope.md"} {
		if !strings.Contains(issue, want) {
			t.Errorf("slide issue %q missing %q", issue, want)
		}
	}
	if _, ok := sink.copied["content/c/l/s/face.png"]; !ok {
		t.Errorf("existing embedded image not copied: %v", sink.copied)
	}
}

// TestDiskMessagesNameAbsolutePath: the same course on disk is named by its
// absolute path.
func TestDiskMessagesNameAbsolutePath(t *testing.T) {
	root := t.TempDir()
	for name, f := range embeddedCourse() {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, p, string(f.Data))
	}
	courses, err := Load(source.Disk(root), NewGoldmarkRenderer(), Options{Assets: &recSink{}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	l := courses[0].Lessons[0]
	want := source.Disk(root).Where("c/l")
	if l.Source != want || !filepath.IsAbs(l.Source) {
		t.Errorf("lesson source = %q, want absolute %q", l.Source, want)
	}
	if !strings.Contains(l.Slides[0].Issue, "image not found: "+want+"/missing.png") {
		t.Errorf("slide issue %q does not name the disk path", l.Slides[0].Issue)
	}
}
