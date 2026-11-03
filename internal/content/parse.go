package content

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Options controls a Load: the three numbering patterns and where copied assets
// go. Empty numbering patterns fall back to the documented defaults.
type Options struct {
	CourseNumbering string
	LessonNumbering string
	SlideNumbering  string
	Assets          AssetSink
	// DataDir is where t_mapper `source` files live (e.g. "assets/data"). When set,
	// seed-time validation checks each mapper's source file + root key resolve.
	// Empty (e.g. `admin validate`) skips that filesystem check.
	DataDir string
	// ResolveLmsURL, if set, converts an lms:// reference on a lesson (identified by
	// its bucket+prefix) into a served URL, returning ok=false to leave the
	// reference as authored (e.g. storage unconfigured or an unsupported scope).
	// Supplied by the caller so content stays independent of the storage backend —
	// the point of the lms:// scheme. See internal/lmsfs.
	ResolveLmsURL func(bucket, prefix, ref string) (url string, ok bool)
	// HelpRoot marks this tree as Nabu's built-in help (the --help-content root).
	// Help lessons have a static, guest-viewable route (/nabu/help/<course>/<lesson>),
	// so their `help:` cross-references become real URLs. Authored courses have no
	// such route — they are only viewable inside a session — so their references
	// become in-player slide jumps instead. See rewriteXrefLinks.
	HelpRoot bool
}

// Default numbering patterns (see docs/user/config.md).
const (
	DefaultCourseNumbering = "CC"
	DefaultLessonNumbering = "CC.L"
	DefaultSlideNumbering  = "CC.L.S"
)

func (o Options) coursePattern() string { return orDefault(o.CourseNumbering, DefaultCourseNumbering) }
func (o Options) lessonPattern() string { return orDefault(o.LessonNumbering, DefaultLessonNumbering) }
func (o Options) slidePattern() string  { return orDefault(o.SlideNumbering, DefaultSlideNumbering) }

// helpHrefRe matches an in-app help deep link produced by rewriteXrefLinks, so
// validateXrefs can check the target lesson/slide actually exists.
var helpHrefRe = regexp.MustCompile(`href="/nabu/help/([^"/]+)/([^"#]+)(#([^"]*))?"`)

// unresolvedXrefRe matches a `help:` reference rewriteXrefLinks could NOT turn
// into a link — i.e. one authored in a normal course that points outside its own
// lesson. Those have no route to target (an authored lesson is only reachable
// inside a session), so they survive into the HTML as authored and are reported
// here rather than rendered as a dead link.
var unresolvedXrefRe = regexp.MustCompile(`href="help:([^"]+)"`)

// jumpXrefRe matches an in-player slide jump produced by rewriteXrefLinks for an
// authored course, so the target slug can be checked against the lesson.
var jumpXrefRe = regexp.MustCompile(`data-nabu-slide="([^"]*)"`)

// validateXrefs stamps a WARNING on any slide whose rendered `help:` cross-reference
// points at a course/lesson/slide that doesn't exist — so broken links surface in
// /_/ instead of silently 404ing. Runs after the whole tree is loaded.
func validateXrefs(courses []Course) {
	idx := map[string]map[string]map[string]bool{} // course -> lesson -> set(slide slugs)
	for ci := range courses {
		c := &courses[ci]
		lm := map[string]map[string]bool{}
		for li := range c.Lessons {
			l := &c.Lessons[li]
			sm := map[string]bool{}
			for _, s := range l.Slides {
				sm[s.Slug] = true
			}
			lm[l.Slug] = sm
		}
		idx[c.Slug] = lm
	}
	for ci := range courses {
		c := &courses[ci]
		for li := range c.Lessons {
			l := &c.Lessons[li]
			for si := range l.Slides {
				s := &l.Slides[si]
				for _, m := range helpHrefRe.FindAllStringSubmatch(s.HTML, -1) {
					cls, lesson, slide := m[1], m[2], m[4]
					lm, ok := idx[cls]
					if !ok {
						s.Content = worseStatus(s.Content, StatusWarnings)
						s.Issue = appendIssue(s.Issue, "broken xref: no course "+cls)
						continue
					}
					sm, ok := lm[lesson]
					if !ok {
						s.Content = worseStatus(s.Content, StatusWarnings)
						s.Issue = appendIssue(s.Issue, "broken xref: no lesson "+cls+"/"+lesson)
						continue
					}
					if slide != "" && !sm[slide] {
						s.Content = worseStatus(s.Content, StatusWarnings)
						s.Issue = appendIssue(s.Issue, "broken xref: no slide "+lesson+"#"+slide)
					}
				}
				// In-player jumps (authored courses): the target must be a slide of
				// THIS lesson — that is the only thing the run page can jump to.
				for _, m := range jumpXrefRe.FindAllStringSubmatch(s.HTML, -1) {
					if slide := m[1]; slide != "" && !idx[c.Slug][l.Slug][slide] {
						s.Content = worseStatus(s.Content, StatusWarnings)
						s.Issue = appendIssue(s.Issue, "broken xref: no slide "+l.Slug+"#"+slide)
					}
				}
				// References that could not be rewritten at all. Explain why, because
				// the authored text looks correct — the limitation is that an authored
				// lesson has no addressable route outside a session.
				for _, m := range unresolvedXrefRe.FindAllStringSubmatch(s.HTML, -1) {
					s.Content = worseStatus(s.Content, StatusWarnings)
					s.Issue = appendIssue(s.Issue,
						"unresolvable xref help:"+m[1]+
							" — outside the help course, only same-lesson refs (help:"+l.Slug+"#<slide>) can be linked")
				}
			}
		}
	}
}

// audienceAll is the default persona when none is authored (visible to everyone).
const audienceAll = "all"

// validAudiences is the closed set of persona values (see docs/personas + the
// authoring frontmatter reference). Empty means "inherit / all".
var validAudiences = map[string]bool{"student": true, "teacher": true, "admin": true, audienceAll: true}

// validateAudience returns WARNINGS for an unknown non-empty audience so a typo
// (e.g. "teachers") surfaces on the node's content status; empty is fine (inherits).
func validateAudience(a string) (ContentStatus, string) {
	if a == "" || validAudiences[a] {
		return StatusOK, ""
	}
	return StatusWarnings, fmt.Sprintf("unknown audience %q (want student|teacher|admin|all)", a)
}

// appendIssue joins diagnostic messages onto a node's Issue string.
func appendIssue(existing, msg string) string {
	switch {
	case msg == "":
		return existing
	case existing == "":
		return msg
	default:
		return existing + "; " + msg
	}
}

// joinIssues summarises the non-OK widget issues from a render pass.
func joinIssues(issues []lmsIssue) string {
	var msgs []string
	for _, i := range issues {
		if i.Level != StatusOK && i.Msg != "" {
			msgs = append(msgs, i.Msg)
		}
	}
	return strings.Join(msgs, "; ")
}

// resolveAudiences fills each lesson/slide Audience by inheritance: an empty lesson
// audience defaults to "all", and a slide inherits its lesson's audience unless it
// sets its own. Runs after the tree is loaded and sorted (parent in scope).
func resolveAudiences(courses []Course) {
	for ci := range courses {
		for li := range courses[ci].Lessons {
			l := &courses[ci].Lessons[li]
			if l.Audience == "" {
				l.Audience = audienceAll
			}
			for si := range l.Slides {
				s := &l.Slides[si]
				s.Audience = orDefault(s.Audience, l.Audience)
			}
		}
	}
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// frontmatter is the standard-Hugo top-level shape plus our params bag.
type frontmatter struct {
	Title  string `yaml:"title"`
	Slug   string `yaml:"slug"`
	Weight int    `yaml:"weight"`
	Params params `yaml:"params"`
}

// params holds every custom course/lesson/slide field, nested so Hugo partials
// read them as .Params.*.
type params struct {
	// ID overrides the folder-name-derived slug for a course/lesson. Unset =>
	// the directory name is used (the historical behaviour). Set it to pin a
	// stable identifier that survives folder renames and disambiguates parallel
	// course runs where two folders would otherwise share a slug.
	ID              string        `yaml:"id"`
	Readiness       string        `yaml:"readiness"`
	Kind            string        `yaml:"kind"`
	Summary         string        `yaml:"summary"`
	GuestAccess     bool          `yaml:"guest_access"`
	AgentKey        string        `yaml:"agent_key"`
	NotesLabel      string        `yaml:"notes_label"`
	SimOutput       string        `yaml:"sim_output"`
	ChoiceGroups    []ChoiceGroup `yaml:"t_choice"`
	QuizGroups      []QuizGroup   `yaml:"t_quiz"`
	AgentJobs       []AgentJob    `yaml:"t_agent"`
	DataMappers     []DataMapper  `yaml:"t_mapper"`
	Timers          []Timer       `yaml:"timers"`
	CourseNumbering string        `yaml:"course_numbering"`
	// LegacyClassNumbering is the deprecated pre-rename `class_numbering` key,
	// used only when course_numbering is unset.
	LegacyClassNumbering string `yaml:"class_numbering"`
	LessonNumbering      string `yaml:"lesson_numbering"`
	SlideNumbering       string `yaml:"slide_numbering"`
	// Bucket/Prefix configure a lesson's S3 resource store (lesson-level only).
	Bucket string `yaml:"bucket"`
	Prefix string `yaml:"prefix"`
	// Logo overrides the navbar logo while this course/lesson is on screen. Served
	// by basename (like the app_config logo settings).
	Logo string `yaml:"logo"`
	// CSS names a stylesheet in this course/lesson's bundle (conventionally
	// _course.css / _lesson.css). Copied into the public tree like a logo and linked
	// after nabu.css — course first, then lesson — so a course re-themes the base
	// tokens and a lesson may override its course. Course/lesson level only.
	CSS string `yaml:"css"`
	// Audience names the persona a lesson/slide targets: student|teacher|admin|all.
	// Hierarchical: a slide inherits its lesson's audience unless it sets its own.
	Audience string `yaml:"audience"`
	// ImagesBudget is the per-group image quota for this lesson (an image agent's "make an
	// image" mode). Seeded into each group's images_remaining at creation; 0 means
	// unset (the submit path applies a default). Lesson-level only.
	ImagesBudget int `yaml:"images_budget"`
	// TeacherPages maps a page key (e.g. "start", "end") to a Markdown file path,
	// relative to the lesson dir, holding a teacher-only companion page. Lesson-level.
	TeacherPages map[string]string `yaml:"teacher_pages"`
}

var (
	fmSplitRe  = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n?(.*)\z`)
	filePrefix = regexp.MustCompile(`^(\d+)-(.*)$`)
	firstH1Re  = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
)

// Load walks a Hugo-style content tree and returns the courses it contains,
// each with rendered slides and resolved numbering + publishing status. It is
// non-fatal per node: a file that cannot be parsed is recorded with
// ContentStatus CORRUPT rather than aborting the whole load.
func Load(dir string, r Renderer, opts Options) ([]Course, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read content dir %s: %w", dir, err)
	}
	var courses []Course
	for _, e := range entries {
		if !e.IsDir() || IsControlFile(e.Name()) {
			continue
		}
		c, err := loadCourse(filepath.Join(dir, e.Name()), e.Name(), r, opts)
		if err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	sort.SliceStable(courses, func(i, j int) bool { return courses[i].Order < courses[j].Order })
	assignNumbers(courses, opts)
	resolveAudiences(courses)
	validateXrefs(courses)
	return courses, nil
}

// Problem is one course/lesson/slide whose content status isn't OK, with its path
// and reason — for the seeder / CI to report and fail on.
type Problem struct {
	Path   string // course[/lesson[/slide]]
	Status ContentStatus
	Issue  string
}

// Problems collects every node whose content status is not OK (WARNINGS, ERRORS,
// or CORRUPT) so callers can print them and decide whether to fail.
func Problems(courses []Course) []Problem {
	var out []Problem
	rec := func(path string, st status) {
		if st.Content != StatusOK {
			out = append(out, Problem{Path: path, Status: st.Content, Issue: st.Issue})
		}
	}
	for _, c := range courses {
		rec(c.Slug, c.status)
		for _, l := range c.Lessons {
			rec(c.Slug+"/"+l.Slug, l.status)
			for _, s := range l.Slides {
				rec(c.Slug+"/"+l.Slug+"/"+s.Slug, s.status)
			}
			for _, tp := range l.TeacherPages {
				rec(c.Slug+"/"+l.Slug+"/teacher:"+tp.Key, tp.status)
			}
		}
	}
	return out
}

func loadCourse(dir, slug string, r Renderer, opts Options) (Course, error) {
	c := Course{Slug: slug, Title: slug}
	fm, status, issue := parseIndex(filepath.Join(dir, "_index.md"))
	c.Content = status
	c.Issue = issue
	if status != StatusCorrupt {
		if fm.Title != "" {
			c.Title = fm.Title
		}
		// params.id overrides the folder-name slug (see params.ID). Filesystem
		// reads use `dir`, so only the logical identifier changes.
		if fm.Params.ID != "" {
			c.Slug = fm.Params.ID
		}
		c.Order = fm.Weight
		c.Summary = fm.Params.Summary
		c.GuestAccess = fm.Params.GuestAccess
		c.Logo = resolveBundleFile(dir, path.Join("content", c.Slug), fm.Params.Logo, opts)
		c.CSS = resolveBundleFile(dir, path.Join("content", c.Slug), fm.Params.CSS, opts)
		c.numberingOverride = fm.Params.CourseNumbering
		if c.numberingOverride == "" {
			c.numberingOverride = fm.Params.LegacyClassNumbering
		}
	}
	c.Readiness, c.GoLive = resolveReadiness(fm.Params.Readiness)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return c, fmt.Errorf("read course dir %s: %w", dir, err)
	}
	for _, e := range entries {
		// Directories only, and never a control directory: a "_"-prefixed folder
		// beside the lessons (_original-art, _drafts…) holds working material, not
		// a lesson. See IsControlFile.
		if !e.IsDir() || IsControlFile(e.Name()) {
			continue
		}
		l, err := loadLesson(filepath.Join(dir, e.Name()), c.Slug, e.Name(), r, opts)
		if err != nil {
			return c, err
		}
		c.Lessons = append(c.Lessons, l)
	}
	sort.SliceStable(c.Lessons, func(i, j int) bool { return c.Lessons[i].Order < c.Lessons[j].Order })
	return c, nil
}

func loadLesson(dir, courseSlug, slug string, r Renderer, opts Options) (Lesson, error) {
	l := Lesson{Slug: slug, Title: slug}
	fm, status, issue := parseIndex(filepath.Join(dir, "_index.md"))
	l.Content = status
	l.Issue = issue
	if status != StatusCorrupt {
		if fm.Title != "" {
			l.Title = fm.Title
		}
		// params.id overrides the folder-name slug (see params.ID). Filesystem
		// reads use `dir`, so only the logical identifier changes.
		if fm.Params.ID != "" {
			l.Slug = fm.Params.ID
		}
		l.Order = fm.Weight
		l.Summary = fm.Params.Summary
		l.numberingOverride = fm.Params.LessonNumbering
		l.Bucket = fm.Params.Bucket
		l.Prefix = fm.Params.Prefix
		l.Logo = resolveBundleFile(dir, path.Join("content", courseSlug, l.Slug), fm.Params.Logo, opts)
		l.CSS = resolveBundleFile(dir, path.Join("content", courseSlug, l.Slug), fm.Params.CSS, opts)
		l.Audience = fm.Params.Audience
		l.ImagesBudget = fm.Params.ImagesBudget
		aSt, aMsg := validateAudience(l.Audience)
		l.Content = worseStatus(l.Content, aSt)
		l.Issue = appendIssue(l.Issue, aMsg)
	}
	l.Readiness, l.GoLive = resolveReadiness(fm.Params.Readiness)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return l, fmt.Errorf("read lesson dir %s: %w", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		// Control files and control directories are never slides — that covers
		// _index.md (this lesson's own metadata), _references.md and friends
		// (includable partials), and any _drafts/ subtree. See IsControlFile.
		case IsControlFile(name):
			continue
		case e.IsDir():
			// leaf bundle: <slide>/index.md
			idx := filepath.Join(dir, name, "index.md")
			if _, err := os.Stat(idx); err != nil {
				continue
			}
			l.Slides = append(l.Slides, loadSlide(idx, filepath.Join(dir, name), dir, courseSlug, l.Slug, name, l.Bucket, l.Prefix, r, opts))
		case strings.HasSuffix(name, ".md"):
			base := strings.TrimSuffix(name, ".md")
			l.Slides = append(l.Slides, loadSlide(filepath.Join(dir, name), dir, dir, courseSlug, l.Slug, base, l.Bucket, l.Prefix, r, opts))
		}
	}
	sort.SliceStable(l.Slides, func(i, j int) bool { return l.Slides[i].Order < l.Slides[j].Order })
	resolveChoiceScopes(&l) // settle each choice id's scope lesson-wide (re-shows inherit)
	l.TeacherPages = loadTeacherPages(dir, courseSlug, l.Slug, l.Bucket, l.Prefix, fm.Params.TeacherPages, r, opts)
	return l, nil
}

// loadTeacherPages renders each authored teacher_pages entry (key -> file path relative
// to the lesson dir) as a Slide, keyed by its map key. Keys are visited in sorted order
// so a re-seed is deterministic. A missing/unparseable file yields a CORRUPT page (via
// loadSlide) so the problem surfaces at seed rather than vanishing.
func loadTeacherPages(lessonDir, courseSlug, lessonSlug, bucket, prefix string, pages map[string]string, r Renderer, opts Options) []TeacherPage {
	if len(pages) == 0 {
		return nil
	}
	keys := make([]string, 0, len(pages))
	for k := range pages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]TeacherPage, 0, len(keys))
	for _, key := range keys {
		mdPath := filepath.Join(lessonDir, filepath.FromSlash(pages[key]))
		s := loadSlide(mdPath, filepath.Dir(mdPath), lessonDir, courseSlug, lessonSlug, key, bucket, prefix, r, opts)
		out = append(out, TeacherPage{Key: key, Slide: s})
	}
	return out
}

// loadSlide parses and renders one slide. bundleDir resolves relative images;
// lessonDir is the owning lesson's folder, which is where shared partials live
// (an {{< LMS t="include" >}} falls back to it, so every slide in a lesson can
// reach one _references.md — see expandIncludes); nameBase is the
// filename/dirname (with any NN- ordering prefix); bucket/prefix are the parent
// lesson's resource store, used to resolve lms:// references.
//
// bundleDir and lessonDir are the same folder for a flat slide, and differ for a
// leaf bundle (lesson/<slide>/index.md) and for a teacher page (lesson/teacher/).
func loadSlide(mdPath, bundleDir, lessonDir, courseSlug, lessonSlug, nameBase, bucket, prefix string, r Renderer, opts Options) Slide {
	order, defSlug := splitPrefix(nameBase)
	// default readiness DRAFT so even a CORRUPT slide (which returns early, before
	// readiness is parsed) is recorded as an unpublished draft, not blank.
	s := Slide{Slug: defSlug, Order: order, Title: defSlug,
		status: status{Content: StatusOK, Readiness: "DRAFT"}}

	raw, err := os.ReadFile(mdPath)
	if err != nil {
		s.Content = StatusCorrupt
		return s
	}
	fm, body, ferr := parseFrontmatter(raw)
	if ferr != nil {
		s.Content = StatusCorrupt
		s.Issue = "frontmatter: " + ferr.Error()
		return s
	}

	if fm.Weight != 0 {
		s.Order = fm.Weight
	}
	if fm.Slug != "" {
		s.Slug = fm.Slug
	}
	s.Kind = fm.Params.Kind
	s.Audience = fm.Params.Audience
	s.AgentKey = fm.Params.AgentKey
	s.NotesLabel = fm.Params.NotesLabel
	s.ChoiceGroups = fm.Params.ChoiceGroups
	s.QuizGroups = fm.Params.QuizGroups
	s.AgentJobs = fm.Params.AgentJobs
	s.DataMappers = fm.Params.DataMappers
	s.Timers = fm.Params.Timers
	s.numberingOverride = fm.Params.SlideNumbering
	s.Readiness, s.GoLive = resolveReadiness(fm.Params.Readiness)
	s.Title = pickTitle(fm.Title, body, s.Slug)

	ctx := RenderContext{
		BundleDir:  bundleDir,
		LessonDir:  lessonDir,
		PublicRel:  path.Join("content", courseSlug, lessonSlug, s.Slug),
		Assets:     opts.Assets,
		ChoiceIDs:  choiceIDs(s.ChoiceGroups),
		QuizIDs:    quizIDs(s.QuizGroups),
		CourseSlug: courseSlug,
		LessonSlug: lessonSlug,
		HelpRoot:   opts.HelpRoot,
	}
	// Bind lms:// resolution to this slide's lesson bucket/prefix (a lesson with no
	// bucket can't resolve, so leave refs as authored).
	if opts.ResolveLmsURL != nil && bucket != "" {
		ctx.LmsResolve = func(ref string) (string, bool) { return opts.ResolveLmsURL(bucket, prefix, ref) }
	}

	aSt, aMsg := validateAudience(s.Audience)
	worst := worseStatus(normalizeChoiceStyles(s.ChoiceGroups), aSt)
	s.Issue = appendIssue(s.Issue, aMsg)
	cSt, cMsg := validateChoiceGroups(s.ChoiceGroups)
	worst = worseStatus(worst, cSt)
	s.Issue = appendIssue(s.Issue, cMsg)
	// Validate the quiz BEFORE normalising, so a bad scope/reveal is reported
	// rather than silently replaced by its default.
	qSt, qMsg := validateQuizGroups(s.QuizGroups)
	worst = worseStatus(worst, qSt)
	s.Issue = appendIssue(s.Issue, qMsg)
	normalizeQuizGroups(s.QuizGroups)
	mSt, mMsg := validateDataMappers(s.DataMappers, opts.DataDir)
	worst = worseStatus(worst, mSt)
	s.Issue = appendIssue(s.Issue, mMsg)
	tSt, tMsg := validateTimers(s.Timers)
	worst = worseStatus(worst, tSt)
	s.Issue = appendIssue(s.Issue, tMsg)
	html, issues, rerr := r.Render(body, ctx)
	if rerr != nil {
		s.Content = StatusErrors
		s.Issue = appendIssue(s.Issue, "render: "+rerr.Error())
		return s
	}
	s.HTML = html
	worst = worseStatus(worst, worstOf(issues))
	s.Issue = appendIssue(s.Issue, joinIssues(issues))

	if strings.TrimSpace(fm.Params.SimOutput) != "" {
		simHTML, simIssues, serr := r.Render(fm.Params.SimOutput, ctx)
		if serr != nil {
			worst = StatusErrors
			s.Issue = appendIssue(s.Issue, "sim_output render: "+serr.Error())
		} else {
			s.SimHTML = simHTML
			worst = worseStatus(worst, worstOf(simIssues))
			s.Issue = appendIssue(s.Issue, joinIssues(simIssues))
		}
	}
	s.Content = worst
	return s
}

// assignNumbers resolves the display Number for every node once the tree is
// sorted, so ordinals reflect final order. A per-node numberingOverride wins
// over the global pattern.
func assignNumbers(courses []Course, opts Options) {
	for ci := range courses {
		c := &courses[ci]
		cInfo := levelInfo{Number: ci + 1, Slug: c.Slug, Title: c.Title}
		c.Number = resolveNumbering(orDefault(c.numberingOverride, opts.coursePattern()),
			numberingCtx{Course: cInfo})
		for li := range c.Lessons {
			l := &c.Lessons[li]
			lInfo := levelInfo{Number: li + 1, Slug: l.Slug, Title: l.Title}
			l.Number = resolveNumbering(orDefault(l.numberingOverride, opts.lessonPattern()),
				numberingCtx{Course: cInfo, Lesson: lInfo})
			for si := range l.Slides {
				s := &l.Slides[si]
				sInfo := levelInfo{Number: si + 1, Slug: s.Slug, Title: s.Title}
				s.Number = resolveNumbering(orDefault(s.numberingOverride, opts.slidePattern()),
					numberingCtx{Course: cInfo, Lesson: lInfo, Slide: sInfo})
			}
		}
	}
}

// parseIndex reads a branch _index.md, tolerating a missing file (returns zero
// frontmatter + OK) and reporting CORRUPT for an unparseable one.
func parseIndex(mdPath string) (frontmatter, ContentStatus, string) {
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return frontmatter{}, StatusOK, "" // no _index.md is fine; defaults apply
	}
	fm, _, ferr := parseFrontmatter(raw)
	if ferr != nil {
		return frontmatter{}, StatusCorrupt, "frontmatter: " + ferr.Error()
	}
	return fm, StatusOK, ""
}

// parseFrontmatter splits `---\n…\n---\n` YAML frontmatter from the body and
// unmarshals it. ok is false only if a frontmatter block is present but invalid.
func parseFrontmatter(raw []byte) (frontmatter, string, error) {
	m := fmSplitRe.FindSubmatch(raw)
	if m == nil {
		return frontmatter{}, string(raw), nil // no frontmatter block; body-only
	}
	var fm frontmatter
	if err := yaml.Unmarshal(m[1], &fm); err != nil {
		return frontmatter{}, "", err
	}
	return fm, string(m[2]), nil
}

// splitPrefix separates a leading NN- ordering prefix from the slug portion of a
// filename/dirname base. "10-human-newsroom" -> (10, "human-newsroom").
func splitPrefix(base string) (order int, slug string) {
	if m := filePrefix.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, m[2]
	}
	return 0, base
}

// pickTitle resolves a slide title: frontmatter title, else the first H1 in the
// body, else the slug.
func pickTitle(fmTitle, body, slug string) string {
	if fmTitle != "" {
		return fmTitle
	}
	if m := firstH1Re.FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(m[1])
	}
	return slug
}

// normalizeChoiceStyles defaults each group's Style to "radio" and downgrades an
// unrecognized value to "radio" so a typo never breaks rendering. It mutates the
// groups in place and reports WARNINGS if any value had to be corrected.
func normalizeChoiceStyles(gs []ChoiceGroup) ContentStatus {
	s := StatusOK
	for i := range gs {
		switch gs[i].Style {
		case "", "radio":
			gs[i].Style = "radio"
		case "dropdown":
			// valid
		default:
			gs[i].Style = "radio"
			s = worseStatus(s, StatusWarnings)
		}
	}
	return s
}

// choiceValueFields are the option fields a print widget can show / a group's
// default_value_key may name.
var choiceValueFields = map[string]bool{"key": true, "label": true, "slug": true}

// reservedPrintKeys are the built-in ids a {{< LMS t="print" from="…" >}} resolves
// (username, email, group, session). A t_choice using one of these ids shadows the
// built-in, so it is flagged (a warning, not fatal — the choice still works).
var reservedPrintKeys = map[string]bool{"username": true, "email": true, "group": true, "session": true}

// validateChoiceGroups checks each t_choice group's default_value_key names a real
// option field (key|label|slug) and warns when its id shadows a reserved print
// built-in. Scope is validated lesson-wide (see resolveChoiceScopes) because it is
// a property of the choice id, not the slide.
func validateChoiceGroups(gs []ChoiceGroup) (ContentStatus, string) {
	dvkDefault := catalogChoiceDefaultKey()
	worst := StatusOK
	var msg string
	for _, g := range gs {
		if reservedPrintKeys[g.ID] {
			worst = worseStatus(worst, StatusWarnings)
			msg = appendIssue(msg, fmt.Sprintf("t_choice id %q shadows a built-in print value (username|email|group|session); pick another id", g.ID))
		}
		dvk := g.DefaultValueKey
		if dvk == "" {
			dvk = dvkDefault
		}
		if !choiceValueFields[dvk] {
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("t_choice %q: default_value_key %q must be key|label|slug", g.ID, dvk))
		}
	}
	return worst, msg
}

// resolveChoiceScopes settles each choice id's scope across the whole lesson and
// stamps it onto every declaration of that id, so re-shows inherit the scope of
// the defining declaration (the one with options) and the client always sees it.
// A *definition* (a group with options) must resolve to an implemented scope
// (session|group); reference stubs (no options) may inherit or stay unset.
// lesson|course|nabu|user are reserved and error loudly. See docs/lesson-choice-values.md.
func resolveChoiceScopes(l *Lesson) {
	scopeByID := map[string]string{}
	conflict := map[string]bool{}
	for si := range l.Slides {
		for _, g := range l.Slides[si].ChoiceGroups {
			if g.Scope == "" {
				continue
			}
			if prev, ok := scopeByID[g.ID]; ok && prev != g.Scope {
				conflict[g.ID] = true
			}
			scopeByID[g.ID] = g.Scope
		}
	}
	for si := range l.Slides {
		s := &l.Slides[si]
		worst := StatusOK
		var msg string
		for gi := range s.ChoiceGroups {
			g := &s.ChoiceGroups[gi]
			resolved := scopeByID[g.ID]
			g.Scope = resolved // stamp (may be "") so every declaration carries it
			if conflict[g.ID] {
				worst = worseStatus(worst, StatusErrors)
				msg = appendIssue(msg, fmt.Sprintf("t_choice %q: conflicting scope across slides", g.ID))
			}
			switch resolved {
			case "session", "group":
				// implemented
			case "":
				if len(g.Options) > 0 { // a definition must declare a scope
					worst = worseStatus(worst, StatusErrors)
					msg = appendIssue(msg, fmt.Sprintf("t_choice %q: scope is required (session|group)", g.ID))
				}
			case "lesson", "course", "class", "nabu", "user": // "class": deprecated spelling of course
				worst = worseStatus(worst, StatusErrors)
				msg = appendIssue(msg, fmt.Sprintf("t_choice %q: scope %q not yet implemented (use session or group)", g.ID, resolved))
			default:
				worst = worseStatus(worst, StatusErrors)
				msg = appendIssue(msg, fmt.Sprintf("t_choice %q: unknown scope %q", g.ID, resolved))
			}
		}
		s.Content = worseStatus(s.Content, worst)
		s.Issue = appendIssue(s.Issue, msg)
	}
}

// validateDataMappers checks each t_mapper is structurally sound (id/from/source/
// root present), doesn't shadow a reserved print key, and — when dataDir is set
// (seed) — that its source file exists and its root key resolves. A bad source or
// root is an ERROR (author bug worth failing the seed on).
func validateDataMappers(ms []DataMapper, dataDir string) (ContentStatus, string) {
	worst := StatusOK
	var msg string
	for _, m := range ms {
		if m.ID == "" || m.From == "" || m.Source == "" || m.Root == "" {
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("t_mapper %q: id, from, source and root are all required", m.ID))
			continue
		}
		if reservedPrintKeys[m.ID] {
			worst = worseStatus(worst, StatusWarnings)
			msg = appendIssue(msg, fmt.Sprintf("t_mapper id %q shadows a built-in print value (username|email|group|session); pick another id", m.ID))
		}
		if dataDir == "" {
			continue // no filesystem check (e.g. admin validate)
		}
		raw, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(m.Source)))
		if err != nil {
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("t_mapper %q: source %q not found under the data dir", m.ID, m.Source))
			continue
		}
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("t_mapper %q: source %q is not a YAML map", m.ID, m.Source))
			continue
		}
		if _, ok := doc[m.Root]; !ok {
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("t_mapper %q: root key %q not in %q", m.ID, m.Root, m.Source))
		}
	}
	return worst, msg
}

// validateTimers checks each `timers:` entry. A scoped (shared) timer must name
// an implemented scope and carry an id and a positive duration, since the id is
// the cohort-wide key the first student creates the row under. Legacy local
// segments (no scope, no id) are unconstrained here — they just run in order.
func validateTimers(ts []Timer) (ContentStatus, string) {
	worst := StatusOK
	var msg string
	for _, t := range ts {
		if t.Scope == "" && t.ID == "" {
			continue // legacy local segment
		}
		switch t.Scope {
		case "teacher":
			// implemented: cohort-wide, teacher-controlled
		case "":
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("timer %q: scope is required when an id is set (use teacher)", t.ID))
		default:
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("timer %q: scope %q not yet implemented (use teacher)", t.ID, t.Scope))
		}
		if t.Scope != "" && t.ID == "" {
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, "timer: id is required for a scoped timer")
		}
		if t.Scope == "teacher" && t.Seconds <= 0 {
			worst = worseStatus(worst, StatusErrors)
			msg = appendIssue(msg, fmt.Sprintf("timer %q: seconds must be a positive duration", t.ID))
		}
	}
	return worst, msg
}

// choiceIDs collects the declared t_choice ids so the renderer can validate that
// every {{< LMS t="choice" id="…" >}} points at a group that actually exists.
func choiceIDs(gs []ChoiceGroup) []string {
	ids := make([]string, 0, len(gs))
	for _, g := range gs {
		ids = append(ids, g.ID)
	}
	return ids
}

// worstOf reduces render issues to the most severe ContentStatus (OK if none).
func worstOf(issues []lmsIssue) ContentStatus {
	s := StatusOK
	for _, i := range issues {
		s = worseStatus(s, i.Level)
	}
	return s
}

// worseStatus returns the more severe of two statuses (OK<WARNINGS<ERRORS<CORRUPT).
func worseStatus(a, b ContentStatus) ContentStatus {
	if rank(b) > rank(a) {
		return b
	}
	return a
}

func rank(s ContentStatus) int {
	switch s {
	case StatusWarnings:
		return 1
	case StatusErrors:
		return 2
	case StatusCorrupt:
		return 3
	default:
		return 0
	}
}
