<script lang="ts">
	import { resolvePersona } from '$lib/personas';

	// A small chip showing which persona a lesson/slide targets, in that persona's
	// highlight colour. Renders nothing for "all"/unknown audiences unless
	// showAll is set (then a neutral "Everyone" chip).
	let {
		audience,
		showAll = false,
		size = 'sm'
	}: { audience: string; showAll?: boolean; size?: 'sm' | 'md' } = $props();

	const info = $derived(resolvePersona(audience));
</script>

{#if info}
	<span class="persona {size}" style={`--pc: var(${info.colorVar})`}>
		<i class="fa-solid {info.icon}"></i> {info.label}
	</span>
{:else if showAll}
	<span class="persona all {size}"><i class="fa-solid fa-users"></i> Everyone</span>
{/if}

<style>
	.persona {
		display: inline-flex;
		align-items: center;
		gap: 0.3em;
		border: 1px solid var(--pc, oklch(from var(--hi2) 85% calc(c * 0.4) h));
		color: var(--pc, var(--pi-meta));
		border-radius: 999px;
		padding: 0.05em 0.55em;
		font-weight: 600;
		line-height: 1.5;
		white-space: nowrap;
	}
	.persona.sm {
		font-size: 0.72rem;
	}
	.persona.md {
		font-size: 0.85rem;
	}
	.persona.all {
		--pc: var(--pi-meta);
	}
	.persona i {
		font-size: 0.9em;
	}
</style>
