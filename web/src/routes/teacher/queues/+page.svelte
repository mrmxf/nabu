<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	// Moved to /teacher/dashboard/queues. Kept as a redirect because this URL was the
	// teacher dashboard for a while and may be bookmarked or linked from help content.
	onMount(() => goto('/teacher/dashboard/queues', { replaceState: true }));
</script>

<p class="muted">Moved — opening the dashboard…</p>

<style>
	.muted {
		color: var(--pi-meta);
	}
</style>
