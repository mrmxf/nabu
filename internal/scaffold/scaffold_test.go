package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateCourse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "weather-lab")
	if err := Create(Options{Dir: dir, Lesson: "weatherface"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, f := range []string{"nabu.yaml", ".gitignore", ".gitattributes", "README.md",
		"content/weather-lab/_index.md", "content/weather-lab/weatherface/_index.md",
		"content/weather-lab/weatherface/010-welcome.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	for _, f := range []string{"go.mod", "main.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("%s written without --library", f)
		}
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "nabu.yaml"))
	if !strings.Contains(string(cfg), "name: Weather Lab") {
		t.Errorf("nabu.yaml title not derived from the slug:\n%s", cfg)
	}
	ign, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(ign), "content/*/*/media/") {
		t.Error(".gitignore does not keep media out of git")
	}
	if err := Create(Options{Dir: dir}); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("second Create into the same dir: err = %v", err)
	}
}

func TestCreateLibraryCourse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "makerlab")
	if err := Create(Options{Dir: dir, Lesson: "weatherface", Library: true, Module: "gitlab.com/mrmxf/makerlab"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.HasPrefix(string(mod), "module gitlab.com/mrmxf/makerlab\n") {
		t.Errorf("go.mod = %s", mod)
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	for _, want := range []string{`"github.com/mrmxf/nabu"`, "//go:embed all:content", "nabu.Options{Content: course, ConfigYAML: config}"} {
		if !strings.Contains(string(main), want) {
			t.Errorf("main.go missing %s", want)
		}
	}
}

func TestCreateRejectsBadSlug(t *testing.T) {
	if err := Create(Options{Dir: filepath.Join(t.TempDir(), "x"), Course: "Bad Slug"}); err == nil {
		t.Error("want an error for a bad course slug")
	}
}
