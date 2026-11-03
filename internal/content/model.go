// Package content loads teacher-authored lessons from an on-disk folder tree
// (the Hugo content model: content/<course>/<lesson>/<slide>.md) and renders each
// slide to HTML for seeding into PocketBase. It replaces the hardcoded Go-struct
// lesson content that used to live in internal/seed.
//
// The tree is 100% Hugo-compatible so the same content renders under either the
// built-in goldmark renderer (the default demo path) or an external hugo build:
//
//   - branch metadata (course, lesson) lives in _index.md
//   - a slide is a single NN-name.md file or a leaf bundle NN-name/index.md
//   - custom fields live under a `params:` map so Hugo partials read them as
//     .Params.*; only Hugo's standard title/slug/weight stay top-level
//
// A slide is the atomic unit of display — one screen a student sees in a lesson.
package content

import "time"

// ContentStatus is the ingester-computed health of a course/lesson/slide. It is
// never authored: it records what happened while parsing/rendering the item.
type ContentStatus string

const (
	StatusOK       ContentStatus = "OK"       // clean
	StatusWarnings ContentStatus = "WARNINGS" // rendered but imperfect (missing image, unknown widget)
	StatusErrors   ContentStatus = "ERRORS"   // a part failed to render
	StatusCorrupt  ContentStatus = "CORRUPT"  // frontmatter/file unparseable
)

// status carries the two orthogonal publishing axes shared by every content
// node. Readiness is authored; Content + GoLive are derived at ingest.
//
//   - Readiness: DRAFT | production | deprecated | a "goes live at" timestamp,
//     kept verbatim for admin display/round-trip.
//   - Content:   the ContentStatus above.
//   - GoLive:    the derived publish gate. Non-nil and in the past => live now.
//     production => a fixed past date; a timestamp => that datetime; DRAFT or
//     deprecated => nil (never live).
type status struct {
	Readiness string
	Content   ContentStatus
	GoLive    *time.Time
	// Issue is a human-readable reason when Content != OK (a YAML error, a widget
	// error, a broken xref…), for surfacing at seed/CI time. Empty when clean.
	Issue string
}

// Course is one cohort a student picks; it owns an ordered set of lessons.
type Course struct {
	Slug        string
	Title       string
	Order       int
	Summary     string
	Number      string // resolved from course_numbering
	GuestAccess bool   // if true, signed-out guests may view (e.g. the help course)
	// Logo overrides the default navbar logo while this course is on screen
	// (authored as params.logo in _index.md). Served by basename. Empty = default.
	Logo string
	// CSS is the resolved public URL of this course's stylesheet (authored as
	// params.css in _index.md, conventionally _course.css). Linked after nabu.css
	// while the course is on screen, so a course re-themes the base tokens without
	// forking them. Empty = no course stylesheet.
	CSS string
	status
	numberingOverride string // per-course course_numbering override, if any
	Lessons           []Lesson
}

// Lesson is one teaching unit inside a course; an ordered set of slides.
type Lesson struct {
	// Source names where the lesson was read from, for logs: a disk path or an
	// embed:// label (see internal/source).
	Source  string
	Slug    string
	Title   string
	Order   int
	Summary string
	Number  string // resolved from lesson_numbering
	// Bucket/Prefix configure the lesson's S3 resource store (see internal/s3svc).
	// Bucket is the object-store bucket; Prefix is the optional lesson-root key
	// prefix under which nabu composes role subpaths (resources/ for teachers,
	// session/group/resource/ for students).
	Bucket string
	Prefix string
	// Audience is the persona this lesson targets (student|teacher|admin|all);
	// slides inherit it unless they set their own.
	Audience string
	// Logo overrides the navbar logo while this lesson is on screen (authored as
	// params.logo in _index.md). Served by basename. Empty = fall back to course/default.
	Logo string
	// CSS is the resolved public URL of this lesson's stylesheet (authored as
	// params.css in _index.md, conventionally _lesson.css). Linked AFTER the course
	// stylesheet, so it overrides it for this lesson only. Empty = course theme only.
	CSS string
	// ImagesBudget is the per-group image quota for this lesson (an image agent's "make an
	// image" mode), authored as params.images_budget in _index.md. Seeded into each
	// group's images_remaining at creation. 0 = unset (submit applies a default).
	ImagesBudget int
	status
	numberingOverride string // per-lesson lesson_numbering override, if any
	Slides            []Slide
	// TeacherPages are teacher-only companion pages for this lesson (briefing/debrief
	// screens a teacher reads while running the lesson), authored as a params.teacher_pages
	// map (key -> file path relative to the lesson dir) in _index.md. They render like a
	// slide but are never part of the student slide sequence and are served staff-only.
	TeacherPages []TeacherPage
}

// TeacherPage is one teacher-only companion page for a lesson. It embeds Slide so it
// reuses every rendered field (HTML, t_choice/t_agent/t_mapper, timers, notes_label);
// Key is the frontmatter map key (e.g. "start", "end") used in its staff-gated URL.
type TeacherPage struct {
	Key string
	Slide
}

// Slide is the atomic unit of display. It mirrors the fields the player needs:
// rendered HTML plus the declarative widget config carried in frontmatter.
type Slide struct {
	Slug   string
	Title  string
	Order  int
	Number string // resolved from slide_numbering
	Kind   string // intro|human|presentation|agent|discussion|reflection
	// Audience is the persona this slide targets; inherited from the lesson if unset.
	Audience string

	HTML    string // rendered body HTML
	SimHTML string // rendered sim_output HTML (kind==agent)

	AgentKey     string
	NotesLabel   string
	ChoiceGroups []ChoiceGroup
	QuizGroups   []QuizGroup
	AgentJobs    []AgentJob
	Timers       []Timer
	DataMappers  []DataMapper

	status
	numberingOverride string // per-slide slide_numbering override, if any
}

// ChoiceGroup is one named set of options declared under a slide's `t_choice`
// frontmatter and placed in the body by an {{< LMS t="choice" id="…" >}} widget.
// Keeping the (bulky) options in frontmatter keeps multi-file diffs small; the
// shortcode only controls where the group renders.
//
// ID is both the placement reference and the storage/carry key: two slides that
// use the same ID share the student's saved pick (this replaces the old
// choice_key mechanism).
type ChoiceGroup struct {
	ID     string `json:"id" yaml:"id"`
	Legend string `json:"legend" yaml:"legend"`
	// Style selects how the options render: "radio" (default) shows the button
	// selector; "dropdown" shows a native <select>. A group re-declared on a
	// later slide with the same ID but a different Style re-shows the shared pick
	// in that style (e.g. pick with radio, re-show as a dropdown).
	Style string `json:"style,omitempty" yaml:"style"`
	// Scope controls where a pick persists and who shares it (REQUIRED):
	//   session — personal to the student's session (stored in work_state)
	//   group   — shared by the session's group (stored in the group's Yjs doc,
	//             the `choices` map synced via ydoc_updates; see internal/httpapi/ydoc.go)
	// lesson|course|nabu|user are reserved but not yet implemented (seed errors).
	Scope string `json:"scope,omitempty" yaml:"scope"`
	// DefaultValueKey names the option field a {{< LMS t="print" >}} shows by
	// default: key|label|slug. Empty falls back to the catalogue default (slug).
	DefaultValueKey string   `json:"default_value_key,omitempty" yaml:"default_value_key"`
	Options         []Choice `json:"options" yaml:"options"`
}

// QuizGroup is one named set of self-check questions declared under a slide's
// `t_quiz` frontmatter and placed in the body by an {{< LMS t="quiz" id="…" >}}
// widget. It deliberately mirrors ChoiceGroup: the bulky config lives in
// frontmatter (small diffs) and the shortcode controls only placement.
//
// ── Designed for a persistence layer that does not exist yet ──────────────────
//
// Today a quiz is answered in the browser and forgotten when the slide changes:
// no score, no storage, no network. The SHAPE, however, is the one a persisted
// quiz needs, so adding persistence later is a code change and not a re-authoring
// of every lesson:
//
//   - ID and each Question.ID are STABLE author-assigned keys. A stored answer is
//     keyed (session, user|group, quiz ID, question ID) — a tuple fully
//     determined by what is authored now.
//   - Scope is honoured now (it decides whether the UI is per-student or shared)
//     and is exactly the axis a group quiz will persist on, reusing the same Yjs
//     shared-document plumbing that t_choice group picks already use.
//   - Correct/Rationale live server-side in the parsed model, so a future scored
//     mode can grade without the client being trusted.
//
// See docs/quiz-persistence.md for the contract, and LmsQuiz.svelte — which is a
// controlled component precisely so swapping local state for a store is a change
// in its parent alone.
type QuizGroup struct {
	ID     string `json:"id" yaml:"id"`
	Legend string `json:"legend,omitempty" yaml:"legend"`
	// Scope controls who answers together, matching t_choice's vocabulary:
	//   student — personal to the student (the default)
	//   group   — shared by the session's group
	Scope string `json:"scope,omitempty" yaml:"scope"`
	// Reveal decides when the answer and rationale are shown:
	//   on-answer (default) — as soon as the student commits an answer
	//   on-request          — behind a "show answer" control
	//   never               — collect only; used when a teacher debriefs live
	Reveal    string     `json:"reveal,omitempty" yaml:"reveal"`
	Questions []Question `json:"questions" yaml:"questions"`
}

// Question is one item in a QuizGroup. Stem is the question text; Options are the
// selectable answers (empty for a `short` free-text question). Rationale explains
// the answer once revealed — the teaching payload, and the reason a self-check is
// worth having even without scoring.
type Question struct {
	ID   string `json:"id" yaml:"id"`
	Type string `json:"type" yaml:"type"` // choice|multi|truefalse|short
	Stem string `json:"stem" yaml:"stem"`
	// Answer is the expected response for a `short` question. Advisory only: free
	// text is not auto-marked, it is shown alongside the student's attempt.
	Answer    string       `json:"answer,omitempty" yaml:"answer"`
	Rationale string       `json:"rationale,omitempty" yaml:"rationale"`
	Options   []QuizOption `json:"options,omitempty" yaml:"options"`
}

// QuizOption is one selectable answer. Correct marks the expected option(s) —
// several may be correct for a `multi` question.
type QuizOption struct {
	Key     string `json:"key" yaml:"key"`
	Label   string `json:"label" yaml:"label"`
	Correct bool   `json:"correct,omitempty" yaml:"correct"`
}

// DataMapper declares a derived value: it indexes a bulk data file by the resolved
// value of a choice (`From`) and exposes the result under `ID` for a
// {{< LMS t="tabulate" from="…" >}} widget. Authored under a slide's `t_mapper`
// frontmatter. Semantics: output = data[Root][value-of(From)]. See
// docs/lesson-choice-values.md.
type DataMapper struct {
	ID      string   `json:"id" yaml:"id"`
	From    string   `json:"from" yaml:"from"`                 // the choice id whose value indexes the data
	Source  string   `json:"source" yaml:"source"`             // data file under assets/data (served as /data/<name>.json)
	Root    string   `json:"root" yaml:"root"`                 // top-level key; the chosen value indexes into it
	Columns []string `json:"columns,omitempty" yaml:"columns"` // optional table columns; default = the rows' keys
}

// Choice is one option in a choice group's selector.
type Choice struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Slug is a URL/token-friendly form of the option, resolved into the stored
	// value so {{< LMS t="print" >}} can render it (e.g. "cat" vs label "Cat 🐱").
	Slug string `json:"slug,omitempty" yaml:"slug"`
}

// AgentJob is one job configuration declared under a slide's `t_agent`
// frontmatter and dispatched by an {{< LMS t="agent" id="…" >}} widget. It
// mirrors ChoiceGroup: the bulky config (agent name, inputs, timeout) lives in
// frontmatter for small diffs, while the shortcode controls only placement.
//
// ID is the shortcode reference, the work_state key for this job on the slide,
// and the job's logical step name. Agent is the worker-advertised tool the job
// runs. Inputs is the payload sent to the worker; values may contain
// {{choice:ID}} / {{field:ID}} tokens the client resolves from work_state at
// dispatch time, so the job carries the student's own answers.
type AgentJob struct {
	ID    string `json:"id" yaml:"id"`
	Agent string `json:"agent" yaml:"agent"`
	Title string `json:"title,omitempty" yaml:"title"`
	// Icon is an optional avatar (typically one emoji) shown beside the agent;
	// the player falls back to a robot when unset.
	Icon    string         `json:"icon,omitempty" yaml:"icon"`
	Timeout string         `json:"timeout,omitempty" yaml:"timeout"`
	Inputs  map[string]any `json:"inputs,omitempty" yaml:"inputs"`
	// Profiles supplies per-variant defaults for a config-driven agent whose
	// concrete worker is chosen at run time — e.g. the controller, dispatched as
	// pip|orca|buzz. The map is keyed by the RESOLVED agent key (the same value the
	// {{choice:…}} in Agent resolves to). Each entry is a small string map whose
	// fields feed two things client-side: a `system` field documenting that
	// variant's system prompt (shown by the `agentdef` widget), and default values
	// for group-editable Inputs keys (e.g. additional_prompts, verification) that
	// apply only until the group edits them. Opaque to the server — only the client
	// can read the group's live choice to pick the active profile, so it sends the
	// resolved value and the server's group-policy overlay accepts it.
	Profiles map[string]map[string]string `json:"profiles,omitempty" yaml:"profiles"`
	// ConfigPolicy declares, per input key, whether the group may change it and
	// within what bounds. A key with no policy is treated as `fixed`. The server
	// is the authority: it starts from the authored Inputs above and overlays only
	// group-editable keys, clamping ranges (see internal/httpapi/jobs.go). See the
	// group collaboration plan and the authoring help for a worked example.
	ConfigPolicy map[string]InputPolicy `json:"config_policy,omitempty" yaml:"config_policy"`
	// SimOutputFallback: when the worker is offline, show the slide's authored
	// sim_output instead of erroring, so the lesson still demonstrates a result.
	SimOutputFallback bool `json:"sim_output_fallback,omitempty" yaml:"sim_output_fallback"`
	// CopyNext: show a "copy this output" button once the job is done, so the
	// student can hand this agent's result to the next agent's `material` box on
	// the following slide. The button carries a throbbing highlight to nudge the
	// copy; set only on slides that feed a downstream agent (e.g. search -> sort -> present).
	CopyNext bool `json:"copy_next,omitempty" yaml:"copy_next"`
	// InheritWorkerPrompts: for a controller job, carry each worker's own
	// group-set additional_prompts (from that agent's slide) into the automated run
	// so it steers the workers the same way a manual run would. Only the client can
	// read the group's per-agent config (it lives in the group's Yjs doc), so it
	// gathers them and sends a `worker_prompts` map; this flag tells the server to
	// trust and forward that map (see effectiveInputs). Off for ordinary agents.
	InheritWorkerPrompts bool `json:"inherit_worker_prompts,omitempty" yaml:"inherit_worker_prompts"`
}

// InputPolicy is the per-input access rule for an agent's config. Policy is one
// of:
//
//	fixed       authored value, sent to the worker, not shown as editable
//	hidden      authored value, sent to the worker, never shown to students
//	group       group-editable, free within the value's type — or, when Options is
//	            set, a closed choice the client renders as a <select>
//	group_range group-editable numeric, clamped to [Min, Max]
//	template    group-editable string containing {{choice:ID}}/{{field:ID}} tokens
//	            that the client resolves from the group's shared choices
//
// Min/Max apply only to group_range and are pointers so "unset" is distinct from
// zero. Options applies only to `group` and, when non-empty, restricts the value to
// one of the listed strings (e.g. orientation: [landscape, portrait]); the server
// keeps the authored default when a client proposes a value outside the set.
//
// Merge selects the collaborative write semantics for a group-editable key:
// "crdt" merges concurrent edits per character (free-form prose — held as a Y.Text
// in the group's shared doc, so two typists don't clobber each other); "atomic"
// (the default when empty) is last-writer-wins on the whole value (file paths,
// numbers, choices — where a character merge would be nonsensical). It is advisory
// to the client's Yjs layer only; the server never inspects it.
type InputPolicy struct {
	Policy  string   `json:"policy" yaml:"policy"`
	Merge   string   `json:"merge,omitempty" yaml:"merge"`
	Min     *float64 `json:"min,omitempty" yaml:"min"`
	Max     *float64 `json:"max,omitempty" yaml:"max"`
	Options []string `json:"options,omitempty" yaml:"options"`
}

// Timer is one countdown declaration in a slide's `timers:` frontmatter.
//
// Two flavours share the type:
//   - a legacy LOCAL segment (Scope/ID empty): page-relative, unshared; several
//     run in order (a 15-min task then a 10-min discussion). Rendered by the
//     client's CountdownTimer.
//   - a SYNCHRONIZED shared timer (Scope set, ID required): one countdown per
//     (cohort, ID) that every student in the session sees identically. The same
//     entry is repeated on every slide that should display it — like a choice id.
//     Scope "teacher" means the teacher controls it (pause/resume/extend/reset).
type Timer struct {
	Label   string `json:"label" yaml:"label"`
	Seconds int    `json:"seconds" yaml:"seconds"`
	// ID is the shared timer's key and cross-slide reference; required when Scope
	// is set. Empty for legacy local segments.
	ID string `json:"id,omitempty" yaml:"id"`
	// Scope selects who shares the timer. "teacher" is the only implemented value
	// (cohort-wide, teacher-controlled). Empty = a legacy local segment.
	Scope string `json:"scope,omitempty" yaml:"scope"`
}
