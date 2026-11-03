<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { usePoll, LIVE_MS } from '$lib/poll';
	import { handleAuthError } from '$lib/stores/auth.svelte';

	// Live histogram of in-flight jobs (queued + running) per group. Polled — the
	// least-risky choice: it reuses the jobs collection's staff list rule and needs
	// no realtime wiring or new endpoint. Staff see all groups; the route guards
	// access. Shared by the teacher and admin dashboards.
	//
	// `bare` drops the card chrome + heading so the component can sit inside an
	// accordion <summary>/<details> that supplies both (the queues dashboard).
	let { bare = false }: { bare?: boolean } = $props();

	interface Row {
		tag: string;
		queued: number;
		running: number;
		total: number;
		maxDepth: number;
	}

	let rows = $state<Row[]>([]);
	let error = $state('');
	let updatedAt = $state<Date | null>(null);
	let loading = $state(true);

	// scale bars to the largest total-or-capacity so lengths are comparable.
	const scale = $derived(Math.max(1, ...rows.map((r) => Math.max(r.total, r.maxDepth))));
	const totals = $derived({
		queued: rows.reduce((n, r) => n + r.queued, 0),
		running: rows.reduce((n, r) => n + r.running, 0)
	});

	async function refresh() {
		try {
			const jobs = await pb.collection('jobs').getFullList({
				filter: "status = 'queued' || status = 'running'",
				expand: 'group',
				sort: '-created'
			});
			const map = new Map<string, Row>();
			for (const j of jobs) {
				const g = (j as { expand?: { group?: { tag?: string; max_depth?: number } } }).expand?.group;
				const tag = g?.tag ?? (j.group as string) ?? 'unknown';
				let r = map.get(tag);
				if (!r) {
					r = { tag, queued: 0, running: 0, total: 0, maxDepth: g?.max_depth ?? 0 };
					map.set(tag, r);
				}
				if (j.status === 'queued') r.queued++;
				else r.running++;
				r.total++;
			}
			rows = [...map.values()].sort((a, b) => b.total - a.total || a.tag.localeCompare(b.tag));
			error = '';
			updatedAt = new Date();
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load queue data';
		} finally {
			loading = false;
		}
	}

	// Polls only while this tab is the visible one and the browser tab is in front —
	// see $lib/poll.
	usePoll(refresh, LIVE_MS);
</script>

<div class="hist" class:card={!bare}>
	<div class="head">
		{#if !bare}<h2>Jobs in flight by group</h2>{/if}
		<span class="spacer"></span>
		<span class="muted small">
			{#if updatedAt}updated {updatedAt.toLocaleTimeString()} · polling 3s{/if}
		</span>
	</div>

	{#if error}
		<p class="error">{error}</p>
	{:else if loading}
		<p class="muted">Loading…</p>
	{:else if rows.length === 0}
		<p class="muted">No jobs in flight right now.</p>
	{:else}
		<div class="summary">
			<span class="chip running">running {totals.running}</span>
			<span class="chip queued">queued {totals.queued}</span>
			<span class="chip total">{rows.length} group{rows.length === 1 ? '' : 's'}</span>
		</div>
		<ul class="bars">
			{#each rows as r (r.tag)}
				<li>
					<span class="tag" title={r.tag}>{r.tag}</span>
					<span class="track">
						<span class="seg running" style={`width:${(r.running / scale) * 100}%`}></span>
						<span class="seg queued" style={`width:${(r.queued / scale) * 100}%`}></span>
						{#if r.maxDepth > 0}
							<span
								class="cap"
								style={`left:${(r.maxDepth / scale) * 100}%`}
								title={`cap ${r.maxDepth}`}
							></span>
						{/if}
					</span>
					<span class="count">
						{r.total}{#if r.maxDepth > 0}<span class="muted">/{r.maxDepth}</span>{/if}
					</span>
				</li>
			{/each}
		</ul>
		<p class="legend muted small">
			<span class="swatch running"></span> running
			<span class="swatch queued"></span> queued
			<span class="swatch cap"></span> per-group cap
		</p>
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
		gap: 0.5rem;
	}
	.head h2 {
		margin: 0;
	}
	.spacer {
		flex: 1;
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
	.summary {
		display: flex;
		gap: 0.4rem;
		margin: 0.6rem 0 0.9rem;
	}
	.chip {
		border-radius: 4px;
		padding: 0.05rem 0.5rem;
		font-size: 0.82rem;
		font-weight: 600;
	}
	.chip.running {
		background: var(--hi2);
		color: #fff;
	}
	.chip.queued {
		background: oklch(from var(--hi2) 94% calc(c * 0.4) h);
		color: var(--pi-body);
	}
	.chip.total {
		background: oklch(from var(--pi-body) 96% 0 h);
		color: var(--pi-meta);
	}
	.bars {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.bars li {
		display: grid;
		grid-template-columns: 6rem 1fr 3rem;
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
	.track {
		position: relative;
		display: flex;
		height: 18px;
		background: oklch(from var(--pi-body) 96% 0 h);
		border-radius: 4px;
		overflow: hidden;
	}
	.seg {
		height: 100%;
	}
	.seg.running {
		background: var(--hi2);
	}
	.seg.queued {
		background: oklch(from var(--hi2) 82% calc(c * 0.4) h);
	}
	.cap {
		position: absolute;
		top: -2px;
		bottom: -2px;
		width: 2px;
		background: var(--hi1);
		overflow: visible;
	}
	.count {
		font-variant-numeric: tabular-nums;
		text-align: right;
		font-size: 0.85rem;
	}
	.legend {
		margin: 0.9rem 0 0;
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.swatch {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 2px;
		margin-left: 0.6rem;
	}
	.swatch.running {
		background: var(--hi2);
	}
	.swatch.queued {
		background: oklch(from var(--hi2) 82% calc(c * 0.4) h);
	}
	.swatch.cap {
		background: var(--hi1);
		width: 2px;
		height: 12px;
	}
</style>
