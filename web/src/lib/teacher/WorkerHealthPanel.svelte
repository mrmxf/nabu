<script lang="ts">
	import { getAgentHealth, type AgentHealth } from '$lib/lms/health';

	// Detailed agent-server health, fetched ON REQUEST rather than polled: load once
	// when the tab opens, then only when the teacher asks. The notifications panel on
	// /teacher already watches alive/offline in the background — this is the "tell me
	// more" view, and a teacher reading it wants a stable page, not numbers that move
	// under them while they read.
	//
	// The cost of manual refresh is stale data that still LOOKS live, so the age of
	// the reading is shown at all times and goes amber past a minute. A status panel
	// that quietly shows a five-minute-old "everything fine" is worse than no panel.

	let health = $state<AgentHealth | null>(null);
	let loading = $state(true);
	let fetchedAt = $state<Date | null>(null);
	let now = $state(Date.now());

	// ticks only to age the "checked N ago" label — no network, no re-fetch.
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});

	const ageS = $derived(fetchedAt ? Math.floor((now - fetchedAt.getTime()) / 1000) : 0);
	const stale = $derived(ageS > 60);

	async function load() {
		loading = true;
		try {
			// force: getAgentHealth caches for 3s client-side and nabu's prober caches
			// 3s server-side, so without this a button press can return the value the
			// page already showed — which reads as a broken button.
			health = await getAgentHealth(true);
			fetchedAt = new Date();
			now = Date.now();
		} finally {
			loading = false;
		}
	}

	load();

	function fmtAge(s: number): string {
		if (s < 5) return 'just now';
		if (s < 60) return `${s}s ago`;
		const m = Math.floor(s / 60);
		return m === 1 ? '1 minute ago' : `${m} minutes ago`;
	}

	function fmtUptime(s: number): string {
		if (s <= 0) return '—';
		if (s < 60) return `${s}s`;
		if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
		const h = Math.floor(s / 3600);
		return `${h}h ${Math.floor((s % 3600) / 60)}m`;
	}
</script>

<section class="card">
	<div class="head">
		<h2>
			<i class="fa-solid fa-heart-pulse"></i> Agent server
			{#if health}
				<span class="dot" class:up={health.alive} class:down={!health.alive}></span>
				<span class="state">{health.alive ? 'online' : 'offline'}</span>
			{/if}
		</h2>
		<span class="spacer"></span>
		{#if fetchedAt}
			<span class="age" class:stale>checked {fmtAge(ageS)}</span>
		{/if}
		<button class="pseudo" onclick={load} disabled={loading}>
			<i class="fa-solid fa-rotate" class:spin={loading}></i>
			{loading ? 'Checking…' : 'Refresh'}
		</button>
	</div>

	{#if loading && !health}
		<p class="muted">Contacting the agent server…</p>
	{:else if health && !health.alive}
		<p class="down-msg">
			<i class="fa-solid fa-circle-exclamation"></i>
			<span>
				<strong>Not responding{health.reason ? ` — ${health.reason}` : ''}.</strong>
				Students can still press the button, but their work will queue and never run.
			</span>
		</p>
	{:else if health}
		<dl class="grid">
			<div><dt>Version</dt><dd>{health.version || '—'}</dd></div>
			<div>
				<dt>Uptime</dt>
				<dd>
					{fmtUptime(health.uptime_s)}
					{#if health.uptime_s > 0 && health.uptime_s < 120}
						<span class="hint">restarted recently</span>
					{/if}
				</dd>
			</div>
			<div><dt>Running</dt><dd>{health.running} of {health.capacity}</dd></div>
			<div><dt>Waiting</dt><dd>{health.queue_depth}</dd></div>
		</dl>

		<p class="agents">
			<span class="lbl">Agents loaded ({health.agents.length})</span>
			{#each health.agents as a (a)}<code>{a}</code>{/each}
			{#if health.agents.length === 0}
				<span class="muted">none — the server is up but has no agents registered</span>
			{/if}
		</p>
	{/if}
</section>

<style>
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		flex-wrap: wrap;
	}
	.head h2 {
		margin: 0;
		font-size: 1.1rem;
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
	}
	.spacer {
		flex: 1;
	}
	.dot {
		width: 0.6rem;
		height: 0.6rem;
		border-radius: 50%;
		display: inline-block;
	}
	.dot.up {
		background: var(--hi4);
	}
	.dot.down {
		background: var(--hi1);
	}
	.state {
		font-size: 0.9rem;
		font-weight: normal;
		color: var(--pi-meta);
	}
	.age {
		font-size: 0.85rem;
		color: var(--pi-meta);
	}
	/* a manual-refresh panel's real failure mode is trusted stale data */
	.age.stale {
		color: oklch(from var(--hi3) 42% c h);
		font-weight: 600;
	}
	.spin {
		animation: spin 0.9s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.muted {
		color: var(--pi-meta);
	}
	.down-msg {
		display: flex;
		gap: 0.6rem;
		align-items: flex-start;
		background: oklch(from var(--hi1) 95% calc(c * 0.5) h);
		color: oklch(from var(--hi1) 40% c h);
		border-radius: 6px;
		padding: 0.7rem 0.9rem;
		margin: 0.8rem 0 0;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
		gap: 0.8rem;
		margin: 0.9rem 0 0;
	}
	dt {
		font-size: 0.8rem;
		color: var(--pi-meta);
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}
	dd {
		margin: 0.1rem 0 0;
		font-size: 1.1rem;
	}
	.hint {
		font-size: 0.75rem;
		color: oklch(from var(--hi3) 42% c h);
		display: block;
	}
	.agents {
		margin: 1rem 0 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		align-items: baseline;
	}
	.agents .lbl {
		font-size: 0.8rem;
		color: var(--pi-meta);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		width: 100%;
	}
	.agents code {
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
		border-radius: 4px;
		padding: 0.1rem 0.4rem;
		font-size: 0.9rem;
	}
</style>
