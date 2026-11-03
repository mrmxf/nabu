<script lang="ts">
	import type { SlideRec } from '$lib/types';

	// The lesson progress bar: one numbered, clickable segment per slide. Shared by
	// both lesson players (help viewer + session player) so navigation looks and
	// behaves identically. `onGoto(i)` jumps to slide i (0-based); the host decides
	// whether that also persists.
	let {
		slides,
		current,
		onGoto
	}: { slides: SlideRec[]; current: number; onGoto: (i: number) => void } = $props();
</script>

<div class="segbar" aria-label="Slides">
	{#each slides as s, i (s.id)}
		<button
			class="seg"
			class:current={i === current}
			aria-current={i === current ? 'true' : undefined}
			aria-label={`slide ${i + 1}: ${s.title}`}
			data-tooltip={s.title}
			onclick={() => onGoto(i)}>{i + 1}</button
		>
	{/each}
</div>

<style>
	.segbar {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
		margin-bottom: 0.6rem;
	}
	.segbar .seg {
		min-width: 1.9rem;
		margin: 0;
		padding: 0.1rem 0.4rem;
		font-size: 0.8rem;
		font-variant-numeric: tabular-nums;
		line-height: 1.4;
		cursor: pointer;
		border: 1px solid oklch(from var(--hi2) 85% calc(c * 0.4) h);
		background: oklch(from var(--hi2) 96% calc(c * 0.3) h);
		color: var(--hi2);
		border-radius: 4px;
		transition:
			background 0.15s ease,
			color 0.15s ease;
	}
	.segbar .seg:hover {
		background: oklch(from var(--hi2) 90% calc(c * 0.6) h);
	}
	.segbar .seg.current {
		background: var(--hi2);
		color: #fff;
		border-color: var(--hi2);
		font-weight: 700;
	}
	/* Picnic pins its data-tooltip BELOW the button (top: 100%), where the mouse
	   cursor sits and hides the first word (notably in Opera). Flip the bubble and
	   its arrow ABOVE the button instead; keep Picnic's default styling otherwise. */
	.segbar .seg::after {
		top: auto;
		bottom: calc(100% - 6px);
		margin-top: 0;
		margin-bottom: 12px;
	}
	.segbar .seg::before {
		top: auto;
		bottom: calc(100% - 6px);
		margin-top: 0;
		margin-bottom: 0;
		/* arrow now points down, toward the button */
		border-color: #111 transparent transparent;
	}
</style>
