<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { auth, handleAuthError } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount, onDestroy } from 'svelte';
	import { renderTemplate } from '$lib/templating';
	import {
		setCourseLogo,
		setLessonLogo,
		setCourseCss,
		setLessonCss
	} from '$lib/stores/branding.svelte';
	import {
		choiceValues,
		setChoiceValue,
		commitChoice,
		bindChoices,
		setBuiltins,
		resetChoiceValues,
		resolvePick
	} from '$lib/stores/decisions.svelte';
	import {
		dataValues,
		resetDataValues,
		fetchSource,
		computeMapper
	} from '$lib/stores/dataMappers.svelte';
	import { createGroupDoc, noteText, applyTextDiff, type GroupDoc } from '$lib/yjs/pbProvider';
	import { loadSettings, imagesEnabled } from '$lib/stores/settings.svelte';
	import type { SlideRec, SessionRec, ChoiceGroup, DataMapper } from '$lib/types';
	import CountdownTimer from '$lib/components/CountdownTimer.svelte';
	import SyncedTimer from '$lib/components/SyncedTimer.svelte';
	import Slide from '$lib/components/Slide.svelte';
	import SlideSegments from '$lib/components/SlideSegments.svelte';

	const sessionId = $page.params.id ?? '';

	let session = $state<SessionRec | null>(null);
	let slides = $state<SlideRec[]>([]);
	let idx = $state(0);
	let loading = $state(true);
	let error = $state('');
	let saving = $state(false);

	// free-text notes keyed by slide id — a read mirror of the group's shared Yjs
	// notes (co-edited live by the whole group; no longer per-student in
	// work_state). Choice picks live in the shared choiceValues store.
	let notes = $state<Record<string, string>>({});

	// the group's collaborative document (notes / choices / agent configs), one
	// per (group, lesson). Created on mount once the session (hence group) is known.
	let gdoc = $state<GroupDoc | undefined>(undefined);

	let slide = $derived(slides[idx]);
	let total = $derived(slides.length);

	// An image slide drives an agent in "make an image" mode — any agent-job config
	// whose fixed `format` input is "image". Used to drop these from the run when
	// the image back-end is disabled (see the slides load in onMount).
	function isImageSlide(s: SlideRec): boolean {
		return (s.t_agent ?? []).some((a) => String(a.inputs?.format ?? '') === 'image');
	}

	// Display name for this run: the scheduled cohort's CURRENT name (so a teacher
	// rename propagates — references are by id), falling back to the stored
	// session_name for unscheduled/legacy runs.
	let sessionLabel = $derived(
		session?.expand?.schedule_session?.name || session?.session_name || ''
	);

	// Lesson-wide choice groups by id, so a pick can be resolved and a print can
	// find the default_value_key of a choice defined on any slide.
	let groupsById = $derived.by(() => {
		const m: Record<string, ChoiceGroup> = {};
		for (const s of slides) for (const g of s.t_choice ?? []) m[g.id] = g;
		return m;
	});

	// lesson-wide t_mapper specs; derived tables recompute reactively from the data
	// + the choice values.
	let mapperSpecs = $derived.by(() => {
		const out: DataMapper[] = [];
		for (const s of slides) for (const m of s.t_mapper ?? []) out.push(m);
		return out;
	});
	$effect(() => {
		for (const m of mapperSpecs) computeMapper(m);
	});

	// defaultKeyFor tells a {{< LMS t="print" from="…" >}} which field to show.
	const defaultKeyFor = (from: string) => groupsById[from]?.default_value_key || 'label';

	// onChoose resolves a pick and commits it: a group-scoped pick lands in the
	// shared Yjs `choices` map (replicated to the whole group); a session-scoped
	// pick stays local and is saved into work_state on navigation.
	function onChoose(id: string, key: string) {
		const g = groupsById[id];
		if (!g) return;
		commitChoice(id, resolvePick(g, key));
	}

	// seed the store from work_state (session picks) and bind the shared Yjs
	// `choices` map (group picks). Binding seeds whatever the group has already
	// decided and keeps the mirror live as members pick.
	function seedChoiceValues(ws: Record<string, unknown>) {
		resetChoiceValues();
		// built-in print values for this session (username/email/group/session).
		setBuiltins({
			username: auth.user?.name || auth.user?.email || 'guest',
			email: auth.user?.email ?? '',
			group: session?.group_name ?? '',
			session: sessionLabel
		});
		const wsChoices = (ws.choices ?? {}) as Record<string, unknown>;
		for (const [id, raw] of Object.entries(wsChoices)) {
			if (raw && typeof raw === 'object') setChoiceValue(id, raw as never);
			else if (typeof raw === 'string' && raw) {
				const g = groupsById[id];
				setChoiceValue(id, g ? resolvePick(g, raw) : { key: raw, label: raw, slug: raw, scope: 'session' });
			}
		}
		if (gdoc) bindChoices(gdoc.choices);
	}

	onDestroy(() => {
		resetChoiceValues(); // unbinds the choices observer before the doc is torn down
		gdoc?.destroy();
		resetDataValues();
	});

	// notes label rendered with the session's group name (may contain HTML).
	const notesLabelHtml = (s: SlideRec) =>
		renderTemplate(s.notes_label, { group: session?.group_name });

	// mirror the group's shared notes (Y.Text per slide) into the reactive `notes`
	// map so the textareas re-render when a group member types.
	function syncNotes() {
		if (!gdoc) return;
		const next: Record<string, string> = {};
		for (const [slideId, text] of gdoc.notes.entries()) next[slideId] = text.toString();
		notes = next;
	}

	// onNote applies a keystroke to the shared Y.Text as a minimal diff, so two
	// members editing the same note merge instead of clobbering each other.
	function onNote(slideId: string, value: string) {
		if (!gdoc) {
			notes[slideId] = value; // groupless/offline fallback: local only
			return;
		}
		applyTextDiff(noteText(gdoc, slideId), value);
	}

	onMount(async () => {
		if (!auth.user) {
			goto('/login');
			return;
		}
		try {
			session = await pb
				.collection('sessions')
				.getOne<SessionRec>(sessionId, { expand: 'lesson,course,schedule_session' });
			// navbar logo: this lesson's, falling back to its course's, then default.
			setLessonLogo(session.expand?.lesson?.logo);
			setCourseLogo(session.expand?.course?.logo);
			// authored styling for the slide bodies: course theme, then lesson override.
			setCourseCss(session.expand?.course?.css);
			setLessonCss(session.expand?.lesson?.css);
			// settings gate the image surface: when the Stable Diffusion back-end is
			// disabled (app_config `images_enabled`), drop "make an image" slides
			// from the run entirely so no image option or copy reaches the student.
			await loadSettings();
			const allSlides = await pb.collection('slides').getFullList<SlideRec>({
				filter: `lesson = "${session.lesson}"`,
				sort: 'order'
			});
			slides = imagesEnabled() ? allSlides : allSlides.filter((s) => !isImageSlide(s));
			// fetch each unique t_mapper data source once (served at /data/<name>.json).
			for (const src of new Set(mapperSpecs.map((m) => m.source))) fetchSource(src);
			const ws = session.work_state ?? {};
			// open the group's collaborative document; notes live here now (shared),
			// not in this student's work_state.
			gdoc = createGroupDoc(sessionId);
			gdoc.notes.observeDeep(syncNotes);
			gdoc.ready.then(syncNotes);
			seedChoiceValues(ws);
			idx = Math.min(Math.max(session.current_slide ?? 0, 0), Math.max(slides.length - 1, 0));
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load session';
		} finally {
			loading = false;
		}
	});

	// Persist position + notes + choices on every navigation so a reload or
	// re-login resumes exactly where the student left off.
	async function persist(nextIdx: number, status?: 'active' | 'complete') {
		if (!session) return;
		saving = true;
		try {
			// only session-scoped picks belong in work_state; group picks live in the
			// group's Yjs `choices` map, and built-ins are re-derived each visit.
			const choices: Record<string, unknown> = {};
			for (const [id, v] of Object.entries(choiceValues)) {
				if (v.scope === 'session') choices[id] = v;
			}
			// notes are no longer persisted here — they live in the group's shared
			// Yjs document (ydoc_updates), co-edited by the whole group.
			const work_state = { ...session.work_state, choices };
			const data: Record<string, unknown> = { current_slide: nextIdx, work_state };
			if (status) data.status = status;
			session = await pb.collection('sessions').update<SessionRec>(session.id, data);
			captureDerived();
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not save progress';
		} finally {
			saving = false;
		}
	}

	// captureDerived snapshots the current derived tables to the captures collection
	// (fire-and-forget) so a teacher can later inspect what data the group worked on.
	function captureDerived() {
		const items = mapperSpecs
			.filter((m) => dataValues[m.id])
			.map((m) => ({
				id: m.id,
				scope: groupsById[m.from]?.scope ?? choiceValues[m.from]?.scope ?? 'session',
				value: dataValues[m.id]
			}));
		if (!items.length) return;
		fetch('/api/captures/set', {
			method: 'POST',
			headers: { Authorization: pb.authStore.token, 'Content-Type': 'application/json' },
			body: JSON.stringify({ session_id: sessionId, items })
		}).catch(() => {
			/* capture is best-effort audit data; never block navigation */
		});
	}

	async function next() {
		if (idx < total - 1) {
			const n = idx + 1;
			await persist(n);
			idx = n;
			window.scrollTo(0, 0);
		}
	}
	async function back() {
		if (idx > 0) {
			const n = idx - 1;
			await persist(n);
			idx = n;
			window.scrollTo(0, 0);
		}
	}
	async function finish() {
		await persist(idx, 'complete');
		goto('/');
	}
	// jump to slide n (0-based) from the segmented bar, persisting position.
	async function jumpTo(n: number) {
		if (n >= 0 && n < total && n !== idx) {
			await persist(n);
			idx = n;
			window.scrollTo(0, 0);
		}
	}

	// In-lesson cross-references — the client half of a two-part mechanism.
	//
	// An authored `[text](help:<lesson>#<slide>)` pointing at THIS lesson is
	// rendered at seed time as `<a href="#slug" data-nabu-slide="slug">` by
	// rewriteXrefLinks (nabu/internal/content/render.go — read its doc comment
	// first if this looks odd). It is a data attribute rather than a URL because
	// an authored course has no per-slide route at all: this route is
	// /session/[id]/run, and which slide is showing is `idx` below, persisted as
	// sessions.current_slide. There is no /BTASB/lesson/slide to link to.
	//
	// So "following" such a link is a deck jump, not a navigation.
	//
	// Delegated from the container rather than bound per link, because slide
	// bodies arrive via {@html} and are replaced wholesale on every slide change —
	// per-link listeners would be lost on the first Next. preventDefault stops the
	// bare fragment from also scrolling the page. A target that no longer exists
	// is left alone (seed already warned the author about it), so a stale link is
	// inert rather than jumping somewhere arbitrary.
	function onBodyClick(e: MouseEvent) {
		const a = (e.target as HTMLElement | null)?.closest?.('a[data-nabu-slide]');
		if (!a) return;
		const slug = a.getAttribute('data-nabu-slide');
		if (!slug) return;
		const n = slides.findIndex((s) => s.slug === slug);
		if (n < 0) return;
		e.preventDefault();
		void jumpTo(n);
	}
</script>

{#if loading}
	<p class="muted">Loading attempt…</p>
{:else if error}
	<div class="card"><p class="error">{error}</p><a href="/">← Back to dashboard</a></div>
{:else if slide}
	<SlideSegments {slides} current={idx} onGoto={jumpTo} />
	<div class="row step">
		<p class="muted">
			Slide {idx + 1} of {total} · <span class="kind">{slide.kind}</span>
			{#if sessionLabel}· {sessionLabel}{/if}
			{#if saving}· saving…{/if}
		</p>
		<span class="spacer"></span>
		{#if slide.timers && slide.timers.length}
			{#key slide.id}
				<!-- Scoped timers are cohort-wide synchronized countdowns (one per id,
				     the same value for every student); the rest are legacy local segments. -->
				{#each slide.timers.filter((t) => t.scope) as t (t.id)}
					{#if session}<SyncedTimer attemptId={session.id} timer={t} />{/if}
				{/each}
				{@const localSegs = slide.timers.filter((t) => !t.scope)}
				{#if localSegs.length}<CountdownTimer segments={localSegs} />{/if}
			{/key}
		{/if}
	</div>

	<!-- Click delegation for in-lesson xref links inside the rendered body; see
	     onBodyClick. Keyboard activation of an <a> fires a click too, so this needs
	     no separate key handler. -->
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div onclick={onBodyClick}>
		<Slide
			{slide}
			{onChoose}
			{defaultKeyFor}
			noteValue={notes[slide.id] ?? ''}
			onNote={(v) => onNote(slide.id, v)}
			notesLabelHtml={notesLabelHtml(slide)}
			groupTag={session?.group_name ?? ''}
			sessionId={session?.id ?? ''}
			lessonId={session?.lesson ?? ''}
			groupDoc={gdoc}
		/>
	</div>

	<div class="row nav">
		<button class="pseudo" onclick={back} disabled={idx === 0}>← Back</button>
		<span class="spacer"></span>
		{#if idx < total - 1}
			<button onclick={next}>Next →</button>
		{:else}
			<button class="finish" onclick={finish}>Finish attempt ✓</button>
		{/if}
	</div>

	<p class="muted"><a href="/">Save &amp; exit to dashboard</a></p>
{/if}

<style>
	.step {
		margin: 0 0 0.8rem;
		align-items: center;
	}
	.step p {
		margin: 0;
	}
	.kind {
		text-transform: uppercase;
		font-size: 0.75rem;
		letter-spacing: 0.05em;
		background: oklch(from var(--hi2) 94% calc(c * 0.4) h);
		color: var(--pi-body);
		padding: 0.05rem 0.4rem;
		border-radius: 4px;
	}
	.nav {
		margin: 1rem 0 0.5rem;
		align-items: center;
	}
	.finish {
		background: oklch(from var(--hi4) 42% c h);
	}
	.error {
		color: var(--hi1);
	}
</style>
