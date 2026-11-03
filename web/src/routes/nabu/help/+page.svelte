<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { onMount } from 'svelte';
	import type { CourseRec, LessonRec } from '$lib/types';
	import PersonaBadge from '$lib/components/PersonaBadge.svelte';
	import { personas, matchesFilter, type Persona } from '$lib/personas';

	// Public help index. Lists guest-viewable courses (guest_access = true) and
	// their lessons; no login required. Content is public-read via the liveRule.
	let courses = $state<CourseRec[]>([]);
	let lessons = $state<LessonRec[]>([]);
	let loading = $state(true);
	let error = $state('');

	// persona filter: '' = show all. A lesson matches if it targets the persona or
	// is audience "all".
	let filter = $state<Persona | ''>('');

	const lessonsOf = (courseId: string) =>
		lessons.filter((l) => l.course === courseId && matchesFilter(l.audience, filter));

	onMount(async () => {
		try {
			courses = await pb.collection('courses').getFullList<CourseRec>({
				filter: 'guest_access = true',
				sort: 'order'
			});
			if (courses.length) {
				const ids = courses.map((c) => `course = "${c.id}"`).join(' || ');
				lessons = await pb.collection('lessons').getFullList<LessonRec>({
					filter: ids,
					sort: 'order'
				});
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Could not load help';
		} finally {
			loading = false;
		}
	});
</script>

<h1>Nabu help</h1>
<p class="muted">Learn Nabu from Nabu — these guides are Nabu lessons. No login needed.</p>

<div class="filter" role="group" aria-label="Filter help by audience">
	<span class="muted">For:</span>
	<button class="chip" class:active={filter === ''} onclick={() => (filter = '')}>
		<i class="fa-solid fa-users"></i> Everyone
	</button>
	{#each personas as p (p.id)}
		<button
			class="chip"
			class:active={filter === p.id}
			style={`--pc: var(${p.colorVar})`}
			onclick={() => (filter = p.id)}
		>
			<i class="fa-solid {p.icon}"></i> {p.label}
		</button>
	{/each}
</div>

{#if loading}
	<p class="muted">Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else if !courses.length}
	<p class="muted">No help lessons are published yet.</p>
{:else}
	{#each courses as c (c.id)}
		<div class="card">
			<h2>{c.title}</h2>
			{#if c.summary}<p class="muted">{c.summary}</p>{/if}
			<ul class="lessons">
				{#each lessonsOf(c.id) as l (l.id)}
					<li>
						<a href={`/nabu/help/${c.slug}/${l.slug}`}>{l.title}</a>
						<PersonaBadge audience={l.audience} />
						{#if l.summary}<span class="muted"> — {l.summary}</span>{/if}
					</li>
				{/each}
				{#if lessonsOf(c.id).length === 0}
					<li class="muted">No lessons for this audience.</li>
				{/if}
			</ul>
		</div>
	{/each}
{/if}

<style>
	.filter {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.4rem;
		margin: 0.6rem 0 1rem;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 0.3em;
		border: 1px solid var(--pc, oklch(from var(--hi2) 85% calc(c * 0.4) h));
		color: var(--pc, var(--pi-meta));
		background: transparent;
		border-radius: 999px;
		padding: 0.15em 0.7em;
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}
	.chip.active {
		background: var(--pc, var(--ink-cuneiform));
		color: #fff;
	}
	.lessons {
		margin: 0.4rem 0 0;
		padding-left: 1.1rem;
	}
	.lessons li {
		margin: 0.35rem 0;
	}
	.error {
		color: var(--hi1);
	}
</style>
