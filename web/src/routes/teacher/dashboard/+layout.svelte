<script lang="ts">
	import { auth, isStaff } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import Tabs from '$lib/components/Tabs.svelte';

	let { children } = $props();

	// The dashboard shell: one auth guard and one tab bar for every panel.
	//
	// Tabs are real routes, not component state. That means F5 keeps the teacher on
	// the tab they were watching, a tab can be linked from help content, and — the
	// practical win — only the visible panel is mounted, so only it polls.
	//
	// Only built tabs appear. A tab that opens an empty panel is worse than a missing
	// one when someone is troubleshooting in front of a course.
	const tabs = [
		{ href: '/teacher/dashboard/queues', icon: 'fa-solid fa-users-between-lines', label: 'Queues' },
		{ href: '/teacher/dashboard/schedule', icon: 'fa-solid fa-calendar-days', label: 'Schedule' },
		{ href: '/teacher/dashboard/resources', icon: 'fa-solid fa-upload', label: 'Resources' },
		{ href: '/teacher/dashboard/agent', icon: 'fa-solid fa-heart-pulse', label: 'Agent server' }
	];

	onMount(() => {
		if (!auth.user) goto('/login');
		else if (!isStaff()) goto('/');
	});
</script>

<div class="head">
	<h1><i class="fa-solid fa-gauge-high"></i> Dashboard</h1>
	<span class="spacer"></span>
	<a class="back" href="/teacher"><i class="fa-solid fa-arrow-left"></i> Teacher home</a>
</div>

<Tabs {tabs} />

<div class="panel">
	{@render children()}
</div>

<style>
	.head {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
	}
	.head h1 {
		margin-bottom: 0.4rem;
	}
	.spacer {
		flex: 1;
	}
	.back {
		font-size: 0.9rem;
		color: var(--pi-meta);
		text-decoration: none;
	}
	.back:hover {
		color: var(--pi-body);
	}
	.panel {
		margin-top: 1rem;
	}
</style>
