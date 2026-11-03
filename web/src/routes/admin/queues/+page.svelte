<script lang="ts">
	import { auth, isAdmin } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import QueueHistogram from '$lib/components/QueueHistogram.svelte';
	import LmsUsername from '$lib/lms/LmsUsername.svelte';

	// Admin dashboard: identical to the teacher dashboard, admin-only, with the
	// signed-in username in the header, via the reusable LmsUsername component.
	// (Inline lesson content uses {{< LMS t="print" from="username" >}} instead.)
	onMount(() => {
		if (!auth.user) goto('/login');
		else if (!isAdmin()) goto('/');
	});
</script>

<h1>Admin dashboard - <LmsUsername /></h1>
<p class="muted">Live queue pressure across all groups. Updates every few seconds.</p>

<QueueHistogram />

<style>
	.muted {
		color: var(--pi-meta);
	}
</style>
