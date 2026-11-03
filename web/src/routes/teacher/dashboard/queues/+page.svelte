<script lang="ts">
	import QueueHistogram from '$lib/components/QueueHistogram.svelte';
	import ImageBudgets from '$lib/components/ImageBudgets.svelte';
	import TimersCard from '$lib/components/TimersCard.svelte';
	import TeacherPages from '$lib/components/TeacherPages.svelte';
	import { imagesEnabled } from '$lib/stores/settings.svelte';

	// Tab (a): live queue pressure per group, per-group image budgets, and the
	// lesson teacher pages — each in a collapsed, pure-CSS <details> accordion so the
	// tab stays compact and the teacher expands what they need. Auth is handled by the
	// dashboard layout; the cards render `bare` so the accordion supplies the chrome.
</script>

<details class="acc">
	<summary><i class="fa-solid fa-users-between-lines"></i> Jobs in flight by group</summary>
	<p class="muted">
		Jobs waiting and running for each group. A group at its cap refuses new submissions
		until one finishes.
	</p>
	<QueueHistogram bare />
</details>

<!-- Image budget card only when the Stable Diffusion back-end is enabled
     (app_config `images_enabled`); otherwise there is no image surface to meter. -->
{#if imagesEnabled()}
	<details class="acc">
		<summary><i class="fa-solid fa-image"></i> Image budget by group</summary>
		<p class="muted">
			Images each group can still make with image agents. Grant a group more (or fewer) by setting a
			new number — it takes effect immediately.
		</p>
		<ImageBudgets bare />
	</details>
{/if}

<details class="acc">
	<summary><i class="fa-solid fa-stopwatch"></i> Timers</summary>
	<p class="muted">
		Shared countdowns for the session — every student in a cohort sees the same time. A timer
		starts when the first student reaches a timed slide; pause, add minutes, or reset it here.
	</p>
	<TimersCard bare />
</details>

<details class="acc">
	<summary><i class="fa-solid fa-chalkboard-user"></i> Teacher pages</summary>
	<p class="muted">
		Teacher-only briefing and debrief pages for each lesson. Open one to follow a lesson
		alongside a group — you pick which group to assist on the page itself.
	</p>
	<TeacherPages bare />
</details>

<style>
	.acc {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.4rem 1.2rem;
		margin: 0.7rem 0;
	}
	.acc > summary {
		list-style: none;
		cursor: pointer;
		font-weight: 700;
		padding: 0.5rem 0;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	/* hide the native disclosure triangle across engines and draw our own marker. */
	.acc > summary::-webkit-details-marker {
		display: none;
	}
	.acc > summary::before {
		content: '▸';
		display: inline-block;
		transition: transform 0.15s ease;
		color: var(--pi-meta);
	}
	.acc[open] > summary::before {
		transform: rotate(90deg);
	}
	.muted {
		color: var(--pi-meta);
		margin: 0.3rem 0 0.6rem;
	}
</style>
