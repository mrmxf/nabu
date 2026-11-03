<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { auth, handleAuthError } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { loadSettings, setting, logoUrl } from '$lib/stores/settings.svelte';
	import { setCourseLogo, setLessonLogo } from '$lib/stores/branding.svelte';
	import { renderTemplate } from '$lib/templating';
	import type {
		CourseRec,
		LessonRec,
		SessionRec,
		LessonScheduleRec,
		ScheduleSessionRec,
		GroupRec
	} from '$lib/types';
	import { httpError } from '$lib/httpError';

	let courses = $state<CourseRec[]>([]);
	let lessons = $state<LessonRec[]>([]);
	let sessions = $state<SessionRec[]>([]);
	let loading = $state(true);
	let error = $state('');
	let starting = $state(false);

	// form state
	let courseId = $state('');
	let lessonId = $state('');
	let groupName = $state(''); // the group tag the student joins/creates (submitted)

	// group formation for the picked lesson: an existing-groups dropdown plus a
	// "start a new group" dialog. Groups are lesson-scoped.
	let groups = $state<GroupRec[]>([]);
	let groupsLoading = $state(false);
	let defaultGroupName = $state(''); // template value, prefilled into the new-group dialog
	let showNewGroup = $state(false);
	let newGroupName = $state('');
	let creatingGroup = $state(false);
	let groupError = $state('');

	// schedule state for the picked lesson (loaded reactively; see loadSchedule).
	// scheduleConfigured is false when the lesson has no lesson_schedules row — we
	// then behave as "unscheduled allowed" so lessons without a schedule still work.
	let scheduleConfigured = $state(false);
	let allowUnscheduled = $state(false);
	let scheduleSessions = $state<ScheduleSessionRec[]>([]);
	let scheduleLoading = $state(false);

	// the two possible session-name sources: a picked scheduled session (by id) or
	// a student-typed custom name. useCustom toggles between them when both apply.
	let selectedScheduleId = $state('');
	let customName = $state('');
	let useCustom = $state(false);

	let classLessons = $derived(lessons.filter((l) => l.course === courseId));

	onMount(async () => {
		if (!auth.user) {
			goto('/login');
			return;
		}
		try {
			await loadSettings();
			[courses, lessons, sessions] = await Promise.all([
				pb.collection('courses').getFullList<CourseRec>({ sort: 'order' }),
				pb.collection('lessons').getFullList<LessonRec>({ sort: 'order' }),
				pb.collection('sessions').getFullList<SessionRec>({
					filter: `user = "${auth.user.id}"`,
					sort: '-updated',
					expand: 'course,lesson,schedule_session'
				})
			]);
			if (courses.length) courseId = courses[0].id;
			if (classLessons.length) lessonId = classLessons[0].id;
			autofillCustomName();
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load courses';
		} finally {
			loading = false;
		}
	});

	// Pre-fill the custom (free-text) session name and the default new-group name
	// from the configured templates. USER is the student's name (or email local-part).
	// The group itself is joined/created below, so this only seeds the new-group dialog.
	function autofillCustomName() {
		const user = auth.user?.name || auth.user?.email?.split('@')[0] || 'user';
		customName = renderTemplate(setting('auto_session_template'), { user });
		defaultGroupName = renderTemplate(setting('auto_group_template'), { user });
	}

	$effect(() => {
		if (courseId && !classLessons.some((l) => l.id === lessonId)) {
			lessonId = classLessons[0]?.id ?? '';
		}
	});

	// Reflect the picked course in the navbar logo (no lesson context on this page).
	$effect(() => {
		setCourseLogo(courses.find((c) => c.id === courseId)?.logo);
		setLessonLogo('');
	});

	// Load the schedule for the picked lesson whenever it changes.
	$effect(() => {
		const id = lessonId;
		if (!id) {
			scheduleConfigured = false;
			allowUnscheduled = false;
			scheduleSessions = [];
			return;
		}
		loadSchedule(id);
	});

	async function loadGroups(lesson: string, session: string) {
		groupsLoading = true;
		groupError = '';
		try {
			groups = await pb.collection('groups').getFullList<GroupRec>({
				filter: `lesson = "${lesson}" && session = "${session}"`,
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

	function openNewGroup() {
		groupError = '';
		newGroupName = defaultGroupName;
		showNewGroup = true;
	}

	// Create (or join, if it already exists) a group for this (lesson, Session) via
	// the server, which sets the quota; then select it. Idempotent on the
	// (lesson, session, tag) key. A shared group requires a scheduled Session.
	async function createGroup() {
		const tag = newGroupName.trim();
		const session = sessionForGroup;
		if (!tag || !lessonId || !session) return;
		creatingGroup = true;
		groupError = '';
		try {
			const res = await fetch('/api/group/create', {
				method: 'POST',
				headers: { Authorization: pb.authStore.token, 'Content-Type': 'application/json' },
				body: JSON.stringify({ lesson_id: lessonId, session_id: session, tag })
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

	async function loadSchedule(id: string) {
		scheduleLoading = true;
		try {
			const cfg = await pb
				.collection('lesson_schedules')
				.getFirstListItem<LessonScheduleRec>(`lesson = "${id}"`)
				.catch(() => null);
			scheduleConfigured = !!cfg;
			allowUnscheduled = cfg?.allow_unscheduled ?? false;
			scheduleSessions = cfg
				? await pb.collection('schedule_sessions').getFullList<ScheduleSessionRec>({
						filter: `lesson_schedule = "${cfg.id}"`,
						sort: '-start_from'
					})
				: [];
			useCustom = false;
		} catch (err) {
			if (handleAuthError(err)) return;
			// a schedule read failure shouldn't strand the student: fall back to custom.
			scheduleConfigured = false;
			allowUnscheduled = false;
			scheduleSessions = [];
		} finally {
			scheduleLoading = false;
		}
	}

	// parse a PocketBase datetime ('2026-07-20 00:00:00.000Z') to a Date, or null.
	function toDate(s: string): Date | null {
		if (!s) return null;
		const d = new Date(s.replace(' ', 'T'));
		return isNaN(d.getTime()) ? null : d;
	}

	// a session is enrollable when NOW is within [enroll_from, enroll_to]; an empty
	// bound is open-ended on that side.
	function isEnrollable(s: ScheduleSessionRec): boolean {
		const now = Date.now();
		const from = toDate(s.enroll_from);
		const to = toDate(s.enroll_to);
		return (!from || now >= from.getTime()) && (!to || now <= to.getTime());
	}

	// enrollable sessions, newest-first (the fetch already sorts by -start_from).
	let enrollable = $derived(scheduleSessions.filter(isEnrollable));

	// effective gate: an unconfigured lesson behaves as "unscheduled allowed".
	let unscheduledOk = $derived(!scheduleConfigured || allowUnscheduled);

	// which session-name UI to show, from the gate + what's enrollable now.
	//   loading          — schedule still loading
	//   custom           — free-text only (unscheduled allowed, none enrollable)
	//   choice-or-custom — pick an enrollable session OR type a custom name
	//   single           — exactly one enrollable session, auto-used (no free text)
	//   dropdown         — several enrollable sessions to choose between
	//   blocked          — none enrollable and custom names not allowed
	let mode = $derived.by(() => {
		if (scheduleLoading) return 'loading';
		if (unscheduledOk) return enrollable.length > 0 ? 'choice-or-custom' : 'custom';
		if (enrollable.length === 0) return 'blocked';
		if (enrollable.length === 1) return 'single';
		return 'dropdown';
	});

	// keep the dropdown selection valid as the enrollable set changes.
	$effect(() => {
		if (enrollable.length && !enrollable.some((s) => s.id === selectedScheduleId)) {
			selectedScheduleId = enrollable[0].id;
		}
	});

	// the resolved name + schedule-session id to submit, and whether Start is allowed.
	let resolved = $derived.by(() => {
		const pick = () =>
			enrollable.find((s) => s.id === selectedScheduleId) ?? enrollable[0] ?? null;
		switch (mode) {
			case 'single': {
				const s = enrollable[0];
				return { name: s.name, scheduleId: s.id, canStart: true };
			}
			case 'dropdown': {
				const s = pick();
				return { name: s?.name ?? '', scheduleId: s?.id ?? '', canStart: !!s };
			}
			case 'choice-or-custom': {
				if (useCustom) return { name: customName, scheduleId: '', canStart: !!customName.trim() };
				const s = pick();
				return { name: s?.name ?? '', scheduleId: s?.id ?? '', canStart: !!s };
			}
			case 'custom':
				return { name: customName, scheduleId: '', canStart: !!customName.trim() };
			default: // loading | blocked
				return { name: '', scheduleId: '', canStart: false };
		}
	});

	// A shared group belongs to ONE (lesson, Session), so the group list is scoped
	// to the chosen Session (the schedule window). An unscheduled/custom run has no
	// Session → no shared groups → the student works solo. (Declared after
	// `resolved` because it reads resolved.scheduleId.)
	let sessionForGroup = $derived(resolved.scheduleId); // '' = unscheduled/solo
	let needsGroup = $derived(!!sessionForGroup);

	// Reload the group list whenever the picked lesson OR Session changes.
	$effect(() => {
		const lesson = lessonId;
		const session = sessionForGroup;
		if (lesson && session) loadGroups(lesson, session);
		else {
			groups = [];
			groupName = ''; // solo: no group tag
		}
	});

	async function startSession(e: SubmitEvent) {
		e.preventDefault();
		if (!lessonId || !auth.user || !resolved.canStart) return;
		starting = true;
		error = '';
		try {
			const rec = await pb.collection('sessions').create<SessionRec>({
				user: auth.user.id,
				course: courseId,
				lesson: lessonId,
				current_slide: 0,
				session_name: resolved.name,
				schedule_session: resolved.scheduleId,
				group_name: groupName,
				work_state: {},
				status: 'active'
			});
			// new session: names already captured here, go straight into the lesson.
			goto(`/session/${rec.id}/run`);
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not start session';
			starting = false;
		}
	}

	// an attempt's Session display name follows the scheduled session's current name
	// (renames propagate), falling back to the stored session_name.
	const sessionLabel = (s: SessionRec) =>
		s.expand?.schedule_session?.name || s.session_name || '—';
	const lessonTitle = (s: SessionRec) => s.expand?.lesson?.title ?? 'Lesson';
	const courseTitle = (s: SessionRec) => s.expand?.course?.title ?? '';
	// the lesson's stable id (params.id, else its folder slug) — distinguishes
	// otherwise same-named lessons across parallel course runs.
	const lessonSlug = (s: SessionRec) => s.expand?.lesson?.slug ?? '';

	// windowClosed: an attempt is read-only once its Session's active window has
	// ended (now > start_to). Unscheduled attempts, or Sessions with no start_to,
	// stay open. (Frontend affordance only; server-side lock is a later step.)
	const windowClosed = (s: SessionRec): boolean => {
		const end = toDate(s.expand?.schedule_session?.start_to ?? '');
		return !!end && Date.now() > end.getTime();
	};

	// attemptNo numbers a student's attempts within each (lesson, Session) in
	// start order, so two runs of the same offering are distinguishable (#1, #2…).
	let attemptNo = $derived.by(() => {
		const key = (s: SessionRec) => `${s.lesson}::${s.schedule_session || s.session_name}`;
		const byKey: Record<string, SessionRec[]> = {};
		for (const s of sessions) (byKey[key(s)] ??= []).push(s);
		const out: Record<string, number> = {};
		for (const k in byKey) {
			byKey[k]
				.slice()
				.sort((a, b) => (a.created ?? '').localeCompare(b.created ?? ''))
				.forEach((s, i) => (out[s.id] = i + 1));
		}
		return out;
	});
</script>

{#if loading}
	<p class="muted">Loading…</p>
{:else}
	<div class="card picker">
		{#if logoUrl('course_logo')}
			<img
				class="logo"
				src={logoUrl('course_logo')}
				alt=""
				onerror={(e) => ((e.currentTarget as HTMLImageElement).style.display = 'none')}
			/>
		{/if}
		<h2>{sessions.length === 0 ? 'Enroll in a course' : 'Start a New Attempt'}</h2>

		<form onsubmit={startSession}>
			<label>
				Course
				<select bind:value={courseId}>
					{#each courses as c (c.id)}
						<option value={c.id}>{c.title}</option>
					{/each}
				</select>
			</label>

			<label>
				Lesson
				<select bind:value={lessonId}>
					{#each classLessons as l (l.id)}
						<option value={l.id}>{l.title}</option>
					{/each}
				</select>
			</label>

			<!-- Session name: sourced from the lesson's schedule (by id). -->
			<div class="field">
				<span class="field-label">Session</span>
				{#if mode === 'loading'}
					<p class="muted small">Loading sessions…</p>
				{:else if mode === 'blocked'}
					<p class="blocked">No open sessions for this lesson yet — ask your teacher.</p>
				{:else if mode === 'single'}
					<p class="chosen">{enrollable[0].name}</p>
				{:else if mode === 'dropdown'}
					<select bind:value={selectedScheduleId}>
						{#each enrollable as s (s.id)}
							<option value={s.id}>{s.name}</option>
						{/each}
					</select>
				{:else if mode === 'custom'}
					<input type="text" bind:value={customName} placeholder="e.g. Morning newsroom" />
				{:else if mode === 'choice-or-custom'}
					<select bind:value={selectedScheduleId} disabled={useCustom}>
						{#each enrollable as s (s.id)}
							<option value={s.id}>{s.name}</option>
						{/each}
					</select>
					<label class="custom-toggle">
						<input type="checkbox" bind:checked={useCustom} />
						Name my own session
					</label>
					{#if useCustom}
						<input type="text" bind:value={customName} placeholder="e.g. Morning newsroom" />
					{/if}
				{/if}
			</div>

			<!-- Group: join/start a group in THIS session, or work solo when the run
			     is unscheduled (a shared group belongs to one scheduled session). -->
			<div class="field">
				<span class="field-label">Group <small>Required - this is a group exercise</small></span>
				{#if !needsGroup}
					<p class="muted small">Unscheduled — you'll work solo (no shared group).</p>
				{:else if groupsLoading}
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
					<button type="button" class="pseudo new-group" onclick={openNewGroup}>
						＋ Start a new group
					</button>
				{/if}
			</div>

			{#if error}<p class="error">{error}</p>{/if}

			<button
				type="submit"
				disabled={starting || !lessonId || !resolved.canStart || (needsGroup && !groupName)}
			>
				{starting ? 'Starting…' : 'Start attempt'}
			</button>
		</form>
	</div>

	<div class="card sessions">
		<h3>Your attempts</h3>
		{#if sessions.length === 0}
			<p class="muted">No attempts yet — start one above.</p>
		{:else}
			<table>
				<thead>
					<tr>
						<th>Lesson</th><th>Session</th><th>Attempt</th><th>Group</th><th>Status</th><th></th>
					</tr>
				</thead>
				<tbody>
					{#each sessions as s (s.id)}
						<tr>
							<td>
								{lessonTitle(s)}
								<div class="muted small">
									{courseTitle(s)}
									{#if lessonSlug(s)}· <code class="lesson-id">{lessonSlug(s)}</code>{/if}
								</div>
							</td>
							<td>{sessionLabel(s)}</td>
							<td>#{attemptNo[s.id] ?? 1}</td>
							<td>{s.group_name || 'solo'}</td>
							<td><span class="tag" class:done={s.status === 'complete'}>{s.status}</span></td>
							<td class="right">
								{#if windowClosed(s)}
									<a class="button view-closed" href={`/session/${s.id}`}>View (closed)</a>
								{:else}
									<a class="button" href={`/session/${s.id}`}>Continue</a>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
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
	/* Match the lesson-content reading column rather than a cramped dialog. */
	.picker {
		max-width: 720px;
		margin: 0 auto 1rem;
	}
	.logo {
		max-height: 48px;
		margin-bottom: 0.6rem;
	}
	.picker label,
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
	.picker select,
	.picker input[type='text'] {
		width: 100%;
	}
	.custom-toggle {
		margin-top: 0.5rem;
		font-weight: 400;
		font-size: 0.9rem;
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.custom-toggle input {
		width: auto;
	}
	.chosen {
		margin: 0.3rem 0 0;
		font-weight: 600;
	}
	.blocked {
		margin: 0.3rem 0 0;
		font-weight: 400;
		color: var(--hi1);
	}
	.picker button[type='submit'] {
		margin-top: 1.2rem;
		width: 100%;
	}
	.sessions {
		/* Full lesson width: a slide renders as a plain .card in the shell with no
		   reading-column cap, so the attempts table fills the same width. */
		margin: 0;
	}
	table {
		width: 100%;
	}
	.right {
		text-align: right;
	}
	.small {
		font-size: 0.8rem;
	}
	.lesson-id {
		font-family: var(--pi-mono, monospace);
		font-size: 0.75rem;
		background: oklch(from var(--hi2) 94% calc(c * 0.3) h);
		padding: 0.05rem 0.35rem;
		border-radius: 4px;
	}
	.tag {
		background: oklch(from var(--hi2) 94% calc(c * 0.4) h);
		color: var(--pi-body);
		padding: 0.1rem 0.5rem;
		border-radius: 10px;
		font-size: 0.8rem;
	}
	.tag.done {
		background: oklch(from var(--hi4) 93% calc(c * 0.5) h);
		color: oklch(from var(--hi4) 42% c h);
	}
	/* a closed session's attempt is view-only: muted, non-primary affordance. */
	.view-closed {
		background: transparent;
		color: var(--pi-meta);
		border: 1px solid oklch(from var(--hi2) 85% calc(c * 0.3) h);
	}
	.error {
		color: var(--hi1);
	}
	.new-group {
		margin-top: 0.5rem;
		padding: 0.15rem 0;
		font-size: 0.9rem;
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
