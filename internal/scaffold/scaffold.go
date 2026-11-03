// Package scaffold creates a new Nabu course repository (`nabu init`): the
// course config, one course with one lesson and slide, git ignore and LFS rules,
// and — for a library-form course — a go.mod and main.go that import Nabu.
package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

//go:embed templates
var templates embed.FS

// Options configures one scaffold.
type Options struct {
	Dir     string // target directory; created if missing, must be empty
	Course  string // course slug (folder under content/)
	Lesson  string // first lesson slug
	Title   string // course title
	Library bool   // also write go.mod + main.go importing Nabu
	Module  string // Go module path (library only)
}

// data is what the templates see.
type data struct {
	Options
	LessonTitle string
	Binary      string
}

// defaultLesson is the first lesson's slug when none is given.
const defaultLesson = "first-lesson"

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// NewCommand returns `nabu init <dir>`. It touches no database, so nabu.App
// runs it before PocketBase bootstraps.
func NewCommand() *cobra.Command {
	var o Options
	cmd := &cobra.Command{
		Use:   "init <dir>",
		Short: "Create a new course repository",
		Long: "Create a new Nabu course in <dir>: nabu.yaml, content/<course>/<lesson>/\n" +
			"with a welcome slide, .gitignore (media and build output), .gitattributes\n" +
			"(git-lfs for binaries) and a README. With --library, also a go.mod and\n" +
			"main.go that build the course into its own binary with Nabu as a library.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			o.Dir = args[0]
			return Create(o)
		},
	}
	cmd.Flags().StringVar(&o.Course, "course", "", "course slug (default: the directory name)")
	cmd.Flags().StringVar(&o.Lesson, "lesson", defaultLesson, "first lesson slug")
	cmd.Flags().StringVar(&o.Title, "title", "", "course title (default: from the course slug)")
	cmd.Flags().BoolVar(&o.Library, "library", false, "also write go.mod + main.go (Nabu as a Go library)")
	cmd.Flags().StringVar(&o.Module, "module", "", "Go module path for --library (default: example.nabu/<course>)")
	return cmd
}

// Create writes the scaffold. It refuses a non-empty target directory.
func Create(o Options) error {
	if o.Dir == "" {
		return errors.New("init: no directory")
	}
	if o.Course == "" {
		o.Course = strings.ToLower(filepath.Base(filepath.Clean(o.Dir)))
	}
	if o.Lesson == "" {
		o.Lesson = defaultLesson
	}
	for name, v := range map[string]string{"course": o.Course, "lesson": o.Lesson} {
		if !slugRe.MatchString(v) {
			return fmt.Errorf("init: %s slug %q must be lowercase letters, digits and single hyphens", name, v)
		}
	}
	if o.Title == "" {
		o.Title = titleCase(o.Course)
	}
	if o.Module == "" {
		o.Module = "example.nabu/" + o.Course
	}
	if entries, err := os.ReadDir(o.Dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("init: %s is not empty", o.Dir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("init: %w", err)
	}

	d := data{Options: o, LessonTitle: titleCase(o.Lesson), Binary: o.Course}
	lessonDir := filepath.Join("content", o.Course, o.Lesson)
	files := []struct{ tmpl, out string }{
		{"nabu.yaml.tmpl", "nabu.yaml"},
		{"gitignore.tmpl", ".gitignore"},
		{"gitattributes.tmpl", ".gitattributes"},
		{"README.md.tmpl", "README.md"},
		{"course_index.md.tmpl", filepath.Join("content", o.Course, "_index.md")},
		{"lesson_index.md.tmpl", filepath.Join(lessonDir, "_index.md")},
		{"slide_welcome.md.tmpl", filepath.Join(lessonDir, "010-welcome.md")},
	}
	if o.Library {
		files = append(files,
			struct{ tmpl, out string }{"library/go.mod.tmpl", "go.mod"},
			struct{ tmpl, out string }{"library/main.go.tmpl", "main.go"})
	}
	for _, f := range files {
		if err := render(f.tmpl, filepath.Join(o.Dir, f.out), d); err != nil {
			return err
		}
	}
	slog.Info("course created", "dir", o.Dir, "course", o.Course, "lesson", o.Lesson, "library", o.Library)
	return nil
}

func render(tmpl, out string, d data) error {
	src, err := templates.ReadFile("templates/" + tmpl)
	if err != nil {
		return fmt.Errorf("init: template %s: %w", tmpl, err)
	}
	t, err := template.New(tmpl).Parse(string(src))
	if err != nil {
		return fmt.Errorf("init: parse %s: %w", tmpl, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return fmt.Errorf("init: render %s: %w", tmpl, err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}

// titleCase turns a slug into a title: "my-first-course" -> "My First Course".
func titleCase(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
