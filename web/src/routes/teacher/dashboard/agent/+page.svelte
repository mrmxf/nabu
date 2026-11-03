<script lang="ts">
	import WorkerHealthPanel from '$lib/teacher/WorkerHealthPanel.svelte';

	// Tab: agent server health, on request. Auth is handled by the dashboard layout.
	// Deliberately NOT polled — see WorkerHealthPanel. This tab is also where the
	// worker log tail will live once nabu proxies ni6's /v1/logs.
</script>

<p class="muted">
	The machine that runs the AI agents. Checked when you open this tab, and whenever
	you press Refresh.
</p>

<WorkerHealthPanel />

<style>
	.muted {
		color: var(--pi-meta);
	}
</style>
