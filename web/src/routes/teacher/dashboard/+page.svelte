<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	// /teacher/dashboard has no content of its own — send it to the first tab.
	// replaceState so Back returns to wherever the teacher came from, not here.
	onMount(() => goto('/teacher/dashboard/queues', { replaceState: true }));
</script>

<p class="muted">Opening queues…</p>

<style>
	.muted {
		color: var(--pi-meta);
	}
</style>
