<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { TimerSegment } from '$lib/types';

	// A reusable countdown. Segments run in order relative to when the page is
	// shown: when one hits zero the next begins (e.g. 15-min task -> 10-min
	// discussion). Displays the current label and h:mm:ss remaining.
	let { segments }: { segments: TimerSegment[] } = $props();

	let idx = $state(0);
	let remaining = $state(0);
	let done = $state(false);

	let targetMs = 0;
	let handle: ReturnType<typeof setInterval> | undefined;

	function startSegment(i: number) {
		idx = i;
		remaining = segments[i].seconds;
		targetMs = Date.now() + segments[i].seconds * 1000;
	}

	function tick() {
		const secs = Math.round((targetMs - Date.now()) / 1000);
		if (secs > 0) {
			remaining = secs;
			return;
		}
		if (idx + 1 < segments.length) {
			startSegment(idx + 1);
		} else {
			remaining = 0;
			done = true;
			clearInterval(handle);
		}
	}

	// (re)start whenever the set of segments changes (i.e. a new slide).
	$effect(() => {
		clearInterval(handle);
		done = false;
		if (segments.length) {
			startSegment(0);
			handle = setInterval(tick, 250);
		}
		return () => clearInterval(handle);
	});

	onDestroy(() => clearInterval(handle));

	function fmt(s: number): string {
		const h = Math.floor(s / 3600);
		const m = Math.floor((s % 3600) / 60);
		const sec = s % 60;
		return `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`;
	}

	let current = $derived(segments[idx]);
	let urgent = $derived(!done && remaining <= 60);
</script>

{#if segments.length}
	<div class="timer" class:urgent class:done>
		<span class="clock">⏱️</span>
		{#if done}
			<span class="seg-label">Time's up</span>
			<span class="time">0:00:00</span>
		{:else}
			<span class="seg-label">{current?.label}</span>
			<span class="time">{fmt(remaining)}</span>
			{#if segments.length > 1}
				<span class="of">segment {idx + 1} of {segments.length}</span>
			{/if}
		{/if}
	</div>
{/if}

<style>
	.timer {
		display: inline-flex;
		align-items: baseline;
		gap: 0.5rem;
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.35rem 0.8rem;
		font-variant-numeric: tabular-nums;
	}
	.clock {
		font-size: 1.1rem;
	}
	.seg-label {
		font-weight: 600;
		color: var(--pi-body);
	}
	.time {
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--hi2);
		letter-spacing: 0.02em;
	}
	.of {
		font-size: 0.75rem;
		color: var(--pi-meta);
	}
	.timer.urgent {
		background: oklch(from var(--hi1) 96% calc(c * 0.4) h);
		border-color: oklch(from var(--hi1) 88% calc(c * 0.5) h);
	}
	.timer.urgent .time {
		color: var(--hi1);
	}
	.timer.done {
		background: oklch(from var(--pi-body) 94% 0 h);
	}
	.timer.done .time {
		color: var(--pi-meta);
	}
</style>
