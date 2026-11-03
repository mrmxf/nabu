<script lang="ts">
	import { choiceValues } from '$lib/stores/decisions.svelte';
	import type { ResolvedValue } from '$lib/types';

	// Inline value echo mounted by the LMS hydrator from a
	// {{< LMS t="print" from="…" >}} shortcode. It renders, as text, the value the
	// student/group chose for the `from` choice — read reactively from the shared
	// choiceValues store, so it updates live when a group member picks. Shows the
	// choice's default_value_key (resolved by the hydrator) unless `show` overrides.
	// A dangling / unset `from` renders nothing.
	let {
		from,
		show = '',
		defaultKey = 'label'
	}: { from: string; show?: string; defaultKey?: string } = $props();

	let text = $derived.by(() => {
		const v: ResolvedValue | undefined = choiceValues[from];
		if (!v) return '';
		const field = (show || defaultKey || 'label') as keyof ResolvedValue;
		return (v[field] as string) || v.label || '';
	});
</script>

<span class="lms-print">{text}</span>

<style>
	.lms-print {
		font-weight: 600;
	}
</style>
