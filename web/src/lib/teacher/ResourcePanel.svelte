<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { httpError } from '$lib/httpError';
	import { handleAuthError } from '$lib/stores/auth.svelte';
	import { onMount } from 'svelte';
	import type { CourseRec, LessonRec } from '$lib/types';
	import LmsBrowser from '$lib/lms/LmsBrowser.svelte';

	// Resource panel: pick a course + lesson, then upload/import/browse that lesson's
	// S3 resources and see total bucket usage. Extracted from the old
	// /teacher/resources page so the dashboard tab and that route share one
	// implementation. Assumes the caller has already established staff access.
	let courses = $state<CourseRec[]>([]);
	let lessons = $state<LessonRec[]>([]);
	let courseId = $state('');
	let lessonId = $state('');
	let loadErr = $state('');

	let usage = $state<{ bytes: number; count: number } | null>(null);
	let usageErr = $state('');

	// course -> lesson cascade (same pattern as the student dashboard).
	let classLessons = $derived(lessons.filter((l) => l.course === courseId));
	let lesson = $derived(lessons.find((l) => l.id === lessonId));
	$effect(() => {
		if (courseId && !classLessons.some((l) => l.id === lessonId)) {
			lessonId = classLessons[0]?.id ?? '';
		}
	});

	onMount(async () => {
		try {
			[courses, lessons] = await Promise.all([
				pb.collection('courses').getFullList<CourseRec>({ sort: 'order' }),
				pb.collection('lessons').getFullList<LessonRec>({ sort: 'order' })
			]);
			if (courses.length) courseId = courses[0].id;
		} catch (err) {
			if (handleAuthError(err)) return;
			loadErr = err instanceof Error ? err.message : 'Could not load courses';
		}
	});

	// fresh bypasses the server's usage cache — used after an upload, which the
	// server never sees directly (the browser PUTs straight to S3).
	async function loadUsage(fresh = false) {
		usage = null;
		usageErr = '';
		if (!lessonId || !lesson?.bucket) return;
		try {
			const q = fresh ? '&fresh=1' : '';
			const res = await fetch(
				`/api/resources/usage?lesson_id=${encodeURIComponent(lessonId)}${q}`,
				{ headers: { Authorization: pb.authStore.token } }
			);
			if (!res.ok) throw await httpError(res, 'Could not load usage');
			usage = await res.json();
		} catch (e) {
			usageErr = e instanceof Error ? e.message : 'Could not load usage';
		}
	}

	// refetch usage when the lesson changes (the cached value is fine here).
	$effect(() => {
		void lessonId;
		loadUsage();
	});

	function human(n: number): string {
		if (n < 1024) return `${n} B`;
		if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
		if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
		return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
	}
</script>

{#if loadErr}
	<div class="card"><p class="error">{loadErr}</p></div>
{:else}
	<section class="card">
		<div class="chooser">
			<label>Course
				<select bind:value={courseId}>
					{#each courses as c (c.id)}<option value={c.id}>{c.title}</option>{/each}
				</select>
			</label>
			<label>Lesson
				<select bind:value={lessonId}>
					{#each classLessons as l (l.id)}<option value={l.id}>{l.title}</option>{/each}
				</select>
			</label>
			<span class="spacer"></span>
			<div class="usage" title="Total objects and size in this lesson's bucket">
				<i class="fa-solid fa-database"></i>
				{#if usageErr}
					<span class="error">usage n/a</span>
				{:else if usage}
					<strong>{human(usage.bytes)}</strong> <span class="muted">· {usage.count} files</span>
				{:else}
					<span class="muted">—</span>
				{/if}
			</div>
		</div>

		{#if lesson && !lesson.bucket}
			<p class="warn">
				<i class="fa-solid fa-circle-info"></i> This lesson has no resource bucket. Add
				<code>params.bucket</code> (and optionally <code>params.prefix</code>) to its
				<code>_index.md</code>.
			</p>
		{:else if lesson}
			<p class="muted small">
				bucket <code>{lesson.bucket}</code>{#if lesson.prefix}
					· prefix <code>{lesson.prefix}</code>{/if} · uploads land under
				<code>resources/</code>
			</p>
		{/if}
	</section>

	{#if lesson?.bucket}
		<section class="card">
			<LmsBrowser
				tabs="resources group student"
				{lessonId}
				onChanged={() => loadUsage(true)}
			/>
		</section>
	{/if}
{/if}

<style>
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	.chooser {
		display: flex;
		gap: 1rem;
		align-items: flex-end;
		flex-wrap: wrap;
	}
	.chooser label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-weight: normal;
	}
	/* PicnicCSS gives the native select only ~0.45em right padding, so a long
	   option value runs under the dropdown arrow (fixed ~15px from the edge).
	   Reserve room so the text is clipped before it reaches the arrow. */
	.chooser select {
		padding-right: 2rem;
	}
	.spacer {
		flex: 1;
	}
	.usage {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.4rem 0.8rem;
		background: oklch(from var(--hi2) 97% calc(c * 0.3) h);
	}
	.muted {
		color: var(--pi-meta);
	}
	.small {
		font-size: 0.85rem;
	}
	.error {
		color: var(--hi1);
	}
	.warn {
		background: oklch(from var(--hi3) 95% calc(c * 0.5) h);
		color: oklch(from var(--hi3) 42% c h);
		border-radius: 6px;
		padding: 0.6rem 0.8rem;
	}
</style>
