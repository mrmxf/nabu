package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/internal/assetpipe"
	"github.com/mrmxf/nabu/internal/content"
	"github.com/mrmxf/nabu/internal/source"
)

// BuildMarker is written at the root of every `nabu build` output. A rebuild only
// clears a directory that carries it, so `--out` can never wipe an unrelated one.
const BuildMarker = ".nabu-build"

// IndexFile and LessonFile are the published JSON documents of a static build,
// under /content/ beside the lesson assets so they can never collide with an app
// route: /content/index.json and /content/<course>/<lesson>/lesson.json.
const (
	IndexFile  = "content/index.json"
	LessonFile = "lesson.json"
)

// StaticIndex is /content/index.json: every published course and lesson.
type StaticIndex struct {
	Generated time.Time      `json:"generated"`
	Courses   []StaticCourse `json:"courses"`
}

// StaticCourse is a course's metadata, field names matching the `courses`
// collection so the web app reads either source the same way.
type StaticCourse struct {
	Slug        string         `json:"slug"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary"`
	Order       int            `json:"order"`
	Number      string         `json:"number"`
	Logo        string         `json:"logo"`
	CSS         string         `json:"css"`
	GuestAccess bool           `json:"guest_access"`
	Help        bool           `json:"help"`
	Lessons     []StaticLesson `json:"lessons,omitempty"`
}

// StaticLesson is a lesson's metadata, matching the `lessons` collection.
type StaticLesson struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Order    int    `json:"order"`
	Number   string `json:"number"`
	Audience string `json:"audience"`
	Logo     string `json:"logo"`
	CSS      string `json:"css"`
}

// StaticSlide is one published slide, matching the `slides` collection (the
// web app's SlideRec). ID is "<course>/<lesson>/<slug>": stable, but NOT a
// PocketBase record id, so it cannot key per-slide state such as group notes.
type StaticSlide struct {
	ID         string                `json:"id"`
	Order      int                   `json:"order"`
	Slug       string                `json:"slug"`
	Number     string                `json:"number"`
	Kind       string                `json:"kind"`
	Audience   string                `json:"audience"`
	Title      string                `json:"title"`
	Body       string                `json:"body"`
	AgentKey   string                `json:"agent_key"`
	SimOutput  string                `json:"sim_output"`
	Choices    []content.ChoiceGroup `json:"t_choice"`
	Quizzes    []content.QuizGroup   `json:"t_quiz"`
	Agents     []content.AgentJob    `json:"t_agent"`
	Mappers    []content.DataMapper  `json:"t_mapper"`
	Timers     []content.Timer       `json:"timers"`
	NotesLabel string                `json:"notes_label"`
}

// StaticLessonDoc is /content/<course>/<lesson>/lesson.json.
type StaticLessonDoc struct {
	Course StaticCourse  `json:"course"`
	Lesson StaticLesson  `json:"lesson"`
	Slides []StaticSlide `json:"slides"`
}

// BuildOptions configures one static build.
type BuildOptions struct {
	Out      string // output directory
	Public   string // built web app on disk; "" => publicFS, then ./pb_public
	Renderer string
	Strict   bool
	Now      time.Time // publish gate; zero => time.Now()
}

// NewBuildCommand returns `nabu build`: render the course into a static site
// for any static host (Cloudflare Pages, GitHub/GitLab Pages, a CDN). It needs no
// database, so nabu.App runs it before PocketBase bootstraps (see RunBuildCommand).
// publicFS is the web app embedded in a library build (nabu.Options.Public).
func NewBuildCommand(src Sources, publicFS fs.FS) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Render the course into a static site (web app + published lessons)",
		Long: "Render the course and Nabu's help into a static site: the web app, the\n" +
			"published lessons as JSON (/content/<course>/<lesson>/lesson.json), their\n" +
			"assets, and a catalogue (/content/index.json). Only content that is live now\n" +
			"is published; drafts, scheduled lessons and teacher pages are left out.\n" +
			"Guest lessons then work with no server at all; signed-in features still need\n" +
			"`nabu serve`. Uses no database.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBuildFlags(cmd, src, publicFS)
		},
	}
	cmd.Flags().String("out", "dist", "output directory (cleared if it holds a previous build)")
	cmd.Flags().String("public", "", "built web app to copy (default: the embedded one, then ./pb_public)")
	cmd.Flags().String("content", "", "authored content tree (default: course.content from the config)")
	cmd.Flags().String("help-content", "", "Nabu help tree on disk (default: the help embedded in the binary)")
	cmd.Flags().String("assets", "", "assets tree (default: course.assets from the config)")
	cmd.Flags().String("renderer", defaultRenderer, "markdown renderer: goldmark|hugo")
	cmd.Flags().Bool("strict", false, "also fail on content warnings, not just errors")
	return cmd
}

func runBuildFlags(cmd *cobra.Command, src Sources, publicFS fs.FS) error {
	var p Paths
	p.Content, _ = cmd.Flags().GetString("content")
	p.Help, _ = cmd.Flags().GetString("help-content")
	p.Assets, _ = cmd.Flags().GetString("assets")
	var o BuildOptions
	o.Out, _ = cmd.Flags().GetString("out")
	o.Public, _ = cmd.Flags().GetString("public")
	o.Renderer, _ = cmd.Flags().GetString("renderer")
	o.Strict, _ = cmd.Flags().GetBool("strict")
	return Build(p, src, publicFS, o)
}

// Build writes a static site to o.Out. See NewBuildCommand.
func Build(p Paths, src Sources, publicFS fs.FS, o BuildOptions) error {
	if o.Out == "" {
		return errors.New("build: no output directory")
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	cp, err := CoursePaths()
	if err != nil {
		return err
	}
	trees := resolveTrees(p, src, cp)
	web := resolveWebApp(o.Public, publicFS)
	slog.Info("build sources", "content", trees.content.Label, "assets", trees.assets.Label,
		"help", trees.help.Label, "web", web.Label, "out", o.Out)
	if !web.Exists() {
		return fmt.Errorf("build: no built web app at %s — run scripts/build.sh, or pass --public", web.Label)
	}
	if err := prepareOut(o.Out); err != nil {
		return err
	}

	// 1. the web app, then the assets pipeline (assets/** -> /**, yaml -> json)
	if err := copyTree(web, o.Out); err != nil {
		return fmt.Errorf("build: copy web app: %w", err)
	}
	if err := assetpipe.Build(trees.assets, o.Out); err != nil {
		return fmt.Errorf("build: assets pipeline: %w", err)
	}

	// 2. render every course; bundle assets land in /content/... beside the JSON
	r, err := content.NewRenderer(o.Renderer)
	if err != nil {
		return err
	}
	sink := &fileAssetSink{publicDir: o.Out}
	idx := StaticIndex{Generated: o.Now.UTC()}
	var probs []content.Problem
	var lessons, slides, skipped int
	for _, root := range []contentRoot{{tree: trees.content}, {tree: trees.help, help: true}} {
		if !root.tree.Exists() {
			slog.Warn("content root not found; skipping", "source", root.tree.Label)
			continue
		}
		courses, err := content.Load(root.tree, r, content.Options{
			Assets:        sink,
			Data:          trees.assets.Sub("data"),
			ResolveLmsURL: newLmsResolver(),
			HelpRoot:      root.help,
		})
		if err != nil {
			return fmt.Errorf("build: load content %s: %w", root.tree.Label, err)
		}
		probs = append(probs, content.Problems(courses)...)
		for _, c := range courses {
			if !isLive(c.GoLive, c.Content, o.Now) {
				skipped++
				slog.Info("not published (not live)", "course", c.Slug, "readiness", c.Readiness)
				continue
			}
			sc := staticCourse(c, root.help)
			for _, l := range c.Lessons {
				if !isLive(l.GoLive, l.Content, o.Now) {
					skipped++
					slog.Info("not published (not live)", "course", c.Slug, "lesson", l.Slug, "readiness", l.Readiness)
					continue
				}
				doc := StaticLessonDoc{Course: sc, Lesson: staticLesson(l)}
				for _, s := range l.Slides {
					if !isLive(s.GoLive, s.Content, o.Now) {
						skipped++
						continue
					}
					doc.Slides = append(doc.Slides, staticSlide(c.Slug, l.Slug, s))
				}
				rel := path.Join("content", c.Slug, l.Slug, LessonFile)
				if err := writeJSON(filepath.Join(o.Out, filepath.FromSlash(rel)), doc); err != nil {
					return err
				}
				sc.Lessons = append(sc.Lessons, doc.Lesson)
				lessons++
				slides += len(doc.Slides)
				slog.Info("published lesson", "course", c.Slug, "lesson", l.Slug,
					"slides", len(doc.Slides), "source", l.Source, "file", "/"+rel)
			}
			idx.Courses = append(idx.Courses, sc)
		}
	}
	if err := reportContentProblems(probs, o.Strict); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(o.Out, filepath.FromSlash(IndexFile)), idx); err != nil {
		return err
	}
	slog.Info("build complete", "out", o.Out, "courses", len(idx.Courses), "lessons", lessons,
		"slides", slides, "not_live", skipped)
	return nil
}

// isLive mirrors the API's liveRule (go_live <= now, content not ERRORS/CORRUPT)
// at build time. A scheduled go-live in the future is NOT published: a static
// site cannot gate on the viewer's clock, so rebuild when it goes live.
func isLive(goLive *time.Time, st content.ContentStatus, now time.Time) bool {
	return goLive != nil && !goLive.After(now) &&
		st != content.StatusErrors && st != content.StatusCorrupt
}

// resolveWebApp picks the built web app: --public, then the embedded copy, then
// ./pb_public.
func resolveWebApp(flag string, embedded fs.FS) source.Tree {
	switch {
	case flag != "":
		return source.Disk(flag)
	case embedded != nil:
		return source.Embed(embedded, "web")
	default:
		return source.Disk("pb_public")
	}
}

// prepareOut makes out an empty directory. It refuses to clear a non-empty
// directory that is not a previous build (no BuildMarker), so a mistyped --out
// can never delete someone's work.
func prepareOut(out string) error {
	entries, err := os.ReadDir(out)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("build: read %s: %w", out, err)
	case len(entries) > 0:
		if _, err := os.Stat(filepath.Join(out, BuildMarker)); err != nil {
			return fmt.Errorf("build: %s is not empty and is not a previous nabu build (no %s); choose another --out", out, BuildMarker)
		}
		if err := os.RemoveAll(out); err != nil {
			return fmt.Errorf("build: clear %s: %w", out, err)
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("build: create %s: %w", out, err)
	}
	return os.WriteFile(filepath.Join(out, BuildMarker), []byte("generated by nabu build; safe to delete\n"), 0o644)
}

// copyTree copies every file of t under dst.
func copyTree(t source.Tree, dst string) error {
	return fs.WalkDir(t.FS, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return t.CopyTo(rel, filepath.Join(dst, filepath.FromSlash(rel)))
	})
}

func writeJSON(file string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("build: encode %s: %w", file, err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("build: %w", err)
	}
	return os.WriteFile(file, append(data, '\n'), 0o644)
}

func staticCourse(c content.Course, help bool) StaticCourse {
	return StaticCourse{Slug: c.Slug, Title: c.Title, Summary: c.Summary, Order: c.Order,
		Number: c.Number, Logo: c.Logo, CSS: c.CSS, GuestAccess: c.GuestAccess, Help: help}
}

func staticLesson(l content.Lesson) StaticLesson {
	return StaticLesson{Slug: l.Slug, Title: l.Title, Summary: l.Summary, Order: l.Order,
		Number: l.Number, Audience: l.Audience, Logo: l.Logo, CSS: l.CSS}
}

func staticSlide(course, lesson string, s content.Slide) StaticSlide {
	return StaticSlide{ID: course + "/" + lesson + "/" + s.Slug, Order: s.Order, Slug: s.Slug,
		Number: s.Number, Kind: s.Kind, Audience: s.Audience, Title: s.Title, Body: s.HTML,
		AgentKey: s.AgentKey, SimOutput: s.SimHTML, Choices: s.ChoiceGroups, Quizzes: s.QuizGroups,
		Agents: s.AgentJobs, Mappers: s.DataMappers, Timers: s.Timers, NotesLabel: s.NotesLabel}
}
