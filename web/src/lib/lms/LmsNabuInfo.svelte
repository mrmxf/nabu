<script lang="ts">
	import { nabuInfo, loadNabuInfo } from '$lib/stores/nabu.svelte';
	import { onMount } from 'svelte';

	// verbose defaults to false — the registry factory maps the shortcode's
	// verbose="true" string to this boolean.
	let { verbose = false }: { verbose?: boolean } = $props();

	onMount(() => {
		loadNabuInfo();
	});

	const shortHash = $derived(nabuInfo.hash ? nabuInfo.hash.slice(0, 7) : '');
</script>

{#if verbose}
	<dl class="nabu-info">
		<dt>Version</dt>
		<dd>{nabuInfo.version || '—'}</dd>
		<dt>Tag</dt>
		<dd>{nabuInfo.tag || '—'}</dd>
		<dt>Commit</dt>
		<dd><code>{nabuInfo.hash || '—'}</code></dd>
		<dt>Built</dt>
		<dd>{nabuInfo.built || '—'} ({nabuInfo.build || '—'})</dd>
		<dt>Platform</dt>
		<dd>{nabuInfo.os || '?'}/{nabuInfo.arch || '?'}</dd>
	</dl>
{:else}
	<span class="nabu-info-terse"
		>Nabu {nabuInfo.version || '…'}{#if shortHash}&nbsp;(<code>{shortHash}</code>){/if}</span
	>
{/if}

<style>
	.nabu-info {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 0.2rem 0.8rem;
		margin: 0.5rem 0;
		font-size: 0.9rem;
	}
	.nabu-info dt {
		font-weight: 600;
		opacity: 0.75;
	}
	.nabu-info dd {
		margin: 0;
	}
	.nabu-info-terse {
		font-size: 0.95rem;
	}
	code {
		font-size: 0.85em;
	}
</style>
