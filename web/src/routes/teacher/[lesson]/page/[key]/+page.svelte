<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { auth, isStaff, handleAuthError } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount, onDestroy } from 'svelte';
	import {
		setCourseLogo,
		setLessonLogo,
		setCourseCss,
		setLessonCss
	} from '$lib/stores/branding.svelte';
	import {
		choiceValues,
		commitChoice,
		bindChoices,
		setBuiltins,
		resetChoiceValues,
		resolvePick
	} from '$lib/stores/decisions.svelte';
	import { resetDataValues, fetchSource, computeMapper } from '$lib/stores/dataMappers.svelte';
	import { createGroupDoc, noteText, applyTextDiff, type GroupDoc } from '$lib/yjs/pbProvider';
	import type {
		SlideRec,
		TeacherPageRec,
		LessonRec,
		ChoiceGroup,
		DataMapper,
		LessonScheduleRec,
		ScheduleSessionRec,
		GroupRec
	} from '$lib/types';
	import Slide from '$lib/components/Slide.svelte';

	// A teacher-only companion page for a lesson, rendered through the same <Slide>
	// player as a student slide. To see a group's LIVE shared state (choices, notes,
	// agent configs) the teacher enrols into a group exactly like a student — the Yjs
	// room is derived from the caller's own attempt, so a teacher attempt carrying a
	// group's tag + cohort shares that group's room. A selector lets the teacher jump
	// between groups to assist remotely; switching re-keys the shared document.

	const lessonSlug = $derived($page.params.lesson ?? '');
	const pageKey = $derived($page.params.key ?? '');

	let lesson = $state<LessonRec | null>(null);
	let teacherPage = $state<TeacherPageRec | null>(null);
	let slides = $state<SlideRec[]>([]); // the lesson's student slides (for choice/mapper context)
	let loading = $state(true);
	let error = $state('');

	// group enrolment: a flat list of "cohort · group" options across the lesson's
	// scheduled sessions. `selected` encodes "<scheduleSessionId>::<groupTag>".
	interface GroupOption {
		value: string;
		label: string;
		sessionId: string;
		tag: string;
	}
	let groupOptions = $state<GroupOption[]>([]);
	let selected = $state('');
	let switching = $state(false);

	// the teacher's own attempt (a `sessions` row) — the authorisation anchor for the
	// group's Yjs room — plus the live document it opens.
	let attemptId = $state('');
	let gdoc = $state<GroupDoc | undefined>(undefined);
	let notes = $state<Record<string, string>>({});

	let selectedOption = $derived(groupOptions.find((o) => o.value === selected));

	// Lesson-wide choice groups by id (student slides + this page's own t_choice), so a
	// {{< LMS t="print" from="…" >}} on the teacher page resolves a group's chosen value
	// and its default_value_key.
	let groupsById = $derived.by(() => {
		const m: Record<string, ChoiceGroup> = {};
		for (const s of slides) for (const g of s.t_choice ?? []) m[g.id] = g;
		for (const g of teacherPage?.t_choice ?? []) m[g.id] = g;
		return m;
	});

	// lesson-wide t_mapper specs (student slides + this page); derived tables recompute
	// reactively from the fetched data + the group's choice values.
	let mapperSpecs = $derived.by(() => {
		const out: DataMapper[] = [];
		for (const s of slides) for (const m of s.t_mapper ?? []) out.push(m);
		for (const m of teacherPage?.t_mapper ?? []) out.push(m);
		return out;
	});
	$effect(() => {
		for (const m of mapperSpecs) computeMapper(m);
	});

	const defaultKeyFor = (from: string) => groupsById[from]?.default_value_key || 'label';

	// Adapt the teacher page to the SlideRec shape the <Slide> player expects (it uses
	// `slug` as the work_state key and hydration id — the page key serves that role).
	let slideRec = $derived<SlideRec | null>(
		teacherPage
			? ({ ...teacherPage, slug: teacherPage.key, order: 0, number: '' } as unknown as SlideRec)
			: null
	);

	function onChoose(id: string, key: string) {
		const g = groupsById[id];
		if (!g) return;
		commitChoice(id, resolvePick(g, key)); // group-scoped picks land in the shared Yjs map
	}

	function syncNotes() {
		if (!gdoc) return;
		const next: Record<string, string> = {};
		for (const [slideId, text] of gdoc.notes.entries()) next[slideId] = text.toString();
		notes = next;
	}
	function onNote(slideId: string, value: string) {
		if (!gdoc) return;
		applyTextDiff(noteText(gdoc, slideId), value);
	}

	// seed the shared choice store: built-ins for this group + bind the group's Yjs
	// `choices` map so group-scoped picks appear (and update) live.
	function seedRuntime(tag: string) {
		resetChoiceValues();
		setBuiltins({
			username: auth.user?.name || auth.user?.email || 'teacher',
			email: auth.user?.email ?? '',
			group: tag,
			session: selectedOption?.label ?? ''
		});
		if (gdoc) bindChoices(gdoc.choices);
	}

	// Open (or re-open) the group's collaborative document for the current selection.
	// The teacher's attempt is created/updated to carry the chosen cohort + group tag,
	// which is what keys the room; then the doc is bound and the runtime seeded.
	async function openGroup(opt: GroupOption) {
		switching = true;
		error = '';
		try {
			// tear down any doc for the previous group before switching rooms.
			resetChoiceValues();
			gdoc?.destroy();
			gdoc = undefined;

			if (!attemptId) {
				// reuse an existing teacher attempt for this lesson if present, else create.
				const existing = await pb
					.collection('sessions')
					.getFirstListItem(`user = "${auth.user?.id}" && lesson = "${lesson!.id}"`)
					.catch(() => null);
				if (existing) attemptId = existing.id;
			}
			const data = {
				user: auth.user?.id,
				course: lesson!.course,
				lesson: lesson!.id,
				schedule_session: opt.sessionId,
				group_name: opt.tag,
				session_name: opt.label
			};
			if (attemptId) await pb.collection('sessions').update(attemptId, data);
			else
				attemptId = (
					await pb.collection('sessions').create({ ...data, current_slide: 0, work_state: {}, status: 'active' })
				).id;

			gdoc = createGroupDoc(attemptId);
			gdoc.notes.observeDeep(syncNotes);
			gdoc.ready.then(syncNotes);
			seedRuntime(opt.tag);
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not open the group';
		} finally {
			switching = false;
		}
	}

	function onSelect() {
		const opt = groupOptions.find((o) => o.value === selected);
		if (opt) openGroup(opt);
	}

	onMount(async () => {
		if (!auth.user) {
			goto('/login');
			return;
		}
		if (!isStaff()) {
			goto('/');
			return;
		}
		try {
			lesson = await pb
				.collection('lessons')
				.getFirstListItem<LessonRec>(`slug = "${lessonSlug}"`, { expand: 'course' });
			setLessonLogo(lesson.logo);
			setCourseLogo(lesson.expand?.course?.logo);
			// authored styling for the page body: course theme, then lesson override.
			setCourseCss(lesson.expand?.course?.css);
			setLessonCss(lesson.css);

			[teacherPage, slides] = await Promise.all([
				pb
					.collection('teacher_pages')
					.getFirstListItem<TeacherPageRec>(`lesson = "${lesson.id}" && key = "${pageKey}"`),
				pb
					.collection('slides')
					.getFullList<SlideRec>({ filter: `lesson = "${lesson.id}"`, sort: 'order' })
			]);
			for (const src of new Set(mapperSpecs.map((m) => m.source))) fetchSource(src);

			await loadGroupOptions(lesson.id);
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load the teacher page';
		} finally {
			loading = false;
		}
	});

	// Build the "cohort · group" options: every group of every scheduled session for
	// this lesson. A teacher can only join a shared (scheduled) group's room.
	async function loadGroupOptions(lessonId: string) {
		const cfg = await pb
			.collection('lesson_schedules')
			.getFirstListItem<LessonScheduleRec>(`lesson = "${lessonId}"`)
			.catch(() => null);
		if (!cfg) return;
		const sessions = await pb.collection('schedule_sessions').getFullList<ScheduleSessionRec>({
			filter: `lesson_schedule = "${cfg.id}"`,
			sort: '-start_from'
		});
		const opts: GroupOption[] = [];
		for (const s of sessions) {
			const groups = await pb.collection('groups').getFullList<GroupRec>({
				filter: `lesson = "${lessonId}" && session = "${s.id}"`,
				sort: 'tag'
			});
			for (const g of groups) {
				opts.push({
					value: `${s.id}::${g.tag}`,
					label: `${s.name} · ${g.tag}`,
					sessionId: s.id,
					tag: g.tag
				});
			}
		}
		groupOptions = opts;
	}

	onDestroy(() => {
		resetChoiceValues();
		gdoc?.destroy();
		resetDataValues();
	});
</script>

{#if loading}
	<p class="muted">Loading teacher page…</p>
{:else if error}
	<div class="card"><p class="error">{error}</p><a href="/teacher/dashboard/queues">← Dashboard</a></div>
{:else if teacherPage}
	<div class="head">
		<div>
			<a class="back" href="/teacher/dashboard/queues"><i class="fa-solid fa-arrow-left"></i> Dashboard</a>
			<p class="muted small">
				Teacher page · {lesson?.title ?? ''} · <code>{teacherPage.key}</code>
			</p>
		</div>
		<label class="picker">
			Assisting group
			<select bind:value={selected} onchange={onSelect} disabled={switching}>
				{#if groupOptions.length === 0}
					<option value="" disabled selected>No groups yet — none to assist</option>
				{:else}
					<option value="" disabled selected>Choose a group…</option>
					{#each groupOptions as o (o.value)}
						<option value={o.value}>{o.label}</option>
					{/each}
				{/if}
			</select>
			{#if switching}<span class="muted small">opening…</span>{/if}
		</label>
	</div>

	{#if !selected}
		<div class="card notice">
			<p>
				Pick a group above to follow this lesson alongside them. The page's live values
				(each group's shared choices and notes) bind to whichever group you're assisting.
			</p>
		</div>
	{:else if slideRec && gdoc}
		<!-- mount only once the group's document is open, so the widgets hydrate WITH
		     the shared doc; re-key on the selection so switching groups re-hydrates. -->
		{#key selected}
			<Slide
				slide={slideRec}
				{onChoose}
				{defaultKeyFor}
				noteValue={notes[slideRec.slug] ?? ''}
				onNote={(v) => onNote(slideRec.slug, v)}
				groupTag={selectedOption?.tag ?? ''}
				sessionId={attemptId}
				lessonId={lesson?.id ?? ''}
				groupDoc={gdoc}
			/>
		{/key}
	{:else if selected}
		<p class="muted">Opening group…</p>
	{/if}
{/if}

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1rem;
	}
	.head p {
		margin: 0.2rem 0 0;
	}
	.back {
		font-size: 0.9rem;
		color: var(--pi-meta);
		text-decoration: none;
	}
	.back:hover {
		color: var(--pi-body);
	}
	.picker {
		font-weight: 600;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		min-width: 240px;
	}
	.picker select {
		font-weight: 400;
	}
	.notice {
		border: 1px dashed oklch(from var(--hi2) 85% calc(c * 0.4) h);
	}
	.muted {
		color: var(--pi-meta);
	}
	.small {
		font-size: 0.85rem;
	}
	.error {
		color: var(--hi1);
	}
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
</style>
