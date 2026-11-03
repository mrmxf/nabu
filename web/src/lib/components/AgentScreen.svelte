<script lang="ts">
	// Simulated agent screen. There is no real worker in Phase 2 — pressing Run
	// plays a short "thinking" animation (the cheat) and then reveals the canned
	// sim_output authored on the slide (server-rendered HTML). Phase 3 will swap
	// this for a real job.
	let {
		agentKey,
		simOutput,
		disabled = false
	}: { agentKey: string; simOutput: string; disabled?: boolean } = $props();

	type Status = 'idle' | 'running' | 'done';
	let status = $state<Status>('idle');
	let tick = $state(0);

	const avatar = '🤖';

	// fake progress lines, cycled while "running" to feel alive.
	const running = ['Thinking…', 'Working…', 'Finishing…'];
	const line = $derived(running[tick % running.length]);

	let timer: ReturnType<typeof setInterval> | undefined;

	function run() {
		status = 'running';
		tick = 0;
		clearInterval(timer);
		timer = setInterval(() => (tick += 1), 550);
		// reveal the canned result after a short, demo-friendly delay.
		setTimeout(() => {
			clearInterval(timer);
			status = 'done';
		}, 1900);
	}

	function reset() {
		clearInterval(timer);
		status = 'idle';
	}
</script>

<div class="agent">
	<div class="head">
		<span class="avatar">{avatar}</span>
		<strong>{agentKey || 'agent'}</strong>
		<span class="spacer"></span>
		{#if status === 'idle'}
			<button onclick={run} {disabled}>Run {agentKey}</button>
		{:else if status === 'running'}
			<button disabled>Running…</button>
		{:else}
			<button class="pseudo" onclick={reset}>Run again</button>
		{/if}
	</div>

	{#if status === 'running'}
		<div class="running">
			<span class="dot"></span><span class="dot"></span><span class="dot"></span>
			<span class="line">{line}</span>
		</div>
	{:else if status === 'done'}
		<div class="output">{@html simOutput}</div>
	{:else if disabled}
		<p class="muted hint">Sign in to run <em>{agentKey}</em> and see what it produces.</p>
	{:else}
		<p class="muted hint">Press <em>Run {agentKey}</em> to see what this agent produces.</p>
	{/if}
</div>

<style>
	.agent {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		background: oklch(from var(--hi2) 97% calc(c * 0.3) h);
		padding: 0.8rem 1rem;
		margin-top: 1rem;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.avatar {
		font-size: 1.3rem;
	}
	.spacer {
		flex: 1;
	}
	.running {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.8rem 0.2rem;
		color: var(--pi-meta);
	}
	.running .dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--hi2);
		animation: pulse 1s infinite ease-in-out;
	}
	.running .dot:nth-child(2) {
		animation-delay: 0.15s;
	}
	.running .dot:nth-child(3) {
		animation-delay: 0.3s;
	}
	.running .line {
		margin-left: 0.5rem;
		font-style: italic;
	}
	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
			transform: translateY(0);
		}
		50% {
			opacity: 1;
			transform: translateY(-2px);
		}
	}
	.output {
		margin-top: 0.6rem;
		animation: fade 0.4s ease;
	}
	.output :global(table) {
		width: 100%;
	}
	@keyframes fade {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
	.hint {
		margin: 0.6rem 0 0;
	}
</style>
