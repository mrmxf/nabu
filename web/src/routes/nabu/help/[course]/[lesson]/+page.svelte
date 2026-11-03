<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { auth, isAuthed } from '$lib/stores/auth.svelte';
	import { page } from '$app/stores';
	import { onMount, onDestroy } from 'svelte';
	import type { SlideRec, CourseRec, LessonRec, ChoiceGroup, DataMapper } from '$lib/types';
	import { staticLesson } from '$lib/lms/staticContent';
	import CountdownTimer from '$lib/components/CountdownTimer.svelte';
	import Slide from '$lib/components/Slide.svelte';
	import PersonaBadge from '$lib/components/PersonaBadge.svelte';
	import ShareBar from '$lib/components/ShareBar.svelte';
	import SlideSegments from '$lib/components/SlideSegments.svelte';
	import { loadSettings } from '$lib/stores/settings.svelte';
	import {
		setCourseLogo,
		setLessonLogo,
		setCourseCss,
		setLessonCss
	} from '$lib/stores/branding.svelte';
	import {
		setChoiceValue,
		setBuiltins,
		resetChoiceValues,
		resolvePick
	} from '$lib/stores/decisions.svelte';
	import { resetDataValues, fetchSource, computeMapper } from '$lib/stores/dataMappers.svelte';

	// NOTE FOR ANYONE CHASING A LESSON LINK: this is the ONLY static
	// course/lesson route in the app, and it deliberately serves the help tree
	// only (guest_access courses — see the getFirstListItem filter below).
	//
	// An authored course has no equivalent route. Its slides are viewable solely
	// inside a running session (/session/[id]/run), where the current slide is
	// player state, not part of the URL. That asymmetry is why cross-reference
	// links render differently per tree: real URLs here, in-player jumps there.
	// The full explanation lives on rewriteXrefLinks in
	// nabu/internal/content/render.go — start there before changing either side.
	const courseSlug = $page.params.course ?? '';
	const lessonSlug = $page.params.lesson ?? '';

	let title = $state('');
	let slides = $state<SlideRec[]>([]);
	let idx = $state(0);
	let loading = $state(true);
	let error = $state('');

	// In help, all write/try-it actions are disabled unless the visitor is signed in.
	let disabled = $derived(!isAuthed());

	let slide = $derived(slides[idx]);
	let total = $derived(slides.length);

	// Lesson-wide choice groups by id (for pick resolution + print default_value_key).
	let groupsById = $derived.by(() => {
		const m: Record<string, ChoiceGroup> = {};
		for (const s of slides) for (const g of s.t_choice ?? []) m[g.id] = g;
		return m;
	});
	const defaultKeyFor = (from: string) => groupsById[from]?.default_value_key || 'label';

	// derived tables (t_mapper) recompute reactively from the fetched data + choices.
	let mapperSpecs = $derived.by(() => {
		const out: DataMapper[] = [];
		for (const s of slides) for (const m of s.t_mapper ?? []) out.push(m);
		return out;
	});
	$effect(() => {
		for (const m of mapperSpecs) computeMapper(m);
	});

	// Help is a read-only preview: picks update the shared store for this visit only
	// (no persistence, no group sync).
	function onChoose(id: string, key: string) {
		const g = groupsById[id];
		if (g) setChoiceValue(id, resolvePick(g, key));
	}

	onDestroy(() => {
		resetChoiceValues();
		resetDataValues();
	});

	onMount(async () => {
		loadSettings();
		try {
			// a static build first (works with no server), then PocketBase.
			const published = await staticLesson(courseSlug, lessonSlug);
			if (published && !published.course.guest_access) throw new Error('This lesson is not public.');
			const cls =
				published?.course ??
				(await pb
					.collection('courses')
					.getFirstListItem<CourseRec>(`slug = "${courseSlug}" && guest_access = true`));
			const lesson =
				published?.lesson ??
				(await pb
					.collection('lessons')
					.getFirstListItem<LessonRec>(`course = "${cls.id}" && slug = "${lessonSlug}"`));
			title = lesson.title;
			// navbar logo: this lesson's, falling back to its course's, then default.
			setLessonLogo(lesson.logo);
			setCourseLogo(cls.logo);
			// authored styling for the slide bodies: course theme, then lesson override.
			setCourseCss(cls.css);
			setLessonCss(lesson.css);
			slides =
				published?.slides ??
				(await pb.collection('slides').getFullList<SlideRec>({
					filter: `lesson = "${lesson.id}"`,
					sort: 'order'
				}));
			resetChoiceValues(); // fresh, ephemeral picks for this preview visit
			resetDataValues();
			// fetch each unique t_mapper data source once.
			for (const src of new Set((slides.flatMap((s) => s.t_mapper ?? []) as DataMapper[]).map((m) => m.source)))
				fetchSource(src);
			// built-ins: help has no session, so only username/email resolve.
			setBuiltins({
				username: auth.user?.name || auth.user?.email || 'guest',
				email: auth.user?.email ?? ''
			});
			// deep link: open on the slide named by the URL hash, if any.
			gotoHash();
		} catch (err) {
			error = err instanceof Error ? err.message : 'This help lesson is not available.';
		} finally {
			loading = false;
		}
	});

	// react to same-page hash links (e.g. a `help:` xref to a sibling slide).
	$effect(() => {
		window.addEventListener('hashchange', gotoHash);
		return () => window.removeEventListener('hashchange', gotoHash);
	});

	// gotoHash points idx at the slide named by the current URL hash, if it exists.
	function gotoHash() {
		const want = decodeURIComponent(location.hash.replace(/^#/, ''));
		const found = slides.findIndex((s) => s.slug === want);
		if (found >= 0 && found !== idx) {
			idx = found;
			window.scrollTo(0, 0);
		}
	}

	// keep the URL hash in sync with the current slide, so the address bar is
	// always a shareable deep link to exactly this page.
	$effect(() => {
		if (slide) history.replaceState(null, '', `#${slide.slug}`);
	});

	function next() {
		if (idx < total - 1) {
			idx += 1;
			window.scrollTo(0, 0);
		}
	}
	function back() {
		if (idx > 0) {
			idx -= 1;
			window.scrollTo(0, 0);
		}
	}
	// jump straight to slide n (0-based) via the segmented progress bar.
	function goto(n: number) {
		if (n >= 0 && n < total && n !== idx) {
			idx = n;
			window.scrollTo(0, 0);
		}
	}
</script>

<p class="muted"><a href="/nabu/help">← All help</a></p>

{#if loading}
	<p class="muted">Loading…</p>
{:else if error}
	<div class="card"><p class="error">{error}</p><a href="/nabu/help">← All help</a></div>
{:else if slide}
	<h1>{title}</h1>
	<SlideSegments {slides} current={idx} onGoto={goto} />
	<div class="row step">
		<p class="muted">
			Slide {idx + 1} of {total} · <span class="kind">{slide.kind}</span>
			{#if slide.audience}· <PersonaBadge audience={slide.audience} />{/if}
		</p>
		<span class="spacer"></span>
		{#if slide.timers && slide.timers.length}
			{#key slide.id}
				<CountdownTimer segments={slide.timers} />
			{/key}
		{/if}
	</div>

	<Slide {slide} {disabled} {onChoose} {defaultKeyFor} />

	<div class="row nav">
		<button class="pseudo" onclick={back} disabled={idx === 0}>← Back</button>
		<span class="spacer"></span>
		{#if idx < total - 1}
			<button onclick={next}>Next →</button>
		{:else}
			<a class="button" href="/nabu/help">Done ✓</a>
		{/if}
	</div>

	<ShareBar title={`${title} · ${slide.title}`} />
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
	.error {
		color: var(--hi1);
	}
</style>
