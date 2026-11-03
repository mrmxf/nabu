<script lang="ts">
	import { auth, isStaff } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import TeacherNotifications from '$lib/teacher/TeacherNotifications.svelte';

	// The teacher's home. Deliberately calm: anything urgent is in the notifications
	// panel at the top, the dashboard is one click away, and the rest of the page is
	// orientation for someone running a lesson for the first time.
	//
	// Help lives in the existing /nabu/help tree (audience: teacher) rather than a
	// separate markdown file, so there is one place to write teacher docs and it
	// ships and versions with the rest of the help content.
	onMount(() => {
		if (!auth.user) goto('/login');
		else if (!isStaff()) goto('/');
	});
</script>

<h1><i class="fa-solid fa-chalkboard-user"></i> Teacher</h1>

<TeacherNotifications />

<div class="actions">
	<a class="button" href="/teacher/dashboard/queues">
		<i class="fa-solid fa-gauge-high"></i> Dashboard
	</a>
	<a class="button pseudo" href="/nabu/help" target="_blank" rel="noopener">
		<i class="fa-solid fa-circle-question"></i> Help
	</a>
</div>

<section class="card help">
	<h2>Running a lesson</h2>
	<dl>
		<dt><i class="fa-solid fa-users-between-lines"></i> Queues</dt>
		<dd>
			Live view of jobs in flight for each group. If work seems stuck, look here first — a
			full queue means students are submitting faster than the agent server can answer.
		</dd>

		<dt><i class="fa-solid fa-upload"></i> Resources</dt>
		<dd>
			Upload and browse the files attached to a lesson. Pick the course and lesson first;
			uploads land in that lesson's own store.
		</dd>

		<dt><i class="fa-solid fa-bell"></i> Notifications</dt>
		<dd>
			Checked automatically every few seconds. <strong>The agent server is not
			responding</strong> is the one to act on — until it clears, students' work will queue up
			and never run.
		</dd>
	</dl>

	<p class="muted small">
		Full documentation is in <a href="/nabu/help" target="_blank" rel="noopener">Nabu help</a>.
	</p>
</section>

<style>
	.actions {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
		margin: 1rem 0;
	}
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	.help h2 {
		margin-top: 0;
		font-size: 1.1rem;
	}
	dl {
		margin: 0;
	}
	dt {
		font-weight: 600;
		margin-top: 0.8rem;
	}
	dd {
		margin: 0.2rem 0 0;
		color: var(--pi-meta);
	}
	.muted {
		color: var(--pi-meta);
	}
	.small {
		font-size: 0.85rem;
	}
</style>
