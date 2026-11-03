<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { auth, handleAuthError } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { logoUrl, loadSettings } from '$lib/stores/settings.svelte';
	import type { SessionRec, GroupRec } from '$lib/types';

	// Landing page shown when *continuing* a session: the session name is fixed
	// (set at start / by the schedule) and shown read-only; the student confirms or
	// changes which group they're in before resuming where they left off.
	const sessionId = $page.params.id ?? '';

	let session = $state<SessionRec | null>(null);
	let groups = $state<GroupRec[]>([]);
	let groupName = $state('');
	let loading = $state(true);
	let error = $state('');
	let busy = $state(false);

	// The session's display name: the scheduled cohort's current name (renames
	// propagate — references are by id), else the stored session_name.
	let sessionLabel = $derived(
		session?.expand?.schedule_session?.name || session?.session_name || '—'
	);

	// Group tags available in this session's course, plus the student's current group
	// if it isn't one of them (so an existing free-text group is never dropped).
	let groupOptions = $derived.by(() => {
		const tags = groups.map((g) => g.tag);
		return groupName && !tags.includes(groupName) ? [groupName, ...tags] : tags;
	});

	onMount(async () => {
		if (!auth.user) {
			goto('/login');
			return;
		}
		loadSettings();
		try {
			session = await pb.collection('sessions').getOne<SessionRec>(sessionId, {
				expand: 'course,lesson,schedule_session'
			});
			groupName = session.group_name;
			// Groups available in THIS attempt's (lesson, Session) — a group belongs to
			// one Session (see docs/glossary.md). An unscheduled attempt (no Session)
			// has no shared groups and is solo.
			if (session.schedule_session) {
				groups = await pb.collection('groups').getFullList<GroupRec>({
					filter: `lesson = "${session.lesson}" && session = "${session.schedule_session}"`,
					sort: 'tag'
				});
			} else {
				groups = [];
			}
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load session';
		} finally {
			loading = false;
		}
	});

	async function resume(e: SubmitEvent) {
		e.preventDefault();
		if (!session) return;
		busy = true;
		error = '';
		try {
			// session name is fixed now; only the group can change here.
			await pb.collection('sessions').update(session.id, { group_name: groupName });
			goto(`/session/${session.id}/run`);
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not update session';
			busy = false;
		}
	}
</script>

{#if loading}
	<p class="muted">Loading…</p>
{:else if error}
	<div class="card"><p class="error">{error}</p><a href="/">← Back to dashboard</a></div>
{:else if session}
	<div class="card picker">
		{#if logoUrl('lesson_logo')}
			<img
				class="logo"
				src={logoUrl('lesson_logo')}
				alt=""
				onerror={(e) => ((e.currentTarget as HTMLImageElement).style.display = 'none')}
			/>
		{/if}
		<h2>Continue your attempt</h2>
		<p class="muted">
			{session.expand?.lesson?.title ?? 'Lesson'}
			{#if session.expand?.course}· {session.expand.course.title}{/if}
			· resuming at slide {(session.current_slide ?? 0) + 1}
		</p>

		<form onsubmit={resume}>
			<div class="field">
				<span class="field-label">Session</span>
				<p class="session-name">{sessionLabel}</p>
			</div>

			{#if !session.schedule_session}
				<div class="field">
					<span class="field-label">Group</span>
					<p class="session-name">Unscheduled — you're working solo.</p>
				</div>
			{:else}
				<label>
					You are in group
					<select bind:value={groupName}>
						{#if groupOptions.length === 0}
							<option value="" disabled>No groups yet — ask your teacher</option>
						{:else}
							{#if !groupName}<option value="" disabled selected>Choose your group…</option>{/if}
							{#each groupOptions as tag (tag)}
								<option value={tag}>{tag}</option>
							{/each}
						{/if}
					</select>
				</label>
			{/if}

			{#if error}<p class="error">{error}</p>{/if}

			<div class="row actions">
				<a class="pseudo button" href="/">← Dashboard</a>
				<span class="spacer"></span>
				<button type="submit" disabled={busy}>{busy ? 'Resuming…' : 'Continue →'}</button>
			</div>
		</form>
	</div>
{/if}

<style>
	.picker {
		max-width: 460px;
		margin: 1.5rem auto;
	}
	.logo {
		max-height: 48px;
		margin-bottom: 0.6rem;
	}
	label,
	.field {
		display: block;
		font-weight: 600;
		margin-top: 0.8rem;
	}
	.field-label {
		display: block;
	}
	.session-name {
		margin: 0.2rem 0 0;
		font-weight: 400;
	}
	select {
		width: 100%;
	}
	.actions {
		margin-top: 1.2rem;
		align-items: center;
	}
	.error {
		color: var(--hi1);
	}
</style>
