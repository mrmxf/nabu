<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { handleAuthError } from '$lib/stores/auth.svelte';
	import { onMount } from 'svelte';
	import type { CourseRec, LessonRec, LessonScheduleRec, ScheduleSessionRec } from '$lib/types';

	// Schedule tab: pick a course + lesson, toggle whether students may name their own
	// session, and manage that lesson's cohort windows. Auth (staff) is enforced by
	// the dashboard layout; writes are additionally gated by the collection rules.

	interface EditRow {
		id: string; // '' until first saved
		sched_id: string;
		name: string;
		// datetime-local input strings (browser-local); '' = open-ended.
		enroll_from: string;
		enroll_to: string;
		start_from: string;
		start_to: string;
		saving: boolean;
		error: string;
	}

	let courses = $state<CourseRec[]>([]);
	let lessons = $state<LessonRec[]>([]);
	let courseId = $state('');
	let lessonId = $state('');
	let loadErr = $state('');

	let scheduleId = $state(''); // lesson_schedules record id ('' until created)
	let allowUnscheduled = $state(false);
	let allowSaving = $state(false);
	let rows = $state<EditRow[]>([]);
	let rowsLoading = $state(false);

	let classLessons = $derived(lessons.filter((l) => l.course === courseId));
	$effect(() => {
		if (courseId && !classLessons.some((l) => l.id === lessonId)) {
			lessonId = classLessons[0]?.id ?? '';
		}
	});

	onMount(async () => {
		try {
			[courses, lessons] = await Promise.all([
				pb.collection('courses').getFullList<CourseRec>({ sort: 'order' }),
				pb.collection('lessons').getFullList<LessonRec>({ sort: 'order' })
			]);
			if (courses.length) courseId = courses[0].id;
		} catch (err) {
			if (handleAuthError(err)) return;
			loadErr = err instanceof Error ? err.message : 'Could not load courses';
		}
	});

	// reload the schedule whenever the lesson changes.
	$effect(() => {
		const id = lessonId;
		if (id) loadForLesson(id);
		else resetSchedule();
	});

	function resetSchedule() {
		scheduleId = '';
		allowUnscheduled = false;
		rows = [];
	}

	async function loadForLesson(id: string) {
		rowsLoading = true;
		resetSchedule();
		try {
			const cfg = await pb
				.collection('lesson_schedules')
				.getFirstListItem<LessonScheduleRec>(`lesson = "${id}"`)
				.catch(() => null);
			if (cfg) {
				scheduleId = cfg.id;
				allowUnscheduled = cfg.allow_unscheduled;
				const list = await pb.collection('schedule_sessions').getFullList<ScheduleSessionRec>({
					filter: `lesson_schedule = "${cfg.id}"`,
					sort: 'start_from'
				});
				rows = list.map(toRow);
			}
		} catch (err) {
			if (handleAuthError(err)) return;
			loadErr = err instanceof Error ? err.message : 'Could not load schedule';
		} finally {
			rowsLoading = false;
		}
	}

	// ---- datetime conversions (PB UTC string <-> datetime-local input) --------
	const pad = (n: number) => String(n).padStart(2, '0');
	function toInput(s: string): string {
		if (!s) return '';
		const d = new Date(s.replace(' ', 'T'));
		if (isNaN(d.getTime())) return '';
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}
	function fromInput(v: string): string {
		if (!v) return '';
		const d = new Date(v); // interpreted as browser-local
		return isNaN(d.getTime()) ? '' : d.toISOString();
	}

	function toRow(s: ScheduleSessionRec): EditRow {
		return {
			id: s.id,
			sched_id: s.sched_id,
			name: s.name,
			enroll_from: toInput(s.enroll_from),
			enroll_to: toInput(s.enroll_to),
			start_from: toInput(s.start_from),
			start_to: toInput(s.start_to),
			saving: false,
			error: ''
		};
	}

	function slug(s: string): string {
		return s
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}
	// a stable id for a new session: slug of name + short suffix so two same-named
	// sessions don't collide on the (schedule, sched_id) unique index.
	function newSchedId(name: string): string {
		const base = slug(name) || 'session';
		return `${base}-${Math.random().toString(36).slice(2, 6)}`;
	}

	// create the lesson_schedules row on demand (first toggle or first session).
	async function ensureSchedule(): Promise<string> {
		if (scheduleId) return scheduleId;
		const rec = await pb.collection('lesson_schedules').create<LessonScheduleRec>({
			lesson: lessonId,
			allow_unscheduled: allowUnscheduled
		});
		scheduleId = rec.id;
		return scheduleId;
	}

	async function saveAllow() {
		allowSaving = true;
		try {
			const id = await ensureSchedule();
			await pb.collection('lesson_schedules').update(id, { allow_unscheduled: allowUnscheduled });
		} catch (err) {
			if (handleAuthError(err)) return;
			loadErr = err instanceof Error ? err.message : 'Could not save setting';
			allowUnscheduled = !allowUnscheduled; // revert the optimistic toggle
		} finally {
			allowSaving = false;
		}
	}

	function addRow() {
		rows = [
			...rows,
			{
				id: '',
				sched_id: '',
				name: '',
				enroll_from: '',
				enroll_to: '',
				start_from: '',
				start_to: '',
				saving: false,
				error: ''
			}
		];
	}

	async function saveRow(row: EditRow) {
		if (!row.name.trim()) {
			row.error = 'Name is required';
			return;
		}
		row.saving = true;
		row.error = '';
		try {
			const ls = await ensureSchedule();
			const data = {
				lesson_schedule: ls,
				sched_id: row.sched_id || newSchedId(row.name),
				name: row.name.trim(),
				enroll_from: fromInput(row.enroll_from),
				enroll_to: fromInput(row.enroll_to),
				start_from: fromInput(row.start_from),
				start_to: fromInput(row.start_to)
			};
			if (row.id) {
				await pb.collection('schedule_sessions').update(row.id, data);
			} else {
				const rec = await pb
					.collection('schedule_sessions')
					.create<ScheduleSessionRec>(data);
				row.id = rec.id;
				row.sched_id = rec.sched_id;
			}
		} catch (err) {
			if (handleAuthError(err)) return;
			row.error = err instanceof Error ? err.message : 'Could not save';
		} finally {
			row.saving = false;
		}
	}

	async function deleteRow(row: EditRow) {
		if (!row.id) {
			rows = rows.filter((r) => r !== row);
			return;
		}
		if (!confirm(`Delete session “${row.name}”? Students who already joined keep their run.`))
			return;
		row.saving = true;
		try {
			await pb.collection('schedule_sessions').delete(row.id);
			rows = rows.filter((r) => r !== row);
		} catch (err) {
			if (handleAuthError(err)) return;
			row.error = err instanceof Error ? err.message : 'Could not delete';
			row.saving = false;
		}
	}
</script>

<p class="muted">
	Set a lesson's cohort sessions and their enroll/start windows. Students pick from
	the sessions that are open for enrolment now.
</p>

{#if loadErr}<p class="error">{loadErr}</p>{/if}

<div class="pickers">
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
</div>

{#if lessonId}
	<label class="allow">
		<input
			type="checkbox"
			bind:checked={allowUnscheduled}
			onchange={saveAllow}
			disabled={allowSaving}
		/>
		Allow students to name their own session (unscheduled)
		{#if allowSaving}<span class="muted small">saving…</span>{/if}
	</label>

	<div class="sessions-head">
		<h3>Sessions</h3>
		<button class="pseudo" onclick={addRow}><i class="fa-solid fa-plus"></i> Add session</button>
	</div>

	{#if rowsLoading}
		<p class="muted">Loading sessions…</p>
	{:else if rows.length === 0}
		<p class="muted">
			No sessions yet. {allowUnscheduled
				? 'Students may name their own session.'
				: 'Add one so students can enrol.'}
		</p>
	{/if}

	{#each rows as row (row)}
		<div class="row-card">
			<div class="grid">
				<label class="name">
					Name
					<input type="text" bind:value={row.name} placeholder="e.g. Spring 2027 (testing)" />
				</label>
				<label>
					Enrol lesson opens
					<input type="datetime-local" bind:value={row.enroll_from} />
				</label>
				<label>
					Enrol lesson closes
					<input type="datetime-local" bind:value={row.enroll_to} />
				</label>
				<label>
					Start lesson at
					<input type="datetime-local" bind:value={row.start_from} />
				</label>
				<label>
					End lesson at
					<input type="datetime-local" bind:value={row.start_to} />
				</label>
			</div>
			<div class="row-actions">
				{#if row.sched_id}<code class="sched-id">id: {row.sched_id}</code>{/if}
				<span class="spacer"></span>
				{#if row.error}<span class="error small">{row.error}</span>{/if}
				<button onclick={() => saveRow(row)} disabled={row.saving}>
					{row.saving ? 'Saving…' : row.id ? 'Save' : 'Create'}
				</button>
				<button class="pseudo danger" onclick={() => deleteRow(row)} disabled={row.saving}>
					Delete
				</button>
			</div>
		</div>
	{/each}
{/if}

<style>
	.muted {
		color: var(--pi-meta);
	}
	.small {
		font-size: 0.85rem;
	}
	.error {
		color: var(--hi1);
	}
	.pickers {
		display: flex;
		gap: 1rem;
		flex-wrap: wrap;
		margin: 1rem 0;
	}
	.pickers label {
		font-weight: 600;
	}
	.allow {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.5rem 0 1rem;
		font-weight: 600;
	}
	.allow input {
		width: auto;
	}
	.sessions-head {
		display: flex;
		align-items: center;
		gap: 1rem;
	}
	.sessions-head h3 {
		margin: 0;
	}
	.row-card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.8rem 1rem;
		margin: 0.7rem 0;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 0.6rem;
	}
	.grid label {
		display: block;
		font-weight: 600;
		font-size: 0.85rem;
	}
	.grid input {
		width: 100%;
	}
	.name {
		grid-column: 1 / -1;
	}
	.row-actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		margin-top: 0.7rem;
	}
	.spacer {
		flex: 1;
	}
	.sched-id {
		font-size: 0.8rem;
		color: var(--pi-meta);
	}
	.danger:hover {
		color: var(--hi1);
	}
</style>
