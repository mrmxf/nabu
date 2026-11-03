<script lang="ts">
	import Chooser from '$lib/components/Chooser.svelte';
	import { choiceValues } from '$lib/stores/decisions.svelte';
	import type { Choice } from '$lib/types';

	// Inline multiple-choice widget mounted by the LMS hydrator from a
	// {{< LMS t="choice" id="…" >}} shortcode. The hydrator resolves the id to the
	// slide's t_choice group and passes its options + id in here.
	//
	// The visible selection is read reactively from the shared choiceValues store
	// by `id`, so a group member's pick live-updates every member's picker. A click
	// notifies the host via onSelect, which resolves + stores + persists the value.
	//
	// `style` picks the presentation: 'radio' (default) is the Chooser button
	// selector; 'dropdown' is a native <select>. Because picks are shared by id, a
	// later slide can re-declare the same group as a dropdown to re-show the pick.
	let {
		id,
		options,
		legend = 'Choose one',
		style = 'radio',
		disabled = false,
		onSelect
	}: {
		id: string;
		options: Choice[];
		legend?: string;
		style?: 'radio' | 'dropdown';
		disabled?: boolean;
		onSelect?: (key: string) => void;
	} = $props();

	// Live selection: the stored pick for this id (updates on our own pick and on a
	// group member's pick via realtime).
	let selected = $derived(choiceValues[id]?.key ?? '');

	function pick(k: string) {
		onSelect?.(k);
	}
</script>

{#if style === 'dropdown'}
	<fieldset class="dropdown">
		<legend>{legend}</legend>
		<select {disabled} value={selected} onchange={(e) => pick(e.currentTarget.value)}>
			<option value="" disabled>— choose —</option>
			{#each options as c (c.key)}
				<option value={c.key}>{c.key}) {c.label}</option>
			{/each}
		</select>
	</fieldset>
{:else}
	<Chooser choices={options} {selected} {legend} {disabled} onSelect={pick} />
{/if}

<style>
	.dropdown {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.6rem 1rem 1rem;
		margin-top: 1rem;
	}
	.dropdown legend {
		font-weight: 600;
		padding: 0 0.4rem;
	}
	.dropdown select {
		width: 100%;
		margin-top: 0.45rem;
	}
</style>
