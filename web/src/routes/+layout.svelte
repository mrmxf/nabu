<script lang="ts">
	import 'picnic';
	import '$lib/app.css';
	import { auth, logout, isAdmin, isStaff, isAuthed } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import { loadSettings, setting } from '$lib/stores/settings.svelte';
	import { loadNabuInfo, nabuInfo } from '$lib/stores/nabu.svelte';
	import { navLogoCandidates, resetBranding, contentStylesheets } from '$lib/stores/branding.svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';

	// The deployment's name (app_config `app_name`, from pocketbase.app.name).
	const appName = $derived(setting('app_name') || 'Nabu');
	$effect(() => {
		document.title = appName;
	});

	let { children } = $props();

	onMount(() => {
		loadSettings();
		loadNabuInfo();
	});

	// Hamburger menu (shown below the CSS breakpoint). Close it on navigation,
	// Escape, and clicks outside the nav.
	let menuOpen = $state(false);
	let navEl: HTMLElement | undefined = $state();
	function onWindowClick(e: MouseEvent) {
		if (menuOpen && navEl && !navEl.contains(e.target as Node)) menuOpen = false;
	}

	// The navbar logo cascades lesson → course → default, and the authored
	// stylesheets layer course → lesson. Clear both on navigation; the destination
	// page re-sets them if it shows a course/lesson.
	afterNavigate(() => {
		resetBranding();
		menuOpen = false;
	});

	// Authored stylesheets for the current course/lesson, in cascade order. Emitted
	// into <head> AFTER nabu.css (which app.html links statically), so a course
	// theme overrides the base tokens and a lesson overrides its course. Svelte
	// removes them when the view changes, so styling never leaks between courses.
	let stylesheets = $derived(contentStylesheets());

	// Ordered logo candidates (lesson → course → default → bundled Nabu logo). We
	// render the first and step down on a load error, so a missing per-course/lesson
	// image degrades to the next fallback and there is always a logo.
	let candidates = $derived(navLogoCandidates());
	let logoIdx = $state(0);
	$effect(() => {
		// reset to the top candidate whenever the set changes (e.g. on navigation)
		candidates;
		logoIdx = 0;
	});
	let logo = $derived(candidates[Math.min(logoIdx, candidates.length - 1)] ?? '');

	function signOut() {
		logout();
		goto('/login');
	}

	// Guests can read the help lessons (/nabu/help) but every try-it/save action
	// is disabled; this banner explains why and offers a way in.
	let guestBanner = $derived(!isAuthed() && $page.url.pathname.startsWith('/nabu/help'));
	// the help "?" link opens in-place when already in help, else in a new tab.
	let onHelp = $derived($page.url.pathname.startsWith('/nabu/help'));
</script>

<svelte:head>
	{#each stylesheets as href (href)}
		<link rel="stylesheet" {href} />
	{/each}
</svelte:head>

<svelte:window
	onkeydown={(e) => e.key === 'Escape' && (menuOpen = false)}
	onclick={onWindowClick}
/>

<nav bind:this={navEl}>
	<a href="/" class="brand">
		{#if logo}
			<img
				class="site-logo"
				src={logo}
				alt=""
				onerror={() => {
					if (logoIdx < candidates.length - 1) logoIdx += 1;
				}}
			/>
		{:else}
			<span class="logo">📜</span>
		{/if}
		<span>{appName}</span>
	</a>
	<button
		class="nav-toggle"
		aria-label="Menu"
		aria-expanded={menuOpen}
		onclick={() => (menuOpen = !menuOpen)}
	>
		<i class="fa-solid fa-bars"></i>
	</button>
	<div class="menu" class:open={menuOpen}>
		{#if auth.user}
			{#if isStaff()}
				<a href="/teacher" class="admin-link">Teacher</a>
				<a href="/teacher/dashboard/queues" class="admin-link">Dashboard</a>
			{/if}
			{#if isAdmin()}
				<a href="/admin" class="admin-link">Admin</a>
			{/if}
			<span class="who">{auth.user.name || auth.user.email}</span>
			<button class="pseudo" onclick={signOut}>Sign out</button>
		{:else if $page.url.pathname !== '/login'}
			<a href="/login" class="button">Sign in</a>
		{/if}
		<ThemeToggle />
		<a
			href="/nabu/help"
			class="help-link"
			title={`Nabu ${nabuInfo.version || ''}\nHelp`}
			aria-label="Help"
			target={onHelp ? undefined : '_blank'}
			rel={onHelp ? undefined : 'noopener'}
		>
			<i class="fa-solid fa-circle-question"></i>
		</a>
	</div>
</nav>

{#if guestBanner}
	<div class="guest-warning" role="status">
		<span>⚠️ You're browsing Nabu help as a guest — action buttons are disabled.</span>
		<a href="/login" class="button">Sign in</a>
	</div>
{/if}

<main class="shell" class:has-warning={guestBanner}>
	{@render children()}
</main>

<style>
	nav {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.4rem 1.2rem;
		background: var(--ink-cuneiform);
	}
	nav .brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: #fff;
		font-weight: 700;
		text-decoration: none;
	}
	nav .logo {
		font-size: 1.4rem;
	}
	nav .site-logo {
		height: 26px;
		width: auto;
	}
	nav .menu {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		/* Base colour for items that inherit — i.e. the .pseudo "Sign out" button,
		   which Picnic pins to `color: inherit`. Every link overrides with its own. */
		color: oklch(from var(--hi3) 70% calc(c * 0.7) h);
	}
	nav .who {
		color: oklch(from var(--hi2) 86% calc(c * 0.4) h);
		font-size: 0.9rem;
	}
	nav .admin-link {
		color: oklch(from var(--hi3) 82% calc(c * 0.8) h);
		font-weight: 600;
	}
	/* Sign out is a <button> (an action, not a link). It inherits the dim amber
	   from .menu above (Picnic pins .pseudo to color:inherit); brighten on hover. */
	nav .menu button.pseudo:hover {
		color: oklch(from var(--hi3) 82% calc(c * 0.8) h);
	}
	nav .help-link {
		color: #fff;
		font-size: 1.25rem;
		line-height: 1;
		text-decoration: none;
		display: inline-flex;
		align-items: center;
	}
	nav .help-link:hover {
		color: oklch(from var(--hi3) 82% calc(c * 0.8) h);
	}
	/* Hamburger toggle — hidden on wide screens; the menu is a normal flex row. */
	nav .nav-toggle {
		display: none;
		background: none;
		border: none;
		color: #fff;
		font-size: 1.4rem;
		line-height: 1;
		cursor: pointer;
		padding: 0.2rem 0.4rem;
	}
	nav .nav-toggle:hover {
		color: oklch(from var(--hi3) 82% calc(c * 0.8) h);
	}
	/* Below the breakpoint: show the hamburger; the menu collapses into a dropdown
	   anchored under the fixed nav, revealed only when .open. */
	@media (max-width: 640px) {
		nav .nav-toggle {
			display: inline-flex;
			align-items: center;
		}
		nav .menu {
			display: none;
			/* Anchor to the VIEWPORT (not the 3em nav box) so the dropdown always
			   opens from the top, just under the fixed nav — never vertically
			   centred in the bar (which pushed a tall teacher menu off the top). */
			position: fixed;
			top: 3em; /* PicnicCSS nav height */
			right: 0.6rem;
			/* A tall menu (teacher: Teacher/Dashboard/Admin/…) scrolls INSIDE the
			   dropdown instead of clipping past the screen edges. */
			max-height: calc(100vh - 3em - 0.6rem);
			overflow-y: auto;
			flex-direction: column;
			align-items: flex-end;
			gap: 0.7rem;
			padding: 0.8rem 1rem;
			background: var(--ink-cuneiform);
			border-radius: 0 0 8px 8px;
			box-shadow: 0 8px 16px rgba(0, 0, 0, 0.25);
			z-index: 10001; /* above the fixed nav (PicnicCSS z-index 10000) */
		}
		nav .menu.open {
			display: flex;
		}
	}
	.shell {
		/* Desktop default: wide content area. Narrow screens fall back to the
		   original reading width (see the media query below). */
		max-width: 1800px;
		margin: 0 auto;
		/* PicnicCSS makes <nav> position:fixed (height 3em); offset content clear
		   of the banner with ~2em of breathing room below it. */
		padding: calc(3em + 2rem) 1.2rem 4rem;
	}
	@media (max-width: 1000px) {
		.shell {
			max-width: 820px;
		}
	}
	/* extra top offset so the fixed guest warning bar doesn't cover content. */
	.shell.has-warning {
		padding-top: calc(3em + 2.6rem + 2rem);
	}
	.guest-warning {
		position: fixed;
		top: 3em;
		left: 0;
		right: 0;
		z-index: 10;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		padding: 0.5rem 1.2rem;
		background: oklch(from var(--hi3) 85% calc(c * 0.7) h);
		color: oklch(from var(--hi3) 40% c h);
		font-size: 0.9rem;
		box-shadow: 0 2px 6px rgba(0, 0, 0, 0.12);
	}
	.guest-warning .button {
		padding: 0.1rem 0.7rem;
		font-size: 0.85rem;
	}
</style>
