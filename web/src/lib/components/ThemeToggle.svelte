<script lang="ts">
	import { onMount } from 'svelte';

	// Tri-state theme control, matching the pihuw pattern and the wiring documented
	// in nabu.css §2: "light"/"dark" force a theme via the data-theme attribute on
	// <html>; "system" clears it so the prefers-color-scheme @media block decides.
	// The inline theme-init script in app.html restores the saved choice before the
	// first paint (no FOUC); this button keeps it in sync and cycles it on click.
	type Mode = 'light' | 'dark' | 'system';
	const KEY = 'nabu-theme';
	// Cycle order the user asked for: light → dark → system → light.
	const order: Mode[] = ['light', 'dark', 'system'];
	const meta: Record<Mode, { icon: string; label: string }> = {
		light: { icon: 'fa-sun', label: 'Light' },
		dark: { icon: 'fa-moon', label: 'Dark' },
		system: { icon: 'fa-circle-half-stroke', label: 'System' }
	};

	// Starts at 'system'; onMount reconciles with the persisted choice (localStorage
	// is unavailable during SSR/prerender, so we can't read it in the initializer).
	let mode = $state<Mode>('system');

	function apply(m: Mode) {
		const root = document.documentElement;
		if (m === 'system') root.removeAttribute('data-theme');
		else root.dataset.theme = m;
		try {
			if (m === 'system') localStorage.removeItem(KEY);
			else localStorage.setItem(KEY, m);
		} catch {
			// Storage disabled (private mode): the theme still applies for this visit.
		}
	}

	function readSaved(): Mode {
		try {
			const v = localStorage.getItem(KEY);
			if (v === 'light' || v === 'dark') return v;
		} catch {
			// ignore — fall through to system
		}
		return 'system';
	}

	onMount(() => {
		mode = readSaved();
	});

	function cycle() {
		mode = order[(order.indexOf(mode) + 1) % order.length];
		apply(mode);
	}
</script>

<button
	type="button"
	class="theme-toggle"
	data-theme-toggle
	data-theme-state={mode}
	title={`Theme: ${meta[mode].label} — click to change`}
	aria-label={`Theme: ${meta[mode].label}. Click to change.`}
	onclick={cycle}
>
	<i class={`fa-solid ${meta[mode].icon}`}></i>
</button>

<style>
	/* Bare icon button; overrides Picnic's filled button styling from app.css. */
	.theme-toggle {
		background: none;
		border: 0;
		padding: 0;
		margin: 0;
		cursor: pointer;
		font-size: 1.25rem;
		line-height: 1;
		display: inline-flex;
		align-items: center;
	}
	/* Icon colour reflects the current state, mirroring the pihuw toggle. */
	.theme-toggle[data-theme-state='system'] i {
		color: #fff;
	}
	.theme-toggle[data-theme-state='light'] i {
		color: oklch(from var(--hi3) 82% calc(c * 0.9) h); /* warm sun */
	}
	.theme-toggle[data-theme-state='dark'] i {
		color: oklch(from var(--hi4) 80% calc(c * 0.9) h); /* cool moon */
	}
	.theme-toggle:hover i {
		filter: brightness(1.2);
	}
</style>
