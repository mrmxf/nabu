import { mount, unmount, type Component } from 'svelte';
import CountdownTimer from '$lib/components/CountdownTimer.svelte';
import LmsFigure from './LmsFigure.svelte';
import LmsNote from './LmsNote.svelte';
import LmsChoice from './LmsChoice.svelte';
import LmsPrint from './LmsPrint.svelte';
import LmsTabulate from './LmsTabulate.svelte';
import LmsServerStatus from './LmsServerStatus.svelte';
import LmsAgent from './LmsAgent.svelte';
import LmsAgentDef from './LmsAgentDef.svelte';
import LmsBrowser from './LmsBrowser.svelte';
import LmsUploader from './LmsUploader.svelte';
import LmsNabuInfo from './LmsNabuInfo.svelte';
import LmsQuiz from './LmsQuiz.svelte';
import type { ChoiceGroup, AgentJob, QuizGroup, QuizAnswer } from '$lib/types';
import { agentConfig, type GroupDoc } from '$lib/yjs/pbProvider';

// The `t -> component` map is the client half of the LMS widget contract. Its
// keys must match the widget types declared in
// nabu/internal/content/nabu-widget-catalogue.yaml (the single source of truth for
// the arg schemas). Each factory adapts a parsed `param` payload to the
// component's props before mounting.
//
// `choice` and `quiz` are not in this map: they need slide-level data (their
// config lives in t_choice / t_quiz frontmatter) and host callbacks, so
// hydrateLms resolves them specially via the ChoiceContext / QuizContext below.
//
// Purely presentational widgets (define, steps, grid, vocab, toolcard, banner,
// video, …) are absent for a different reason: they are rendered to plain HTML
// at seed time and styled with CSS, so there is nothing to hydrate. See
// nabu/internal/content/contentwidgets.go.
type Payload = Record<string, string>;
type Factory = { component: Component<any>; props: (p: Payload) => Record<string, unknown> };

// ChoiceContext lets hydrateLms turn an inline {{< LMS t="choice" id="…" >}} into
// a live selector and {{< LMS t="print" from="…" >}} into a live value. `groups`
// are the current slide's (to place a picker); `defaultKeyFor` is lesson-wide (a
// print may reference a choice defined on another slide). The picker's current
// selection is read from the shared choiceValues store, not from here.
export interface ChoiceContext {
	groups: ChoiceGroup[];
	onSelect: (id: string, key: string) => void;
	defaultKeyFor: (from: string) => string;
	disabled: boolean;
}

// AgentContext lets hydrateLms resolve {{< LMS t="agent" id="…" >}} against the
// slide's t_agent configs and {{< LMS t="serverstatus" >}} against the student's
// group. simOutput is the slide's authored offline fallback; groupTag is the
// session's group name shown on both widgets.
export interface AgentContext {
	jobs: AgentJob[];
	simOutput: string;
	groupTag: string;
	sessionId: string;
	lessonId: string;
	slide: string;
	disabled: boolean;
	// the group's collaborative document; an agent widget binds its group-editable
	// config to groupDoc's per-agent map so members co-edit settings live. Absent
	// in the read-only help viewer (config renders read-only there).
	groupDoc?: GroupDoc;
}

// QuizContext lets hydrateLms turn an inline {{< LMS t="quiz" id="…" >}} into a
// live self-check. `groups` are the slide's t_quiz declarations; `answers` is the
// host's store of responses, keyed quiz id -> question id -> answer.
//
// The host owns the answers deliberately. Today it keeps them in a plain object
// that dies with the slide (formative only — nothing is stored). When answers are
// persisted, only the host changes: session-scoped storage for scope="student",
// the group's shared Yjs doc for scope="group" (the same plumbing t_choice group
// picks already use). LmsQuiz stays a controlled component throughout.
export interface QuizContext {
	groups: QuizGroup[];
	answers: Record<string, Record<string, QuizAnswer>>;
	onAnswer: (quizId: string, questionId: string, answer: QuizAnswer) => void;
	disabled: boolean;
}

export const lmsRegistry: Record<string, Factory> = {
	timer: {
		component: CountdownTimer,
		// An inline timer reuses the chrome CountdownTimer's single-segment shape.
		props: (p) => ({ segments: [{ label: p.label ?? 'Timer', seconds: parseDuration(p.duration) }] })
	},
	figure: {
		component: LmsFigure,
		props: (p) => ({ src: p.src, alt: p.alt ?? '', caption: p.caption ?? '' })
	},
	note: {
		component: LmsNote,
		props: (p) => ({ tone: p.tone ?? 'info', text: p.text ?? '' })
	},
	'nabu-info': {
		component: LmsNabuInfo,
		props: (p) => ({ verbose: p.verbose === 'true' })
	}
};

// parseDuration converts mm:ss or hh:mm:ss into seconds.
function parseDuration(d: string | undefined): number {
	if (!d) return 0;
	const parts = d.split(':').map((n) => parseInt(n, 10) || 0);
	return parts.reduce((acc, n) => acc * 60 + n, 0);
}

// hydrateLms finds every <lms> element under `root`, parses its param payload,
// and mounts the matching component into it. `choice` and `quiz` widgets are
// resolved via the optional ChoiceContext / QuizContext (they need slide-level
// frontmatter and host callbacks); all other widgets come from lmsRegistry.
// Returns a disposer that unmounts them all (call on teardown / before
// re-hydrating a new slide).
export function hydrateLms(
	root: HTMLElement,
	choiceCtx?: ChoiceContext,
	agentCtx?: AgentContext,
	quizCtx?: QuizContext
): () => void {
	const mounted: Record<string, unknown>[] = [];
	for (const el of Array.from(root.querySelectorAll('lms'))) {
		const raw = el.getAttribute('param');
		if (!raw) continue;
		let payload: Payload;
		try {
			payload = JSON.parse(raw);
		} catch {
			continue;
		}

		if (payload.t === 'serverstatus') {
			el.replaceChildren();
			mounted.push(
				mount(LmsServerStatus, {
					target: el as HTMLElement,
					props: {
						show: payload.show ?? 'health,agents,group',
						groupTag: agentCtx?.groupTag ?? '',
						disabled: agentCtx?.disabled ?? false
					}
				})
			);
			continue;
		}

		if (payload.t === 'browser') {
			el.replaceChildren();
			mounted.push(
				mount(LmsBrowser, {
					target: el as HTMLElement,
					props: {
						tabs: payload.tabs ?? 'resources',
						variant: payload.variant === 'list' ? 'list' : 'full',
						showSize: payload.size !== 'false',
						lessonId: agentCtx?.lessonId ?? '',
						sessionId: agentCtx?.sessionId ?? '',
						disabled: agentCtx?.disabled ?? false
					}
				})
			);
			continue;
		}

		if (payload.t === 'uploader') {
			el.replaceChildren();
			mounted.push(
				mount(LmsUploader, {
					target: el as HTMLElement,
					props: {
						scope: payload.scope === 'group' ? 'group' : 'student',
						accept: payload.accept ?? '',
						label: payload.label ?? '',
						groupTag: agentCtx?.groupTag ?? '',
						sessionId: agentCtx?.sessionId ?? '',
						disabled: agentCtx?.disabled ?? false
					}
				})
			);
			continue;
		}

		if (payload.t === 'quiz') {
			const quiz = quizCtx?.groups.find((q) => q.id === payload.id);
			if (!quiz) continue; // dangling id: leave the inert element in place
			el.replaceChildren();
			mounted.push(
				mount(LmsQuiz, {
					target: el as HTMLElement,
					props: {
						quiz,
						// Controlled: answers live in the parent so persistence can be
						// added there without touching the component. See LmsQuiz.svelte.
						get value() {
							return quizCtx?.answers[quiz.id] ?? {};
						},
						onAnswer: (qid: string, answer: QuizAnswer) =>
							quizCtx?.onAnswer(quiz.id, qid, answer),
						disabled: quizCtx?.disabled ?? false
					}
				})
			);
			continue;
		}

		if (payload.t === 'agent') {
			const job = agentCtx?.jobs.find((j) => j.id === payload.id);
			if (!job) continue; // dangling id: leave the inert element in place
			el.replaceChildren();
			mounted.push(
				mount(LmsAgent, {
					target: el as HTMLElement,
					props: {
						job,
						simOutput: agentCtx?.simOutput ?? '',
						groupTag: agentCtx?.groupTag ?? '',
						sessionId: agentCtx?.sessionId ?? '',
						slide: agentCtx?.slide ?? '',
						// per-agent group config map (live-shared); undefined in the help viewer.
						configMap: agentCtx?.groupDoc ? agentConfig(agentCtx.groupDoc, job.id) : undefined,
						// the whole group doc, so a controller job can read the OTHER agents'
						// group-set additional_prompts at run time (inherit_worker_prompts).
						groupDoc: agentCtx?.groupDoc,
						disabled: agentCtx?.disabled ?? false
					}
				})
			);
			continue;
		}

		if (payload.t === 'agentdef') {
			const job = agentCtx?.jobs.find((j) => j.id === payload.id);
			if (!job) continue; // dangling id: leave the inert element in place
			el.replaceChildren();
			mounted.push(
				mount(LmsAgentDef, {
					target: el as HTMLElement,
					props: { job, field: payload.field ?? 'system' }
				})
			);
			continue;
		}

		if (payload.t === 'choice') {
			if (!choiceCtx) continue; // no host wiring (shouldn't happen); leave inert
			const group = choiceCtx.groups.find((g) => g.id === payload.id);
			if (!group) continue; // dangling id: leave the inert element in place
			el.replaceChildren();
			mounted.push(
				mount(LmsChoice, {
					target: el as HTMLElement,
					props: {
						id: group.id,
						options: group.options,
						legend: group.legend ?? 'Choose one',
						style: group.style ?? 'radio',
						disabled: choiceCtx.disabled,
						onSelect: (k: string) => choiceCtx.onSelect(group.id, k)
					}
				})
			);
			continue;
		}

		if (payload.t === 'tabulate') {
			if (!payload.from) continue;
			el.replaceChildren();
			mounted.push(
				mount(LmsTabulate, { target: el as HTMLElement, props: { from: payload.from } })
			);
			continue;
		}

		if (payload.t === 'print') {
			if (!choiceCtx || !payload.from) continue;
			el.replaceChildren();
			mounted.push(
				mount(LmsPrint, {
					target: el as HTMLElement,
					props: {
						from: payload.from,
						show: payload.show ?? '',
						defaultKey: choiceCtx.defaultKeyFor(payload.from)
					}
				})
			);
			continue;
		}

		const factory = lmsRegistry[payload.t];
		if (!factory) continue; // unknown widget: leave the inert element in place
		el.replaceChildren();
		mounted.push(mount(factory.component, { target: el as HTMLElement, props: factory.props(payload) }));
	}

	// Post-it containers are server-rendered <aside> boxes (not <lms> widgets), so
	// they wrap author markdown that goldmark already rendered. We only attach
	// behaviour (drag + minimise) to each; look/colour is pure CSS.
	const cleanups: Array<() => void> = [];
	for (const el of Array.from(root.querySelectorAll<HTMLElement>('.lms-postit')))
		cleanups.push(wirePostit(el));

	return () => {
		mounted.forEach((m) => unmount(m));
		cleanups.forEach((fn) => fn());
	};
}

// wirePostit prepares a post-it note and returns a disposer that removes every
// listener. It (1) wraps the note body in a .lms-postit-body element so the body
// can be an absolute overlay — the note starts collapsed and expanding it never
// reflows the page; (2) opens external links in a new tab; (3) wires the banner
// as a drag handle and the toggle button to open/close. Drag position is
// EPHEMERAL: a CSS transform offset held in closure state, so it resets whenever
// the slide re-hydrates — by design.
function wirePostit(el: HTMLElement): () => void {
	const banner = el.querySelector<HTMLElement>('.lms-postit-banner');
	const minBtn = el.querySelector<HTMLElement>('.lms-postit-min');
	const cleanups: Array<() => void> = [];

	// Wrap everything after the banner into an overlay body (idempotent).
	if (banner && !el.querySelector(':scope > .lms-postit-body')) {
		const body = document.createElement('div');
		body.className = 'lms-postit-body';
		const rest: ChildNode[] = [];
		for (let n = banner.nextSibling; n; n = n.nextSibling) rest.push(n);
		rest.forEach((n) => body.appendChild(n));
		el.appendChild(body);
	}

	// Post-it links open in a new tab (rel guards against tab-nabbing).
	for (const a of Array.from(el.querySelectorAll<HTMLAnchorElement>('a[href]'))) {
		a.target = '_blank';
		a.rel = 'noopener noreferrer';
	}

	if (minBtn) {
		const onToggle = () => {
			const open = el.classList.toggle('is-open');
			minBtn.setAttribute('aria-expanded', String(open));
		};
		minBtn.addEventListener('click', onToggle);
		cleanups.push(() => minBtn.removeEventListener('click', onToggle));
	}

	if (banner) {
		let dx = 0; // committed offset, updated on each drop
		let dy = 0;
		let sx = 0; // pointer position at drag start
		let sy = 0;
		let dragging = false;
		const onMove = (e: PointerEvent) => {
			if (!dragging) return;
			el.style.transform = `translate(${dx + e.clientX - sx}px, ${dy + e.clientY - sy}px)`;
		};
		const onUp = (e: PointerEvent) => {
			if (!dragging) return;
			dragging = false;
			dx += e.clientX - sx;
			dy += e.clientY - sy;
			banner.classList.remove('is-dragging');
			window.removeEventListener('pointermove', onMove);
			window.removeEventListener('pointerup', onUp);
		};
		const onDown = (e: PointerEvent) => {
			if (e.button !== 0) return; // left button only
			if ((e.target as HTMLElement).closest('.lms-postit-min')) return; // let the button handle its click
			dragging = true;
			sx = e.clientX;
			sy = e.clientY;
			// z-index needs a positioned box; relative keeps the note in flow.
			el.style.position = 'relative';
			el.style.zIndex = '5';
			banner.classList.add('is-dragging');
			window.addEventListener('pointermove', onMove);
			window.addEventListener('pointerup', onUp);
			e.preventDefault();
		};
		banner.addEventListener('pointerdown', onDown);
		cleanups.push(() => {
			banner.removeEventListener('pointerdown', onDown);
			window.removeEventListener('pointermove', onMove);
			window.removeEventListener('pointerup', onUp);
		});
	}

	return () => cleanups.forEach((fn) => fn());
}
