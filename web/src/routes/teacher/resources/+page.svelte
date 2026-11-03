<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	// Moved to /teacher/dashboard/resources; the page body now lives in
	// $lib/teacher/ResourcePanel.svelte. Kept as a redirect for existing bookmarks.
	onMount(() => goto('/teacher/dashboard/resources', { replaceState: true }));
</script>

<p class="muted">Moved — opening resources…</p>

<style>
	.muted {
		color: var(--pi-meta);
	}
</style>
