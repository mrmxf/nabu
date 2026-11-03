<script lang="ts">
	import type { Choice } from '$lib/types';

	// A reusable single-choice selector. Uses real buttons (not styled radios) so
	// clicks always register and the selected option has clear visual feedback,
	// independent of the CSS framework. Used by the topic and controller pickers.
	let {
		choices,
		selected = '',
		onSelect,
		legend = 'Choose one',
		disabled = false
	}: {
		choices: Choice[];
		selected?: string;
		onSelect: (key: string) => void;
		legend?: string;
		disabled?: boolean;
	} = $props();
</script>

<fieldset class="chooser">
	<legend>{legend}</legend>
	{#each choices as c (c.key)}
		<button
			type="button"
			class="option"
			class:selected={selected === c.key}
			aria-pressed={selected === c.key}
			{disabled}
			onclick={() => onSelect(c.key)}
		>
			<span class="dot"></span>
			<span class="key">{c.key})</span>
			<span class="opt-label">{c.label}</span>
		</button>
	{/each}
</fieldset>

<style>
	.chooser {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.6rem 1rem 1rem;
		margin-top: 1rem;
		font-size: 0.9em;
	}
	.chooser legend {
		font-weight: 600;
		padding: 0 0.4rem;
	}
	/* full-width option rows; override PicnicCSS button styling */
	.option {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		width: 100%;
		text-align: left;
		background: #fff;
		color: inherit;
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.6rem 0.8rem;
		margin: 0.45rem 0;
		cursor: pointer;
		font: inherit;
		transition:
			background 0.12s ease,
			border-color 0.12s ease;
	}
	.option:hover:not(:disabled) {
		border-color: var(--hi2);
		background: oklch(from var(--hi2) 97% calc(c * 0.3) h);
	}
	.option:disabled {
		cursor: not-allowed;
		opacity: 0.55;
	}
	.option:focus-visible {
		outline: 2px solid var(--hi2);
		outline-offset: 2px;
	}
	.option.selected {
		border-color: var(--hi2);
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
		box-shadow: inset 0 0 0 1px var(--hi2);
	}
	.dot {
		flex: 0 0 auto;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		border: 2px solid oklch(from var(--hi2) 78% calc(c * 0.4) h);
		position: relative;
	}
	.option.selected .dot {
		border-color: var(--hi2);
	}
	.option.selected .dot::after {
		content: '';
		position: absolute;
		inset: 3px;
		border-radius: 50%;
		background: var(--hi2);
	}
	.key {
		font-weight: normal;
	}
	.opt-label {
		flex: 1;
	}
</style>
