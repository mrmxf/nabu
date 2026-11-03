<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { onDestroy, onMount } from 'svelte';
	import { handleAuthError } from '$lib/stores/auth.svelte';

	// Teacher control for the per-group image budget (an image agent's "make an image" mode).
	// images_remaining lives on the groups record and is the server's authority; the
	// staff UpdateRule lets a teacher grant a group more (or fewer) images live. We
	// load groups once and follow them over realtime so a decrement (a group made an
	// image) or another teacher's edit shows up without a refresh.
	//
	// `bare` drops the card chrome + heading so the component can sit inside an
	// accordion <summary>/<details> that supplies both (the queues dashboard).
	let { bare = false }: { bare?: boolean } = $props();

	interface Row {
		id: string;
		tag: string;
		lesson: string;
		remaining: number;
		draft: number; // the value in the input, before Set
		saving: boolean;
	}

	let rows = $state<Row[]>([]);
	let error = $state('');
	let loading = $state(true);
	let unsub: (() => void) | undefined;

	function toRow(rec: {
		id: string;
		tag?: string;
		images_remaining?: number;
		expand?: { lesson?: { title?: string; slug?: string } };
	}): Row {
		const remaining = rec.images_remaining ?? 0;
		return {
			id: rec.id,
			tag: rec.tag ?? rec.id,
			lesson: rec.expand?.lesson?.title ?? rec.expand?.lesson?.slug ?? '',
			remaining,
			draft: remaining,
			saving: false
		};
	}

	async function load() {
		try {
			const groups = await pb.collection('groups').getFullList({ expand: 'lesson', sort: 'tag' });
			rows = groups.map((g) => toRow(g as unknown as Parameters<typeof toRow>[0]));
			error = '';
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load groups';
		} finally {
			loading = false;
		}
	}

	// Apply a realtime change to a group's remaining count without discarding an
	// in-progress edit: update the authoritative `remaining`, and move `draft` along
	// only when the teacher hasn't diverged from what was last shown.
	function applyRemote(id: string, remaining: number) {
		rows = rows.map((r) => {
			if (r.id !== id) return r;
			const follow = r.draft === r.remaining; // teacher hasn't typed a new value
			return { ...r, remaining, draft: follow ? remaining : r.draft };
		});
	}

	async function setRemaining(row: Row) {
		row.saving = true;
		rows = [...rows];
		try {
			await pb.collection('groups').update(row.id, { images_remaining: row.draft });
			applyRemote(row.id, row.draft);
		} catch (err) {
			if (!handleAuthError(err)) error = err instanceof Error ? err.message : 'Could not update group';
		} finally {
			row.saving = false;
			rows = [...rows];
		}
	}

	onMount(async () => {
		await load();
		try {
			unsub = await pb.collection('groups').subscribe('*', (e) => {
				const rec = e.record as unknown as { id: string; images_remaining?: number };
				if (e.action === 'update') applyRemote(rec.id, rec.images_remaining ?? 0);
				else void load(); // create/delete: reload the list
			});
		} catch {
			// realtime is optional; the initial load still shows current values.
		}
	});

	onDestroy(() => unsub?.());
</script>

<div class="budgets" class:card={!bare}>
	<div class="head">
		{#if !bare}<h2>Image budget by group</h2>{/if}
		<span class="muted small">images each group can still make · live</span>
	</div>

	{#if error}
		<p class="error">{error}</p>
	{:else if loading}
		<p class="muted">Loading…</p>
	{:else if rows.length === 0}
		<p class="muted">No groups yet.</p>
	{:else}
		<ul class="list">
			{#each rows as r (r.id)}
				<li>
					<span class="tag" title={r.tag}>{r.tag}</span>
					{#if r.lesson}<span class="lesson muted">{r.lesson}</span>{/if}
					<span class="remaining" class:none={r.remaining <= 0}>{r.remaining} left</span>
					<input
						type="number"
						min="0"
						bind:value={r.draft}
						disabled={r.saving}
						aria-label={`set images for ${r.tag}`}
					/>
					<button
						class="pseudo"
						onclick={() => setRemaining(r)}
						disabled={r.saving || r.draft === r.remaining}
					>
						{r.saving ? 'Saving…' : 'Set'}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	.head {
		display: flex;
		align-items: baseline;
		gap: 0.6rem;
	}
	.head h2 {
		margin: 0;
	}
	.muted {
		color: var(--pi-meta);
	}
	.small {
		font-size: 0.82rem;
	}
	.error {
		color: var(--hi1);
	}
	.list {
		list-style: none;
		padding: 0;
		margin: 0.6rem 0 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.list li {
		display: grid;
		grid-template-columns: 6rem 1fr auto 4.5rem auto;
		align-items: center;
		gap: 0.6rem;
	}
	.tag {
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
		font-size: 0.85rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.lesson {
		font-size: 0.82rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.remaining {
		font-variant-numeric: tabular-nums;
		font-size: 0.85rem;
		text-align: right;
	}
	.remaining.none {
		color: var(--hi1);
	}
	.list input[type='number'] {
		width: 4.5rem;
		margin: 0;
	}
</style>
