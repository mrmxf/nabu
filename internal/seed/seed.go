// Package seed populates the Nabu database with demo accounts, the
// global config, and the authored lessons ingested from the on-disk content
// tree so the Phase 2 browser demo works out of the box.
//
// It is idempotent: users are upserted by email, courses/lessons by slug, and
// slides are rebuilt (upsert-by-slug) from the content tree on every run so
// edits to the authored Markdown take effect on the next `seed`.
package seed

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/help"
	"github.com/mrmxf/nabu/internal/assetpipe"
	"github.com/mrmxf/nabu/internal/content"
	"github.com/mrmxf/nabu/internal/demo"
	"github.com/mrmxf/nabu/internal/lmsfs"
	"github.com/mrmxf/nabu/internal/s3svc"
	"github.com/mrmxf/nabu/internal/serverctl"
	"github.com/mrmxf/nabu/internal/students"
)

// All demo principals — students, teacher, app-admin AND the dashboard
// superuser — live in the demo roster (internal/demo) and are applied through the
// one user-import path (internal/students.Apply). They are seeded only when
// demo_mode is on; the superuser's password is sourced from the Infisical-injected
// env var its entry names (never a literal).
//
// The content, assets and schedules paths come from the course config
// (course.* in nabu-config.yaml / the course's nabu.yaml); a flag given on the
// command line overrides them. Nabu's own help is embedded (package help)
// unless --help-content names a tree on disk.
const defaultRenderer = "goldmark"

// Paths names the trees one seed reads. Empty fields take the course config
// value (see CoursePaths).
type Paths struct {
	Content   string // authored content tree
	Help      string // help tree on disk; "" => the embedded help
	Assets    string // Hugo-style assets tree, published to pb_public
	Schedules string // lesson-schedules YAML pre-load (skipped if absent)
}

// CoursePaths returns the course config's paths (course.*), each resolved
// relative to the config file that set it.
func CoursePaths() (Paths, error) {
	cfg, err := embedfs.LoadConfig()
	if err != nil {
		return Paths{}, fmt.Errorf("load course config: %w", err)
	}
	return Paths{
		Content:   embedfs.ResolvePath(cfg.Course.Content),
		Assets:    embedfs.ResolvePath(cfg.Course.Assets),
		Schedules: embedfs.ResolvePath(cfg.Course.Schedules),
	}, nil
}

// NewCommand returns the `seed` subcommand. The app is already bootstrapped by
// PocketBase before the command runs, so the DB is open.
func NewCommand(app core.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Populate demo accounts, config, and lessons from the content tree",
		Long: "Idempotently seed config, the content tree, and — when demo_mode is on —\n" +
			"the demo accounts from the demo roster (students, teacher,\n" +
			"app-admin and the dashboard superuser), all via the shared user-import path.\n\n" +
			"The demo superuser's password comes from NABU_SUPERUSER_PW (never a literal);\n" +
			"wrap with Infisical to bake it in, else it is skipped and you create the\n" +
			"first superuser at /_/ on first run:\n" +
			"  infisical run --env prod -- nabu seed",
		SilenceUsage: true, // a runtime failure shouldn't dump the flag list
		RunE: func(cmd *cobra.Command, args []string) error {
			// Safety interlock: seeding a live database races the running server
			// (SQLite writes and pb_public asset copies). Refuse and point the
			// operator at `stop` first. --force overrides for the rare hot reseed.
			force, _ := cmd.Flags().GetBool("force")
			if running, pid := serverctl.ServerRunning(app.DataDir()); running && !force {
				return fmt.Errorf("for safety, stop the server before seeding — a server is running (pid %d); run `nabu stop` first (or pass --force to override)", pid)
			}
			var p Paths
			p.Content, _ = cmd.Flags().GetString("content")
			p.Help, _ = cmd.Flags().GetString("help-content")
			p.Assets, _ = cmd.Flags().GetString("assets")
			p.Schedules, _ = cmd.Flags().GetString("schedules")
			renderer, _ := cmd.Flags().GetString("renderer")
			strict, _ := cmd.Flags().GetBool("strict")
			return Run(app, p, renderer, strict)
		},
	}
	cmd.Flags().String("content", "", "authored content tree (default: course.content from the config)")
	cmd.Flags().String("help-content", "", "Nabu help tree on disk (default: the help embedded in the binary)")
	cmd.Flags().String("assets", "", "assets tree published to pb_public (default: course.assets from the config)")
	cmd.Flags().String("renderer", defaultRenderer, "markdown renderer: goldmark|hugo")
	cmd.Flags().String("schedules", "", "lesson-schedules YAML pre-load, skipped if absent (default: course.schedules from the config)")
	cmd.Flags().Bool("force", false, "seed even if a server is running (races the live DB; use with care)")
	cmd.Flags().Bool("strict", false, "also fail on content warnings (broken xrefs, missing images, bad audience), not just errors")
	return cmd
}

// Run performs the idempotent seed. Empty fields of p take the course config's
// paths; an empty p.Help seeds the help embedded in the binary.
func Run(app core.App, p Paths, renderer string, strict bool) error {
	cp, err := CoursePaths()
	if err != nil {
		return err
	}
	if p.Content == "" {
		p.Content = cp.Content
	}
	if p.Assets == "" {
		p.Assets = cp.Assets
	}
	if p.Schedules == "" {
		p.Schedules = cp.Schedules
	}
	if renderer == "" {
		renderer = defaultRenderer
	}
	if p.Help == "" {
		dir, cleanup, err := extractEmbeddedHelp()
		if err != nil {
			return err
		}
		defer cleanup()
		p.Help = dir
	}
	contentDir, helpDir, schedulesPath := p.Content, p.Help, p.Schedules

	// Apply any pending schema migrations first, so seeding a brand-new data dir
	// works in one step (the collections it writes to must exist). Idempotent — a
	// no-op once migrated. Without this, `seed` on a never-served ./pb_data fails
	// with "find courses collection: no rows".
	if err := app.RunAppMigrations(); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	// Demo accounts — students, teacher, app-admin AND the dashboard superuser —
	// are config-gated. When demo_mode is on they are applied from
	// the demo roster (internal/demo) through the shared user-import path, whose
	// `courses` field also enrols the students; the ingest below creates the
	// courses they reference. When off, none are created (not even the demo
	// superuser) — a real deployment supplies its own via `nabu users import` or
	// PocketBase's first-run "create your first superuser" page at /_/.
	demoMode := demoModeEnabled()
	if err := ingestContent(app, []contentRoot{
		{dir: contentDir},
		{dir: helpDir, help: true},
	}, p.Assets, renderer, strict); err != nil {
		return err
	}

	// publish the Hugo-style assets tree (assets/** -> pb_public/**, yaml->json)
	// so t_mapper data files are served, e.g. assets/data/x.yaml -> /data/x.json.
	if err := assetpipe.Build(p.Assets, publicDir(app.DataDir())); err != nil {
		return fmt.Errorf("assets pipeline: %w", err)
	}

	// apply the demo roster after the courses exist so enrolment can resolve.
	if err := seedDemoAccounts(app, demoMode); err != nil {
		return err
	}

	// seed teacher-authored lesson schedules (lessons now exist). Additive: rows a
	// teacher later adds in the dashboard are never pruned by a re-seed.
	if err := seedSchedules(app, schedulesPath); err != nil {
		return err
	}

	if err := upsertSettings(app, settingsWithConfig()); err != nil {
		return err
	}
	slog.Info("seeded settings", "count", len(defaultSettings))
	slog.Info("seed complete", "demo_mode", demoMode)
	return nil
}

// demoModeEnabled reports whether demo accounts should be seeded, from
// nabu-config.yaml `demo_mode`. A config read failure is treated as demo_mode on
// (the Phase 2 default) so a broken config never silently strands the demo.
func demoModeEnabled() bool {
	cfg, err := embedfs.LoadConfig()
	if err != nil {
		slog.Warn("could not read config for demo_mode; defaulting to on", "err", err)
		return true
	}
	return cfg.DemoMode
}

// seedDemoAccounts applies the demo roster (students, teacher,
// app-admin and the dashboard superuser) in merge mode when demo_mode is on. The
// roster's `courses` field enrols the students into the showcase course, and a
// `role: superuser` entry is routed to _superusers (password from its env var),
// so no separate enrolment or superuser step is needed. When demo_mode is off it
// does nothing — leftover demo accounts are the login page's concern (see
// internal/httpapi/demo.go), not the seeder's.
func seedDemoAccounts(app core.App, demoMode bool) error {
	if !demoMode {
		slog.Info("demo_mode off — skipping demo accounts")
		return nil
	}
	raw, err := demo.RosterYAML()
	if err != nil {
		return err
	}
	recs, err := students.DecodeRoster(raw, students.FormatYAML)
	if err != nil {
		return fmt.Errorf("decode demo roster: %w", err)
	}
	sum, err := students.Apply(app, recs, students.ModeMerge, "")
	if err != nil {
		return fmt.Errorf("apply demo roster: %w", err)
	}
	if len(sum.Errors) > 0 {
		// A per-row problem (e.g. a course not yet ingested) must not pass silently.
		return fmt.Errorf("demo roster applied with errors: %v", sum.Errors)
	}
	slog.Info("seeded demo accounts",
		"created", sum.Created, "updated", sum.Updated, "enrolled", sum.Enrolled,
		"superusers", sum.Superusers, "skipped", sum.Skipped)
	return nil
}

// seedSchedules loads the lesson-schedules YAML pre-load and upserts the
// lesson_schedules + schedule_sessions rows. It is idempotent and additive:
// rows are keyed by the stable ids (lesson slug, and (schedule, sched_id)) so a
// re-seed updates in place and never deletes sessions a teacher added by hand. A
// schedule whose lesson slug matches no lesson is WARNed and skipped.
func seedSchedules(app core.App, path string) error {
	schedules, err := content.LoadSchedules(path)
	if err != nil {
		return fmt.Errorf("load schedules: %w", err)
	}
	if len(schedules) == 0 {
		return nil
	}
	lsCol, err := app.FindCollectionByNameOrId("lesson_schedules")
	if err != nil {
		return fmt.Errorf("find lesson_schedules collection: %w", err)
	}
	ssCol, err := app.FindCollectionByNameOrId("schedule_sessions")
	if err != nil {
		return fmt.Errorf("find schedule_sessions collection: %w", err)
	}
	var sessions int
	for _, sch := range schedules {
		lesson, _ := app.FindFirstRecordByData("lessons", "slug", sch.Lesson)
		if lesson == nil {
			slog.Warn("lesson schedule skipped: no lesson matches slug", "lesson", sch.Lesson)
			continue
		}
		lsRec, _ := app.FindFirstRecordByData("lesson_schedules", "lesson", lesson.Id)
		if lsRec == nil {
			lsRec = core.NewRecord(lsCol)
		}
		lsRec.Set("lesson", lesson.Id)
		lsRec.Set("allow_unscheduled", sch.AllowUnscheduled)
		if err := app.Save(lsRec); err != nil {
			return fmt.Errorf("save lesson_schedule for %s: %w", sch.Lesson, err)
		}
		for _, s := range sch.Sessions {
			ssRec, _ := app.FindFirstRecordByFilter("schedule_sessions",
				"lesson_schedule = {:ls} && sched_id = {:id}",
				map[string]any{"ls": lsRec.Id, "id": s.ID})
			if ssRec == nil {
				ssRec = core.NewRecord(ssCol)
			}
			ssRec.Set("lesson_schedule", lsRec.Id)
			ssRec.Set("sched_id", s.ID)
			ssRec.Set("name", s.Name)
			setScheduleDate(ssRec, "enroll_from", s.Enroll.From)
			setScheduleDate(ssRec, "enroll_to", s.Enroll.To)
			setScheduleDate(ssRec, "start_from", s.Start.From)
			setScheduleDate(ssRec, "start_to", s.Start.To)
			if err := app.Save(ssRec); err != nil {
				return fmt.Errorf("save schedule_session %s/%s: %w", sch.Lesson, s.ID, err)
			}
			sessions++
		}
		slog.Info("seeded schedule", "lesson", sch.Lesson, "sessions", len(sch.Sessions), "allow_unscheduled", sch.AllowUnscheduled)
	}
	slog.Info("seeded schedules", "lessons", len(schedules), "sessions", sessions)
	return nil
}

// setScheduleDate parses an authored datetime string and sets it on a schedule
// session record. An empty or unparseable value clears the field (open-ended).
func setScheduleDate(rec *core.Record, field, value string) {
	if value == "" {
		rec.Set(field, types.DateTime{})
		return
	}
	dt, err := types.ParseDateTime(value)
	if err != nil {
		slog.Warn("schedule date unparseable; leaving empty", "field", field, "value", value, "err", err)
		rec.Set(field, types.DateTime{})
		return
	}
	rec.Set(field, dt)
}

// ingestContent loads each content root, renders it with the chosen renderer,
// and upserts courses/lessons/slides plus their publishing status. Multiple
// roots let Nabu's shipped help tree (nabu/help) be seeded alongside the
// deployment's authored course content; a root that isn't present is skipped so
// seeding still works where only one tree exists.
// contentRoot is one tree to ingest. Help is set for Nabu's shipped help tree,
// which — unlike an authored course — has a static, guest-viewable route, so its
// cross-reference links render as real URLs (see content.Options.HelpRoot).
type contentRoot struct {
	dir  string
	help bool
}

// assetsDir is the course assets tree; t_mapper data files resolve under its data/.
func ingestContent(app core.App, roots []contentRoot, assetsDir, renderer string, strict bool) error {
	r, err := content.NewRenderer(renderer)
	if err != nil {
		return err
	}
	sink := &fileAssetSink{publicDir: publicDir(app.DataDir())}
	// resolveLms rewrites lms://resources/<path> references in lesson text to the
	// object's public URL at build time. Course resources only for now; group/student
	// refs are viewer-dependent and left for runtime resolution.
	s3 := s3svc.New()
	resolveLms := func(bucket, prefix, ref string) (string, bool) {
		if !s3.Configured() || bucket == "" {
			return "", false
		}
		scope, rel, ok := lmsfs.ParseRef(ref)
		if !ok || scope != lmsfs.ScopeResources {
			return "", false
		}
		return s3.PublicURL(bucket, lmsfs.KeyForRef(prefix, scope, rel)), true
	}
	var probs []content.Problem
	for _, root := range roots {
		if root.dir == "" {
			continue
		}
		if _, err := os.Stat(root.dir); err != nil {
			slog.Warn("content root not found; skipping", "dir", root.dir)
			continue
		}
		courses, err := content.Load(root.dir, r, content.Options{
			Assets:        sink,
			DataDir:       filepath.Join(assetsDir, "data"),
			ResolveLmsURL: resolveLms,
			HelpRoot:      root.help,
		})
		if err != nil {
			return fmt.Errorf("load content %s: %w", root.dir, err)
		}
		probs = append(probs, content.Problems(courses)...)
		for _, c := range courses {
			courseRec, err := upsertCourse(app, c)
			if err != nil {
				return err
			}
			for _, l := range c.Lessons {
				lessonRec, err := upsertLesson(app, courseRec, l)
				if err != nil {
					return err
				}
				if err := rebuildSlides(app, lessonRec, l.Slides); err != nil {
					return err
				}
				if err := rebuildTeacherPages(app, lessonRec, l.TeacherPages); err != nil {
					return err
				}
				slog.Info("seeded lesson", "course", c.Slug, "lesson", l.Slug,
					"slides", len(l.Slides), "teacher_pages", len(l.TeacherPages))
			}
		}
	}
	return reportContentProblems(probs, strict)
}

// reportContentProblems logs every non-OK content node and fails the seed on real
// breakage (ERRORS/CORRUPT) — or on any warning under --strict. This makes `seed`
// fragile by design: bad content barks to the CLI and returns a non-zero exit
// (failing CI/CD) instead of silently seeding a hidden, broken lesson.
func reportContentProblems(probs []content.Problem, strict bool) error {
	var hard, warn int
	for _, p := range probs {
		switch p.Status {
		case content.StatusErrors, content.StatusCorrupt:
			slog.Error("content problem", "path", p.Path, "status", string(p.Status), "reason", p.Issue)
			hard++
		default: // WARNINGS
			slog.Warn("content warning", "path", p.Path, "status", string(p.Status), "reason", p.Issue)
			warn++
		}
	}
	switch {
	case hard > 0:
		return fmt.Errorf("%d content error(s) — fix the frontmatter/widgets logged above and re-seed", hard)
	case strict && warn > 0:
		return fmt.Errorf("%d content warning(s) with --strict set — fix them or drop --strict", warn)
	}
	return nil
}

func upsertCourse(app core.App, c content.Course) (*core.Record, error) {
	rec, _ := app.FindFirstRecordByData("courses", "slug", c.Slug)
	if rec == nil {
		col, err := app.FindCollectionByNameOrId("courses")
		if err != nil {
			return nil, fmt.Errorf("find courses collection: %w", err)
		}
		rec = core.NewRecord(col)
	}
	rec.Set("slug", c.Slug)
	rec.Set("title", c.Title)
	rec.Set("order", c.Order)
	rec.Set("summary", c.Summary)
	rec.Set("number", c.Number)
	rec.Set("guest_access", c.GuestAccess)
	rec.Set("logo", c.Logo)
	rec.Set("css", c.CSS)
	setStatus(rec, c.Readiness, c.Content, c.GoLive)
	if err := app.Save(rec); err != nil {
		return nil, fmt.Errorf("save course %s: %w", c.Slug, err)
	}
	return rec, nil
}

func upsertLesson(app core.App, course *core.Record, l content.Lesson) (*core.Record, error) {
	rec, _ := app.FindFirstRecordByFilter("lessons", "course = {:course} && slug = {:slug}",
		map[string]any{"course": course.Id, "slug": l.Slug})
	if rec == nil {
		col, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return nil, fmt.Errorf("find lessons collection: %w", err)
		}
		rec = core.NewRecord(col)
	}
	rec.Set("course", course.Id)
	rec.Set("slug", l.Slug)
	rec.Set("title", l.Title)
	rec.Set("order", l.Order)
	rec.Set("summary", l.Summary)
	rec.Set("number", l.Number)
	rec.Set("bucket", l.Bucket)
	rec.Set("prefix", l.Prefix)
	rec.Set("audience", l.Audience)
	rec.Set("logo", l.Logo)
	rec.Set("css", l.CSS)
	rec.Set("images_budget", l.ImagesBudget)
	setStatus(rec, l.Readiness, l.Content, l.GoLive)
	if err := app.Save(rec); err != nil {
		return nil, fmt.Errorf("save lesson %s: %w", l.Slug, err)
	}
	return rec, nil
}

// rebuildSlides upserts each slide by (lesson, slug) and deletes any stale slides
// no longer present in the content tree, so partial re-ingests are sensible and
// authored deletions propagate.
func rebuildSlides(app core.App, lesson *core.Record, slides []content.Slide) error {
	existing, err := app.FindRecordsByFilter("slides", "lesson = {:lesson}", "", 0, 0,
		map[string]any{"lesson": lesson.Id})
	if err != nil {
		return fmt.Errorf("find existing slides: %w", err)
	}
	bySlug := map[string]*core.Record{}
	for _, rec := range existing {
		bySlug[rec.GetString("slug")] = rec
	}

	col, err := app.FindCollectionByNameOrId("slides")
	if err != nil {
		return fmt.Errorf("find slides collection: %w", err)
	}
	seen := map[string]bool{}
	for _, s := range slides {
		seen[s.Slug] = true
		rec := bySlug[s.Slug]
		if rec == nil {
			rec = core.NewRecord(col)
		}
		rec.Set("lesson", lesson.Id)
		rec.Set("order", s.Order)
		rec.Set("slug", s.Slug)
		rec.Set("number", s.Number)
		rec.Set("kind", s.Kind)
		rec.Set("audience", s.Audience)
		rec.Set("title", s.Title)
		rec.Set("body", s.HTML)
		rec.Set("agent_key", s.AgentKey)
		rec.Set("sim_output", s.SimHTML)
		rec.Set("t_choice", s.ChoiceGroups)
		rec.Set("t_quiz", s.QuizGroups)
		rec.Set("t_mapper", s.DataMappers)
		rec.Set("t_agent", s.AgentJobs)
		rec.Set("timers", s.Timers)
		rec.Set("notes_label", s.NotesLabel)
		setStatus(rec, s.Readiness, s.Content, s.GoLive)
		if err := app.Save(rec); err != nil {
			// Non-fatal: one unsaveable slide must not abort the whole ingest. Any
			// prior good version is left in place (seen keeps it from being pruned).
			slog.Warn("skipping slide that failed to save", "slug", s.Slug, "err", err)
		}
	}
	// prune slides removed from the tree.
	for slug, rec := range bySlug {
		if !seen[slug] {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("delete stale slide %s: %w", slug, err)
			}
		}
	}
	return nil
}

// rebuildTeacherPages upserts each of a lesson's teacher-only pages by (lesson, key)
// and prunes any page whose key is no longer authored, mirroring rebuildSlides. Teacher
// pages carry the same content fields as slides so they render through the same player.
func rebuildTeacherPages(app core.App, lesson *core.Record, pages []content.TeacherPage) error {
	existing, err := app.FindRecordsByFilter("teacher_pages", "lesson = {:lesson}", "", 0, 0,
		map[string]any{"lesson": lesson.Id})
	if err != nil {
		return fmt.Errorf("find existing teacher pages: %w", err)
	}
	byKey := map[string]*core.Record{}
	for _, rec := range existing {
		byKey[rec.GetString("key")] = rec
	}

	col, err := app.FindCollectionByNameOrId("teacher_pages")
	if err != nil {
		return fmt.Errorf("find teacher_pages collection: %w", err)
	}
	seen := map[string]bool{}
	for _, p := range pages {
		seen[p.Key] = true
		rec := byKey[p.Key]
		if rec == nil {
			rec = core.NewRecord(col)
		}
		rec.Set("lesson", lesson.Id)
		rec.Set("key", p.Key)
		rec.Set("kind", p.Kind)
		rec.Set("audience", p.Audience)
		rec.Set("title", p.Title)
		rec.Set("body", p.HTML)
		rec.Set("agent_key", p.AgentKey)
		rec.Set("sim_output", p.SimHTML)
		rec.Set("t_choice", p.ChoiceGroups)
		rec.Set("t_quiz", p.QuizGroups)
		rec.Set("t_mapper", p.DataMappers)
		rec.Set("t_agent", p.AgentJobs)
		rec.Set("timers", p.Timers)
		rec.Set("notes_label", p.NotesLabel)
		rec.Set("readiness", p.Readiness)
		rec.Set("content", string(p.Content))
		if err := app.Save(rec); err != nil {
			// Non-fatal: one unsaveable page must not abort the whole ingest.
			slog.Warn("skipping teacher page that failed to save", "key", p.Key, "err", err)
		}
	}
	for key, rec := range byKey {
		if !seen[key] {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("delete stale teacher page %s: %w", key, err)
			}
		}
	}
	return nil
}

// setStatus writes the two publishing axes + derived go_live onto a content
// record. A nil go_live clears the field so the item is never live.
func setStatus(rec *core.Record, readiness string, cs content.ContentStatus, goLive *time.Time) {
	rec.Set("readiness", readiness)
	rec.Set("content", string(cs))
	if goLive != nil {
		dt, _ := types.ParseDateTime(*goLive)
		rec.Set("go_live", dt)
	} else {
		rec.Set("go_live", types.DateTime{})
	}
}

// settingsWithConfig returns the default settings overlaid with values sourced
// from the embedded nabu-config.yaml, so those defaults are config-driven:
//   - site_logo            <- logopath
//   - signup_require_email <- signup.require_email
//   - demo_mode            <- demo_mode
//   - classroom_fast_login <- classroom_fast_login
//   - images_enabled       <- ai.stable-diffusion.enabled
//
// Falls back to the built-in defaults if the config can't be read.
func settingsWithConfig() []settingSeed {
	cfg, err := embedfs.LoadConfig()
	if err != nil {
		return defaultSettings
	}
	out := make([]settingSeed, len(defaultSettings))
	copy(out, defaultSettings)
	for i := range out {
		switch {
		case out[i].Key == "app_name" && cfg.Pocketbase.App.Name != "":
			out[i].Value = cfg.Pocketbase.App.Name
		case out[i].Key == "site_logo" && cfg.Logopath != "":
			out[i].Value = cfg.Logopath
		case out[i].Key == "signup_require_email":
			out[i].Value = strconv.FormatBool(cfg.Signup.RequireEmail)
		case out[i].Key == "demo_mode":
			out[i].Value = strconv.FormatBool(cfg.DemoMode)
		case out[i].Key == "classroom_fast_login":
			out[i].Value = strconv.FormatBool(cfg.ClassroomFastLogin)
		case out[i].Key == "images_enabled":
			out[i].Value = strconv.FormatBool(cfg.AI.StableDiffusion.Enabled)
		}
	}
	return out
}

// upsertSettings writes the global config rows, keyed by `key`.
func upsertSettings(app core.App, items []settingSeed) error {
	col, err := app.FindCollectionByNameOrId("app_config")
	if err != nil {
		return fmt.Errorf("find app_config collection: %w", err)
	}
	for _, s := range items {
		rec, _ := app.FindFirstRecordByData("app_config", "key", s.Key)
		if rec == nil {
			rec = core.NewRecord(col)
		}
		rec.Set("key", s.Key)
		rec.Set("value", s.Value)
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("save setting %s: %w", s.Key, err)
		}
	}
	return nil
}

// publicDir resolves pb_public as a sibling of the data dir, matching main.go.
func publicDir(dataDir string) string {
	return filepath.Join(filepath.Dir(dataDir), "pb_public")
}

// fileAssetSink copies page-bundle images into pb_public/<publicRel> and returns
// the URL (/<publicRel>) the SPA serves them from.
type fileAssetSink struct{ publicDir string }

func (s *fileAssetSink) Copy(srcPath, publicRel string) (string, error) {
	dst := filepath.Join(s.publicDir, filepath.FromSlash(publicRel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return "/" + publicRel, nil
}

// extractEmbeddedHelp writes the embedded help course tree (package help) to a
// temporary directory so the path-based content loader can ingest it, and
// returns that directory plus a cleanup func that removes it.
func extractEmbeddedHelp() (string, func(), error) {
	dir, err := os.MkdirTemp("", "nabu-help-")
	if err != nil {
		return "", nil, fmt.Errorf("embedded help: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	err = fs.WalkDir(help.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := help.FS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("extract embedded help: %w", err)
	}
	return dir, cleanup, nil
}
