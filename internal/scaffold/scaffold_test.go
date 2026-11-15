package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func read(t *testing.T, dir, f string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, f))
	if err != nil {
		t.Fatalf("read %s: %v", f, err)
	}
	return string(b)
}

// parseYAML fails the test when f is not valid YAML.
func parseYAML(t *testing.T, dir, f string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := yaml.Unmarshal([]byte(read(t, dir, f)), &v); err != nil {
		t.Fatalf("%s is not valid YAML: %v\n%s", f, err, read(t, dir, f))
	}
	return v
}

func TestCreateCourse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-lab")
	if err := Create(Options{Dir: dir, Lesson: "first-steps"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, f := range []string{"nabu.yaml", ".gitignore", ".gitattributes", "README.md",
		"content/sample-lab/_index.md", "content/sample-lab/first-steps/_index.md",
		"content/sample-lab/first-steps/010-welcome.md",
		".clog.yaml", ".clog-version", ".github/workflows/course.yaml",
		"content/lesson-schedules.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	for _, f := range []string{"go.mod", "main.go", ".gitlab-ci.yml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("%s written without asking for it", f)
		}
	}
	if cfg := read(t, dir, "nabu.yaml"); !strings.Contains(cfg, "name: Sample Lab") {
		t.Errorf("nabu.yaml title not derived from the slug:\n%s", cfg)
	}
	if ign := read(t, dir, ".gitignore"); !strings.Contains(ign, "content/*/*/media/") || !strings.Contains(ign, "_clog_build/") ||
		!strings.Contains(ign, "/roster.yaml") {
		t.Errorf(".gitignore does not keep media and build output out of git:\n%s", ign)
	}
	sched := parseYAML(t, dir, "content/lesson-schedules.yaml")
	if ls, _ := sched["lesson-schedules"].([]any); len(ls) != 1 || ls[0].(map[string]any)["lesson"] != "first-steps" {
		t.Errorf("lesson-schedules.yaml does not open a Session on the first lesson: %v", sched)
	}
	if err := Create(Options{Dir: dir}); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("second Create into the same dir: err = %v", err)
	}
}

func TestCreateWritesClogFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-lab")
	if err := Create(Options{Dir: dir}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	clog := parseYAML(t, dir, ".clog.yaml")
	ci, _ := clog["ci"].(map[string]any)
	stacks, _ := ci["stack"].([]any)
	if len(stacks) != 1 {
		t.Fatalf("ci.stack = %v, want one site stack", ci["stack"])
	}
	site, _ := stacks[0].(map[string]any)
	if site["name"] != "site" || !strings.Contains(strings.Join(toStrings(site["make"]), " "), "nabu") {
		t.Errorf("site stack = %v", site)
	}
	if _, on := ci["policy"].(map[string]any)["deploy"]; on {
		t.Error("deploy is on in a new course: it must wait for targets and secrets")
	}
	if _, set := ci["infisical"]; set {
		t.Error("a new course has a ci.infisical block: a placeholder one is a clog error")
	}
	snip, _ := clog["snippets"].(map[string]any)
	if bc, _ := snip["bc-nabu"].(string); !strings.Contains(bc, "nabu build --strict --out") {
		t.Errorf("bc-nabu does not run nabu build:\n%s", bc)
	}
	if v := strings.TrimSpace(lastLine(read(t, dir, ".clog-version"))); v != ClogVersion {
		t.Errorf(".clog-version = %q, want %s", v, ClogVersion)
	}
	wf := parseYAML(t, dir, ".github/workflows/course.yaml")
	raw := read(t, dir, ".github/workflows/course.yaml")
	if !strings.Contains(raw, "build-check.yaml@"+ClogVersion) || !strings.Contains(raw, "${{ github.event_name") {
		t.Errorf("workflow not pinned to clog %s, or GitHub expressions mangled:\n%s", ClogVersion, raw)
	}
	if wf["jobs"] == nil {
		t.Error("workflow has no jobs")
	}
}

func TestCreateGitLabAndNone(t *testing.T) {
	gl := filepath.Join(t.TempDir(), "gl")
	if err := Create(Options{Dir: gl, CI: "gitlab"}); err != nil {
		t.Fatalf("Create gitlab: %v", err)
	}
	parseYAML(t, gl, ".gitlab-ci.yml")
	if raw := read(t, gl, ".gitlab-ci.yml"); !strings.Contains(raw, "mrmxf/clog/"+ClogVersion+"/gitlab/clog.gitlab-ci.yml") {
		t.Errorf("GitLab include not pinned to clog %s:\n%s", ClogVersion, raw)
	}
	if _, err := os.Stat(filepath.Join(gl, ".github")); err == nil {
		t.Error("--ci gitlab also wrote a GitHub workflow")
	}
	none := filepath.Join(t.TempDir(), "none")
	if err := Create(Options{Dir: none, CI: "none"}); err != nil {
		t.Fatalf("Create none: %v", err)
	}
	for _, f := range []string{".github", ".gitlab-ci.yml"} {
		if _, err := os.Stat(filepath.Join(none, f)); err == nil {
			t.Errorf("--ci none wrote %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(none, ".clog.yaml")); err != nil {
		t.Error("--ci none must still write .clog.yaml: clog build works on a laptop")
	}
	if err := Create(Options{Dir: filepath.Join(t.TempDir(), "x"), CI: "jenkins"}); err == nil {
		t.Error("want an error for an unknown --ci")
	}
}

func TestCreateLibraryCourse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-lab")
	if err := Create(Options{Dir: dir, Lesson: "first-steps", Library: true, Module: "example.nabu/sample-lab"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mod := read(t, dir, "go.mod"); !strings.HasPrefix(mod, "module example.nabu/sample-lab\n") {
		t.Errorf("go.mod = %s", mod)
	}
	main := read(t, dir, "main.go")
	for _, want := range []string{`"github.com/mrmxf/nabu"`, "//go:embed all:content",
		"nabu.Options{Content: course, ConfigYAML: config}", `opts.Public = os.DirFS("nabu-web")`} {
		if !strings.Contains(main, want) {
			t.Errorf("main.go missing %s", want)
		}
	}
	clog := parseYAML(t, dir, ".clog.yaml")
	stacks, _ := clog["ci"].(map[string]any)["stack"].([]any)
	if len(stacks) != 2 {
		t.Errorf("library course: %d stacks, want site + binary", len(stacks))
	}
	if bc := clog["snippets"].(map[string]any)["bc-nabu"].(string); !strings.Contains(bc, "go run . web fetch") {
		t.Errorf("library bc-nabu does not fetch the web app:\n%s", bc)
	}
	ign := read(t, dir, ".gitignore")
	for _, want := range []string{"/nabu-web/", "/sample-lab\n"} {
		if !strings.Contains(ign, want) {
			t.Errorf(".gitignore missing %q", want)
		}
	}
	if strings.Contains(ign, "\nsample-lab\n") {
		t.Error(".gitignore ignores the course name everywhere, which hides content/<course>/")
	}
}

func TestCreateRejectsBadSlug(t *testing.T) {
	if err := Create(Options{Dir: filepath.Join(t.TempDir(), "x"), Course: "Bad Slug"}); err == nil {
		t.Error("want an error for a bad course slug")
	}
}

// A new course pins the clog this repo is built with.
func TestClogVersionMatchesRepo(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".clog-version"))
	if err != nil {
		t.Fatalf("read the repo's .clog-version: %v", err)
	}
	if got := strings.TrimSpace(lastLine(string(b))); got != ClogVersion {
		t.Errorf("scaffold.ClogVersion = %s, repo .clog-version = %s: change both together", ClogVersion, got)
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func toStrings(v any) []string {
	var out []string
	if list, ok := v.([]any); ok {
		for _, x := range list {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
