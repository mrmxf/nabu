<script lang="ts">
	import { page } from '$app/stores';

	// The app's one tab bar. Two flavours, because the two callers genuinely differ:
	//
	//   href  — route-backed tabs (the teacher dashboard). Navigation changes the URL,
	//           so F5 keeps your place, a tab is linkable from help content, and each
	//           panel mounts/unmounts — which stops the hidden ones polling for free.
	//   value — in-page tabs (LmsBrowser's resource scopes), where the tab is a view
	//           of one component's state and a URL would be noise.
	//
	// Icon-only rendering is the teacher-dashboard default: five glyphs stay compact
	// on a tablet. `label` is still required — it becomes aria-label and the tooltip,
	// and without it a screen reader gets nothing but a decorative <i>.

	export interface Tab {
		label: string; // required: aria-label + tooltip, and the visible text when showLabels
		icon?: string; // FontAwesome classes, e.g. 'fa-solid fa-upload'
		href?: string; // route-backed
		value?: string; // in-page
	}

	let {
		tabs,
		active = '',
		showLabels = false,
		onSelect
	}: {
		tabs: Tab[];
		active?: string;
		showLabels?: boolean;
		onSelect?: (value: string) => void;
	} = $props();

	// A route tab is current when the path is the tab or sits beneath it, so a
	// nested route still highlights its parent tab.
	function isActive(t: Tab): boolean {
		if (t.href) {
			const p = $page.url.pathname.replace(/\/$/, '');
			const h = t.href.replace(/\/$/, '');
			return p === h || p.startsWith(h + '/');
		}
		return t.value === active;
	}
</script>

<div class="tabs" role="tablist">
	{#each tabs as t (t.href ?? t.value ?? t.label)}
		{#if t.href}
			<a
				class="tab"
				class:active={isActive(t)}
				href={t.href}
				role="tab"
				aria-selected={isActive(t)}
				aria-label={t.label}
				title={t.label}
			>
				{#if t.icon}<i class={t.icon}></i>{/if}
				{#if showLabels || !t.icon}<span class="label">{t.label}</span>{/if}
			</a>
		{:else}
			<button
				class="tab"
				class:active={isActive(t)}
				role="tab"
				aria-selected={isActive(t)}
				aria-label={t.label}
				title={t.label}
				onclick={() => onSelect?.(t.value ?? '')}
			>
				{#if t.icon}<i class={t.icon}></i>{/if}
				{#if showLabels || !t.icon}<span class="label">{t.label}</span>{/if}
			</button>
		{/if}
	{/each}
</div>

<style>
	.tabs {
		display: flex;
		gap: 0.3rem;
		border-bottom: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		flex-wrap: wrap;
	}
	.tab {
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		padding: 0.4rem 0.8rem;
		margin: 0;
		color: var(--pi-meta);
		cursor: pointer;
		border-radius: 0;
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		text-decoration: none;
		font-size: 1rem;
		line-height: 1.6;
	}
	.tab:hover {
		color: var(--pi-body);
		background: oklch(from var(--hi2) 97% calc(c * 0.3) h);
	}
	/* Keyboard users get a real focus ring — with icon-only tabs the tooltip is
	   mouse-only, so the ring is the main "where am I" signal when tabbing. */
	.tab:focus-visible {
		outline: 2px solid var(--hi2);
		outline-offset: -2px;
	}
	.tab.active {
		color: var(--pi-body);
		border-bottom-color: var(--hi2);
		font-weight: 600;
	}
	.tab i {
		font-size: 1.05rem;
	}
</style>
