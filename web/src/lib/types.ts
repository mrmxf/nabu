// Shapes mirror the PocketBase collections defined in
// nabu/migrations. Only the fields the UI uses are typed.

export type SlideKind =
	| 'intro'
	| 'human'
	| 'presentation'
	| 'agent'
	| 'discussion'
	| 'reflection';

export interface CourseRec {
	id: string;
	slug: string;
	title: string;
	order: number;
	summary: string;
	// true when signed-out guests may view this course (e.g. the help course).
	guest_access: boolean;
	// optional navbar logo override while this course is on screen (params.logo).
	logo?: string;
	// optional stylesheet for this course's slide bodies (params.css, conventionally
	// _course.css), as a resolved URL. Linked after nabu.css so the course re-themes
	// the base tokens; a lesson's css layers on top of this one.
	css?: string;
}

export interface LessonRec {
	id: string;
	course: string;
	slug: string;
	title: string;
	order: number;
	summary: string;
	// S3 resource store for this lesson (authored in _index.md frontmatter).
	bucket: string;
	prefix: string;
	// persona this lesson targets: student | teacher | admin | all.
	audience: string;
	// optional navbar logo override while this lesson is on screen (params.logo).
	logo?: string;
	// optional stylesheet for this lesson's slide bodies (params.css, conventionally
	// _lesson.css), as a resolved URL. Linked AFTER the course stylesheet, so it
	// overrides the course theme for this lesson only.
	css?: string;
	// expanded relation when requested via ?expand=course.
	expand?: { course?: CourseRec };
}

export interface Choice {
	key: string;
	label: string;
	// URL/token-friendly form, resolved into the stored value so `print` can show it.
	slug?: string;
}

// ResolvedValue is what a pick stores: the chosen option resolved to its fields
// plus the scope it was saved under. Session-scoped values live in work_state;
// group-scoped values live in the group's Yjs doc (the `choices` map).
export interface ResolvedValue {
	key: string;
	label: string;
	slug: string;
	scope: string;
}

// DataMapper declares a derived value: index a bulk data file by the resolved
// value of a choice (`from`) and expose it under `id` for a `tabulate` widget.
// Authored under a slide's `t_mapper` frontmatter. See docs/lesson-choice-values.md.
export interface DataMapper {
	id: string;
	from: string;
	source: string;
	root: string;
	columns?: string[];
}

// A named set of options declared under a slide's `t_choice` frontmatter and
// placed in the body by an {{< LMS t="choice" id="…" >}} widget. `id` is both the
// placement reference and the storage key: same id across slides => shared pick.
export interface ChoiceGroup {
	id: string;
	legend?: string;
	// how the options render: 'radio' (default) is the button selector, 'dropdown'
	// is a native <select>. Re-declaring a group with the same id but a different
	// style re-shows the shared pick in that style.
	style?: 'radio' | 'dropdown';
	// where a pick persists / who shares it: 'session' (personal, work_state) or
	// 'group' (shared, the group's Yjs `choices` map). Resolved lesson-wide at seed.
	scope?: string;
	// which option field a {{< LMS t="print" >}} shows by default (key|label|slug).
	default_value_key?: string;
	options: Choice[];
}

// One entry in a slide's `timers:` frontmatter. Two flavours share the type:
//   - a legacy LOCAL segment (no scope/id): page-relative, unshared; several run
//     in order, driven by CountdownTimer.
//   - a SYNCHRONIZED shared timer (scope set, id required): one cohort-wide
//     countdown per id that every student sees identically, driven by SyncedTimer.
//     scope 'teacher' means the teacher controls pause/resume/extend/reset.
export interface TimerSegment {
	label: string;
	seconds: number;
	id?: string;
	scope?: string;
}

// A synchronized timer row (session_timers): one shared countdown per
// (schedule_session, timer_id). The teacher writes it (staff UpdateRule); every
// student in the cohort reads it and subscribes for realtime updates.
// `end_time` is the absolute deadline (authoritative while running);
// `remaining_ms` is the frozen remaining time while paused.
export interface SessionTimerRec {
	id: string;
	session: string;
	timer_id: string;
	label: string;
	duration_seconds: number;
	running: boolean;
	end_time: string; // ISO datetime ('' if never set)
	remaining_ms: number;
	expand?: { session?: ScheduleSessionRec };
}

// One agent-job config declared under a slide's `t_agent` frontmatter and
// dispatched by an inline {{< LMS t="agent" id="…" >}} widget. Mirrors
// ChoiceGroup: bulky config in frontmatter, placement by id. `id` is also the
// work_state key for this job on the slide. `inputs` values may carry
// {{choice:ID}} / {{field:ID}} tokens resolved from work_state at dispatch.
// InputPolicy declares, per agent-config input, whether the group may change it
// and within what bounds. A key with no policy is treated as 'fixed'. The server
// re-enforces this at submit (see nabu/internal/httpapi/agentcfg.go); the widget
// uses it to decide which controls to render editable.
//   fixed       authored value, shown read-only
//   hidden      authored value, not shown at all
//   group       group-editable, free within type (bool/string); when `options` is
//               set it's a closed choice, rendered as a <select>
//   group_range group-editable number, clamped to [min, max]
//   template    group-editable string with {{choice:ID}} tokens (resolved for display)
export interface InputPolicy {
	policy: 'fixed' | 'hidden' | 'group' | 'group_range' | 'template';
	// merge selects the collaborative write mode for a group-editable key:
	//   crdt    free-form prose, merged per-character in the shared doc (a Y.Text)
	//           so two typists don't clobber each other (additional_prompts, material,
	//           verification)
	//   atomic  (default) last-writer-wins on the whole value — the right choice for
	//           file paths, numbers and closed choices, where a char merge is garbage
	merge?: 'crdt' | 'atomic';
	min?: number;
	max?: number;
	options?: string[];
}

export interface AgentJob {
	id: string;
	agent: string;
	title?: string;
	// optional avatar (one emoji) authored on the job; the player shows a robot when unset.
	icon?: string;
	timeout?: string;
	inputs?: Record<string, unknown>;
	// per-variant profiles for a config-driven agent (e.g. the controller,
	// dispatched as pip|orca|buzz), keyed by the resolved agent key. Each entry
	// supplies a `system` field (documented by the agentdef widget) plus default
	// values for group-editable inputs, applied only until the group edits them.
	profiles?: Record<string, Record<string, string>>;
	config_policy?: Record<string, InputPolicy>;
	sim_output_fallback?: boolean;
	// show a "copy this output" button (with a throbbing highlight) once the job
	// is done, so the student can paste this result into the next agent's input.
	copy_next?: boolean;
	// controller only: gather each worker's own group-set additional_prompts (from
	// that agent's slide) and send them as `worker_prompts` so an automated run
	// steers the workers the same way a manual run would. See LmsAgent.run and the
	// server's effectiveInputs.
	inherit_worker_prompts?: boolean;
}

// A named set of self-check questions, authored under a slide's `t_quiz`
// frontmatter and placed in the body by {{< LMS t="quiz" id="…" >}}.
//
// `id` and each question's `id` are stable author-assigned keys. They are what a
// future persistence layer will key stored answers on — (session,
// student-or-group, quiz id, question id) — so the shape here is already the
// shape a scored/persisted quiz needs. Nothing is stored today.
export interface QuizGroup {
	id: string;
	legend?: string;
	// who answers together: 'student' (personal) or 'group' (shared). Honoured by
	// the UI now; the axis a group quiz will persist on later.
	scope: 'student' | 'group';
	// when the answer and rationale become visible.
	reveal: 'on-answer' | 'on-request' | 'never';
	questions: Question[];
}

export interface Question {
	id: string;
	type: 'choice' | 'multi' | 'truefalse' | 'short';
	stem: string;
	// expected response for a `short` question — shown on reveal, never auto-marked.
	answer?: string;
	// why the answer is what it is: the teaching payload of a self-check.
	rationale?: string;
	options?: QuizOption[];
}

export interface QuizOption {
	key: string;
	label: string;
	correct?: boolean;
}

// One student's response: an option key, several keys (a `multi` question), or
// free text (a `short` question).
export type QuizAnswer = string | string[];

export interface SlideRec {
	id: string;
	lesson: string;
	order: number;
	slug: string;
	number: string;
	kind: SlideKind;
	// persona this slide targets (inherited from the lesson if unset).
	audience: string;
	title: string;
	// body/sim_output hold server-rendered HTML (goldmark/hugo), not Markdown.
	body: string;
	agent_key: string;
	sim_output: string;
	// named choice groups; each placed in the body by an inline {{< LMS t="choice" >}}.
	t_choice: ChoiceGroup[] | null;
	// named self-check quizzes; each placed in the body by an inline {{< LMS t="quiz" >}}.
	t_quiz: QuizGroup[] | null;
	// agent-job configs; each dispatched by an inline {{< LMS t="agent" >}}.
	t_agent: AgentJob[] | null;
	// derived-data specs; each rendered by an inline {{< LMS t="tabulate" >}}.
	t_mapper: DataMapper[] | null;
	timers: TimerSegment[] | null;
	notes_label: string;
	// publishing status (students only ever receive live rows; kept for admin use).
	readiness: string;
	content: string;
}

// A teacher-only companion page for a lesson (a briefing/debrief screen a teacher
// reads while running the lesson). Carries the same content fields as a slide so it
// renders through the same <Slide> player, plus `key` — the frontmatter map key
// (e.g. "start"/"end") used in its staff-gated URL /teacher/<lesson>/page/<key>.
export interface TeacherPageRec {
	id: string;
	lesson: string;
	key: string;
	kind: SlideKind;
	audience: string;
	title: string;
	body: string;
	agent_key: string;
	sim_output: string;
	t_choice: ChoiceGroup[] | null;
	t_agent: AgentJob[] | null;
	t_mapper: DataMapper[] | null;
	timers: TimerSegment[] | null;
	notes_label: string;
	readiness: string;
	content: string;
	// expanded relation when requested via ?expand=lesson.
	expand?: { lesson?: LessonRec };
}

// A lesson's schedule config: one row per lesson holding the allow-unscheduled
// gate. Its named cohort windows live in schedule_sessions (below).
export interface LessonScheduleRec {
	id: string;
	lesson: string;
	// when true a student may name their own session instead of picking a cohort.
	allow_unscheduled: boolean;
}

// One named cohort window under a lesson_schedule. `sched_id` is the stable
// author/import anchor; `name` is a mutable teacher-editable label. Student runs
// reference the record id, so a rename never breaks a link.
export interface ScheduleSessionRec {
	id: string;
	lesson_schedule: string;
	sched_id: string;
	name: string;
	// ISO datetime strings ('' = open-ended).
	enroll_from: string;
	enroll_to: string;
	start_from: string;
	start_to: string;
}

// A Group: students collaborating within one (lesson, Session). Identity is
// (lesson, session, tag) so two Sessions never share a group (see docs/glossary.md).
// `session` is the schedule_sessions id, or '' for a solo/unscheduled group.
// Students may read them (to pick their group); only staff create/adjust them.
export interface GroupRec {
	id: string;
	tag: string;
	lesson: string;
	session?: string;
	paused: boolean;
}

// An Attempt: a student's individual, remembered run of a lesson within a Session
// (collection is still named `sessions` — see docs/glossary.md phased mapping).
export interface SessionRec {
	id: string;
	user: string;
	course: string;
	lesson: string;
	current_slide: number;
	session_name: string;
	group_name: string;
	// id-reference to the scheduled cohort window this run joined ('' = unscheduled).
	schedule_session: string;
	work_state: Record<string, unknown>;
	status: 'active' | 'complete';
	created: string;
	updated: string;
	// expanded relations when requested via ?expand=course,lesson,schedule_session
	expand?: {
		course?: CourseRec;
		lesson?: LessonRec;
		schedule_session?: ScheduleSessionRec;
	};
}
