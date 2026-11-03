<script lang="ts">
	import ResourcePanel from '$lib/teacher/ResourcePanel.svelte';

	// Tab (d): lesson resource upload/browse. Auth is handled by the dashboard layout.
</script>

<p class="muted">Upload, import, and browse a lesson's resource bucket.</p>

<ResourcePanel />

<style>
	.muted {
		color: var(--pi-meta);
	}
</style>
