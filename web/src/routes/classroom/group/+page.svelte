<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { auth, handleAuthError } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { onMount, onDestroy } from 'svelte';
	import { loadSettings, setting } from '$lib/stores/settings.svelte';
	import { renderTemplate } from '$lib/templating';
	import {
		loadClassroomStatus,
		classroomSessionId,
		type ClassroomSession
	} from '$lib/stores/classroom.svelte';
	import type { SessionRec, GroupRec } from '$lib/types';
	import { httpError } from '$lib/httpError';

	// The landing page for classroomFastLogin (see docs/classroom-fast-login.md).
	// Course, lesson and Session were all fixed by the sign-in, so — unlike the
	// full picker at routes/+page.svelte — this page asks exactly one question:
	// which group are you in? It reuses that page's group + attempt logic
	// verbatim; only the surrounding chrome is stripped away.

	let session = $state<ClassroomSession | null>(null);
	let loading = $state(true);
	let error = $state('');
	let starting = $state(false);

	// group formation for this (lesson, Session): an existing-groups dropdown plus
	// a "start a new group" dialog. Both are scoped to the Session, so two cohorts
	// running the same lesson never share a group.
	let groups = $state<GroupRec[]>([]);
	let groupsLoading = $state(false);
	let groupName = $state('');
	let defaultGroupName = $state('');
	let showNewGroup = $state(false);
	let newGroupName = $state('');
	let creatingGroup = $state(false);
	let groupError = $state('');

	// An attempt this student already has running for this (lesson, Session).
	// Offered as "Continue" so a re-login resumes rather than silently starting a
	// second attempt — in a classroom, a lost half-finished lesson is expensive.
	let existing = $state<SessionRec | null>(null);

	// Realtime subscription to `groups`, so a group a classmate starts appears in
	// everyone's dropdown at once. Without it the list is only as fresh as the
	// last page load, and students have to be told to reload — which in a
	// classroom reads as "the site is broken".
	let unsub: (() => void) | undefined;

	onMount(async () => {
		if (!auth.user) {
			goto('/login');
			return;
		}
		try {
			await loadSettings();
			// Re-read the status: the enrolment window may have closed between the
			// sign-in and now, and a restored tab may carry a stale session id.
			const status = await loadClassroomStatus(true);
			const wanted = classroomSessionId();
			session =
				status.sessions.find((s) => s.id === wanted) ??
				(status.sessions.length === 1 ? status.sessions[0] : null);
			if (!session) {
				goto('/login');
				return;
			}
			const user = auth.user.name || auth.user.uid || 'user';
			defaultGroupName = renderTemplate(setting('auto_group_template'), { user });
			await Promise.all([loadGroups(), loadExistingAttempt()]);
			await watchGroups();
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load your lesson';
		} finally {
			loading = false;
		}
	});

	onDestroy(() => unsub?.());

	// Keep the dropdown live. `groups` is readable by any signed-in user, so a
	// student can subscribe to it directly; events for other lessons/Sessions are
	// filtered out here rather than in the subscription so one connection serves
	// the page whatever the cohort. Realtime is a convenience — if the socket
	// fails the initial load still populated the list.
	async function watchGroups() {
		try {
			unsub = await pb.collection('groups').subscribe('*', (e) => {
				const rec = e.record as unknown as GroupRec;
				if (rec.lesson !== session?.lesson_id || rec.session !== session?.id) return;
				if (e.action === 'create' && !groups.some((g) => g.id === rec.id)) {
					groups = [...groups, rec].sort((a, b) => a.tag.localeCompare(b.tag));
				} else if (e.action === 'delete') {
					groups = groups.filter((g) => g.id !== rec.id);
					if (groupName === rec.tag) groupName = '';
				}
			});
		} catch {
			// realtime is optional; the list loaded above is still correct.
		}
	}

	// Groups belong to one (lesson, Session) — the same query the full picker runs.
	async function loadGroups() {
		if (!session) return;
		groupsLoading = true;
		groupError = '';
		try {
			groups = await pb.collection('groups').getFullList<GroupRec>({
				filter: `lesson = "${session.lesson_id}" && session = "${session.id}"`,
				sort: 'tag'
			});
			// keep the current pick only if it's still a group of this (lesson, session).
			if (groupName && !groups.some((g) => g.tag === groupName)) groupName = '';
		} catch (err) {
			if (handleAuthError(err)) return;
			groups = [];
		} finally {
			groupsLoading = false;
		}
	}

	async function loadExistingAttempt() {
		if (!auth.user || !session) return;
		try {
			existing = await pb
				.collection('sessions')
				.getFirstListItem<SessionRec>(
					`user = "${auth.user.id}" && lesson = "${session.lesson_id}" ` +
						`&& schedule_session = "${session.id}" && status = "active"`,
					{ sort: '-updated' }
				)
				.catch(() => null);
		} catch {
			// no running attempt is the normal case — this is a convenience only.
			existing = null;
		}
	}

	function openNewGroup() {
		groupError = '';
		newGroupName = defaultGroupName;
		showNewGroup = true;
	}

	// Create (or join, if it already exists) a group for this (lesson, Session) via
	// the server, which sets the quota; then select it. Idempotent on the
	// (lesson, session, tag) key, so "start a new group" and "join the one my
	// friend just made" converge on the same record.
	async function createGroup() {
		const tag = newGroupName.trim();
		if (!tag || !session) return;
		creatingGroup = true;
		groupError = '';
		try {
			const res = await fetch('/api/group/create', {
				method: 'POST',
				headers: { Authorization: pb.authStore.token, 'Content-Type': 'application/json' },
				body: JSON.stringify({ lesson_id: session.lesson_id, session_id: session.id, tag })
			});
			if (!res.ok) throw await httpError(res, 'Could not start the group');
			const g = (await res.json()) as {
				id: string;
				tag: string;
				lesson: string;
				session: string;
			};
			if (!groups.some((x) => x.tag === g.tag)) {
				groups = [
					...groups,
					{ id: g.id, tag: g.tag, lesson: g.lesson, session: g.session, paused: false }
				].sort((a, b) => a.tag.localeCompare(b.tag));
			}
			groupName = g.tag;
			showNewGroup = false;
			newGroupName = '';
		} catch (e) {
			groupError = e instanceof Error ? e.message : 'Could not start the group';
		} finally {
			creatingGroup = false;
		}
	}

	// Start the attempt. Same record shape the full picker writes, with the
	// course/lesson/Session taken from the fast login instead of three dropdowns.
	async function start(e: SubmitEvent) {
		e.preventDefault();
		if (!session || !auth.user || !groupName) return;
		starting = true;
		error = '';
		try {
			const rec = await pb.collection('sessions').create<SessionRec>({
				user: auth.user.id,
				course: session.course_id,
				lesson: session.lesson_id,
				current_slide: 0,
				session_name: session.name,
				schedule_session: session.id,
				group_name: groupName,
				work_state: {},
				status: 'active'
			});
			goto(`/session/${rec.id}/run`);
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not start the lesson';
			starting = false;
		}
	}
</script>

{#if loading}
	<p class="muted">Loading…</p>
{:else if session}
	<div class="card picker">
		<h2>Choose your group</h2>
		<p class="muted lesson-line">{session.lesson} — {session.name}</p>

		{#if existing}
			<div class="resume">
				<p>
					You already have this lesson running{existing.group_name
						? ` in group ${existing.group_name}`
						: ''}.
				</p>
				<a class="button" href={`/session/${existing.id}/run`}>Continue where you left off</a>
			</div>
			<p class="muted or">or start again in a group below</p>
		{/if}

		<form onsubmit={start}>
			<div class="field">
				<span class="field-label">
					Group <small>Required - this is a group exercise · updates live</small>
				</span>
				{#if groupsLoading}
					<p class="muted small">Loading groups…</p>
				{:else}
					<select bind:value={groupName}>
						{#if groups.length === 0}
							<option value="" disabled>No groups yet — start one below</option>
						{:else}
							{#if !groupName}<option value="" disabled>Join a group…</option>{/if}
							{#each groups as g (g.id)}
								<option value={g.tag}>{g.tag}</option>
							{/each}
						{/if}
					</select>
					<!-- No refresh button: the list is kept current by watchGroups(). -->
					<button type="button" class="pseudo new-group" onclick={openNewGroup}>
						＋ Start a new group
					</button>
				{/if}
			</div>

			{#if error}<p class="error">{error}</p>{/if}

			<button type="submit" disabled={starting || !groupName}>
				{starting ? 'Starting…' : 'Start the lesson'}
			</button>
		</form>
	</div>
{/if}

{#if showNewGroup}
	<div
		class="modal-overlay"
		role="button"
		tabindex="-1"
		onclick={(e) => e.target === e.currentTarget && !creatingGroup && (showNewGroup = false)}
		onkeydown={(e) => e.key === 'Escape' && !creatingGroup && (showNewGroup = false)}
	>
		<div class="modal" role="dialog" aria-modal="true" tabindex="-1">
			<label class="modal-title" for="new-group-input">Start a new group</label>
			<p class="muted small">Others can join it from this list. Everyone in a group shares its work.</p>
			<!-- svelte-ignore a11y_autofocus -->
			<input
				id="new-group-input"
				class="new-group-input"
				type="text"
				autocomplete="off"
				placeholder="e.g. s7g1"
				bind:value={newGroupName}
				disabled={creatingGroup}
				autofocus
				onkeydown={(e) => {
					if (e.key === 'Enter') createGroup();
				}}
			/>
			{#if groupError}<p class="error small">{groupError}</p>{/if}
			<div class="modal-actions">
				<button class="pseudo" onclick={() => (showNewGroup = false)} disabled={creatingGroup}>
					Cancel
				</button>
				<button onclick={createGroup} disabled={creatingGroup || !newGroupName.trim()}>
					{creatingGroup ? 'Starting…' : 'Start group'}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.picker {
		max-width: 480px;
		margin: 2rem auto;
	}
	.lesson-line {
		margin-top: -0.4rem;
	}
	.picker .field {
		display: block;
		font-weight: 600;
		margin-top: 0.9rem;
	}
	.field-label {
		display: block;
	}
	.field-label small {
		font-weight: 400;
		font-size: 0.8rem;
		color: var(--pi-meta);
	}
	.picker select {
		width: 100%;
	}
	.new-group {
		margin-top: 0.5rem;
		padding: 0.15rem 0;
		font-size: 0.9rem;
	}
	/* the "you already have this running" escape hatch */
	.resume {
		margin-top: 0.8rem;
		padding: 0.7rem 0.9rem;
		border-radius: 6px;
		background: oklch(from var(--hi2) 95% calc(c * 0.35) h);
	}
	.resume p {
		margin: 0 0 0.6rem;
	}
	.or {
		text-align: center;
		font-size: 0.85rem;
		margin: 0.8rem 0 0;
	}
	.picker button[type='submit'] {
		margin-top: 1.2rem;
		width: 100%;
	}
	.small {
		font-size: 0.8rem;
	}
	.error {
		color: var(--hi1);
	}
	.modal-overlay {
		position: fixed;
		inset: 0;
		background: oklch(0% 0 0 / 0.45);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: 1rem;
	}
	.modal {
		background: var(--pi-background, #fff);
		border-radius: 10px;
		padding: 1.2rem 1.4rem;
		max-width: 26rem;
		width: 100%;
		box-shadow: 0 10px 40px oklch(0% 0 0 / 0.3);
	}
	.modal-title {
		display: block;
		font-weight: 600;
		margin: 0 0 0.3rem;
	}
	.new-group-input {
		display: block;
		width: 100%;
		box-sizing: border-box;
		margin: 0.6rem 0 0.4rem;
	}
	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.6rem;
		margin-top: 0.8rem;
	}
</style>
