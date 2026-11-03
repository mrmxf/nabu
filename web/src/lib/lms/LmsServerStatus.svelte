<script lang="ts">
	import { untrack } from 'svelte';
	import { usePoll, STEADY_MS } from '$lib/poll';
	import { getAgentHealth, type AgentHealth } from './health';

	// Read-only status panel mounted by hydrateLms from a
	// {{< LMS t="serverstatus" >}} shortcode. It shows whether the home agent
	// server is alive, which agents are loaded, and the student's group tag so
	// they can find their own jobs in any queue list. It dispatches nothing.
	//
	// `show` is a comma-separated subset of health,agents,group (default all).
	// `groupTag` comes from the host (the session's group name). Mounted
	// imperatively, so state + polling are owned locally.
	let {
		show = 'health,agents,group',
		groupTag = '',
		disabled = false
	}: { show?: string; groupTag?: string; disabled?: boolean } = $props();

	// Props are a one-time seed here: hydrateLms mounts this imperatively and never
	// updates them, so snapshot the reads to keep them out of the reactive graph.
	const panels = untrack(() => show.split(',').map((s) => s.trim()));
	const showHealth = panels.includes('health');
	const showAgents = panels.includes('agents');
	const showGroup = panels.includes('group');

	let health = $state<AgentHealth | null>(null);
	let loading = $state(true);

	async function refresh(force = false) {
		health = await getAgentHealth(force);
		loading = false;
	}

	// Poll while mounted so the online/offline flip is visible without a reload.
	// Disabled in the read-only help preview (no live server context there).
	if (untrack(() => disabled)) {
		refresh();
	} else {
		// First read may use health.ts's shared cache — a slide with several agent
		// widgets should not fire one request each. Every later read forces, so the
		// online/offline flip stays live.
		let first = true;
		usePoll(() => {
			const force = !first;
			first = false;
			return refresh(force);
		}, STEADY_MS);
	}

	function fmtUptime(s: number): string {
		if (s < 60) return `${s}s`;
		if (s < 3600) return `${Math.floor(s / 60)}m`;
		return `${Math.floor(s / 3600)}h`;
	}
</script>

<div class="status" class:disabled>
	{#if showHealth}
		<div class="panel">
			{#if loading}
				<span class="dot loading"></span><span class="muted">Checking agent server…</span>
			{:else if health?.alive}
				<span class="dot ok"></span>
				<strong>Agent server online</strong>
				<span class="meta">
					queue {health.queue_depth}/{health.capacity} · up {fmtUptime(health.uptime_s)} · v{health.version}
				</span>
			{:else}
				<span class="dot off"></span>
				<strong>Agent server offline</strong>
				<span class="meta">{health?.reason ?? 'unreachable'}</span>
			{/if}
		</div>
	{/if}

	{#if showAgents && health?.alive && health.agents.length}
		<div class="panel agents">
			<span class="label">Agents ready:</span>
			{#each health.agents as a (a)}
				<span class="chip">{a}</span>
			{/each}
		</div>
	{/if}

	{#if showGroup}
		<div class="panel group">
			<span class="label">Your group:</span>
			{#if groupTag}
				<span class="tag">{groupTag}</span>
				<span class="meta">jobs you submit are labelled with this tag</span>
			{:else}
				<span class="meta">no group set — add a group name on your dashboard to track your jobs</span>
			{/if}
		</div>
	{/if}
</div>

<style>
	.status {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		background: oklch(from var(--hi2) 97% calc(c * 0.3) h);
		padding: 0.7rem 1rem;
		margin-top: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.status.disabled {
		opacity: 0.75;
	}
	.panel {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		flex-wrap: wrap;
	}
	.dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex: none;
	}
	.dot.ok {
		background: oklch(from var(--hi4) 48% c h);
	}
	.dot.off {
		background: var(--hi1);
	}
	.dot.loading {
		background: oklch(from var(--hi2) 82% calc(c * 0.35) h);
		animation: pulse 1s infinite ease-in-out;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}
	.meta {
		color: var(--pi-meta);
		font-size: 0.85rem;
	}
	.label {
		font-weight: 600;
		color: var(--pi-body);
	}
	.chip {
		background: oklch(from var(--hi2) 94% calc(c * 0.4) h);
		color: var(--pi-body);
		border-radius: 4px;
		padding: 0.05rem 0.45rem;
		font-size: 0.85rem;
	}
	.tag {
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
		background: var(--ink-cuneiform);
		color: #fff;
		border-radius: 4px;
		padding: 0.05rem 0.45rem;
	}
</style>
