<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { onMount } from 'svelte';
	import { handleAuthError } from '$lib/stores/auth.svelte';
	import type { TeacherPageRec } from '$lib/types';

	// Lists every lesson's teacher-only companion pages as staff-gated links. The
	// bullet text is the page key (the frontmatter map key, e.g. start/end); the link
	// opens /teacher/<lesson>/page/<key>. Read from the teacher_pages collection, whose
	// list rule is staff-only. `bare` drops the card chrome so it sits in an accordion.

	let { bare = false }: { bare?: boolean } = $props();

	interface LessonGroup {
		lessonSlug: string;
		lessonTitle: string;
		pages: { key: string }[];
	}

	let lessons = $state<LessonGroup[]>([]);
	let error = $state('');
	let loading = $state(true);

	onMount(async () => {
		try {
			const recs = await pb
				.collection('teacher_pages')
				.getFullList<TeacherPageRec>({ expand: 'lesson', sort: 'key' });
			const byLesson = new Map<string, LessonGroup>();
			for (const r of recs) {
				const l = r.expand?.lesson;
				if (!l) continue; // a page whose lesson we can't resolve isn't linkable
				let g = byLesson.get(l.id);
				if (!g) {
					g = { lessonSlug: l.slug, lessonTitle: l.title, pages: [] };
					byLesson.set(l.id, g);
				}
				g.pages.push({ key: r.key });
			}
			lessons = [...byLesson.values()].sort((a, b) => a.lessonTitle.localeCompare(b.lessonTitle));
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load teacher pages';
		} finally {
			loading = false;
		}
	});
</script>

<div class="pages" class:card={!bare}>
	{#if !bare}<h2>Teacher pages</h2>{/if}

	{#if error}
		<p class="error">{error}</p>
	{:else if loading}
		<p class="muted">Loading…</p>
	{:else if lessons.length === 0}
		<p class="muted">No teacher pages yet. Add a <code>teacher_pages</code> map to a lesson's <code>_index.md</code>.</p>
	{:else}
		{#each lessons as l (l.lessonSlug)}
			<h3>{l.lessonTitle}</h3>
			<ul>
				{#each l.pages as p (p.key)}
					<li>
						<a href={`/teacher/${l.lessonSlug}/page/${p.key}`}>{p.key}</a>
					</li>
				{/each}
			</ul>
		{/each}
	{/if}
</div>

<style>
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	h2 {
		margin: 0 0 0.4rem;
	}
	h3 {
		margin: 0.8rem 0 0.2rem;
		font-size: 0.95rem;
	}
	ul {
		margin: 0.2rem 0 0;
		padding-left: 1.2rem;
	}
	li {
		margin: 0.15rem 0;
	}
	.muted {
		color: var(--pi-meta);
	}
	.error {
		color: var(--hi1);
	}
</style>
