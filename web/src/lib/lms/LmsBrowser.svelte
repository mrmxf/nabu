<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { httpError } from '$lib/httpError';
	import { isStaff } from '$lib/stores/auth.svelte';
	import ResourceUploader from '$lib/components/ResourceUploader.svelte';
	import Tabs from '$lib/components/Tabs.svelte';

	// Tabbed file browser over a lesson's resource store, mounted by the LMS
	// `browser` widget and reused on the teacher page. Each tab is a scope:
	//   resources — course files (read-only for students, managed by staff)
	//   group     — the student's group's shared work
	//   student   — the student's own work
	// It reads the fast PocketBase index via the staff (/api/resources) or student
	// (/api/student/resources) routes depending on the viewer, so students never
	// touch course-only endpoints. All uploads reuse ResourceUploader.
	type Scope = 'resources' | 'group' | 'student';
	const KNOWN: Scope[] = ['resources', 'group', 'student'];
	const LABEL: Record<Scope, string> = {
		resources: 'Course resources',
		group: 'Group work',
		student: 'My work'
	};

	let {
		tabs = 'resources',
		variant = 'full',
		showSize = true,
		lessonId = '',
		sessionId = '',
		disabled = false,
		onChanged
	}: {
		tabs?: string;
		// `full` = the managed tabbed browser; `list` = a compact, read-only
		// two-column list (size + name link) for embedding (e.g. inside a post-it).
		variant?: 'full' | 'list';
		// Show the size column (column 1). The narrow-screen escape hatch: set false
		// to drop it in either variant.
		showSize?: boolean;
		lessonId?: string;
		sessionId?: string;
		disabled?: boolean;
		onChanged?: () => void;
	} = $props();

	const staff = isStaff();
	const scopes = $derived(
		tabs
			.split(/\s+/)
			.map((s) => s.trim())
			.filter((s): s is Scope => (KNOWN as string[]).includes(s))
	);
	let active = $state<Scope>('resources');
	$effect(() => {
		if (scopes.length && !scopes.includes(active)) active = scopes[0];
	});

	// Endpoint base + body for the active scope, chosen by viewer role. Staff drive
	// the course routes (lesson-keyed); students drive the session-keyed routes.
	const base = $derived(staff ? '/api/resources' : '/api/student/resources');
	const body = $derived<Record<string, unknown>>(
		staff ? { lesson_id: lessonId } : { session_id: sessionId, scope: active }
	);
	// The `list` variant shows the COURSE-GLOBAL resources folder (bucket-root
	// `resources/`, keyed only by the lesson's bucket), served to any authed user
	// by one endpoint. The `full` browser keeps its role-split, lesson/session
	// scoped routes.
	const listUrl = $derived(
		variant === 'list'
			? `/api/resources/global?lesson_id=${encodeURIComponent(lessonId)}`
			: staff
				? `/api/resources?lesson_id=${encodeURIComponent(lessonId)}&scope=${active}`
				: `/api/student/resources?session_id=${encodeURIComponent(sessionId)}&scope=${active}`
	);
	// Who may write/delete in the active tab: staff manage course resources; students
	// manage their group + own work (course resources are read-only to them).
	const canWrite = $derived(!disabled && (staff ? active === 'resources' : active !== 'resources'));

	interface Obj {
		key: string;
		size: number;
		last_modified: string;
		public_url: string;
	}
	let objects = $state<Obj[]>([]);
	let loading = $state(false);
	let error = $state('');
	let copied = $state('');
	let pendingDelete = $state<Obj | null>(null); // row awaiting a delete confirm
	let deleting = $state(false);
	let renaming = $state<Obj | null>(null); // row being renamed in the modal
	let renameValue = $state('');
	let renameBusy = $state(false);
	let refreshKey = $state(0);

	// Narrow layout (teacher tool below 800px): the table collapses to just the
	// file name plus a per-row ellipsis that opens `details` — a modal listing the
	// other columns as full-width rows with the row's actions beneath. Default false
	// (wide) so SSR/first paint shows the full table.
	let narrow = $state(false);
	let details = $state<Obj | null>(null);
	$effect(() => {
		const mq = window.matchMedia('(max-width: 799px)');
		const sync = () => (narrow = mq.matches);
		sync();
		mq.addEventListener('change', sync);
		return () => mq.removeEventListener('change', sync);
	});

	// Column sort: click a heading to sort by it, click again to invert.
	type SortKey = 'name' | 'size' | 'modified';
	let sortKey = $state<SortKey>('name');
	let sortDir = $state<1 | -1>(1);

	const authHeaders = () => ({ Authorization: pb.authStore.token });

	async function load() {
		const missingKey =
			variant === 'list' ? !lessonId : (staff && !lessonId) || (!staff && !sessionId);
		if (missingKey) {
			objects = [];
			return;
		}
		loading = true;
		error = '';
		try {
			const res = await fetch(listUrl, { headers: authHeaders() });
			if (!res.ok) throw await httpError(res, 'Could not list resources');
			objects = (await res.json()).objects ?? [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not list resources';
			objects = [];
		} finally {
			loading = false;
		}
	}

	// reload when the tab (or a completed upload) changes.
	$effect(() => {
		void active;
		void refreshKey;
		load();
	});

	async function del() {
		const o = pendingDelete;
		if (!o) return;
		deleting = true;
		error = '';
		try {
			const res = await fetch(`${base}/delete`, {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...body, key: o.key })
			});
			if (!res.ok) throw await httpError(res, 'Delete failed');
			pendingDelete = null;
			refreshKey += 1;
			onChanged?.();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			deleting = false;
		}
	}

	function startRename(o: Obj) {
		renaming = o;
		renameValue = baseName(o.key);
	}
	async function submitRename() {
		const o = renaming;
		const name = renameValue.trim();
		if (!o || !name) return;
		renameBusy = true;
		error = '';
		try {
			const res = await fetch(`${base}/rename`, {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...body, key: o.key, new_name: name })
			});
			if (!res.ok) throw await httpError(res, 'Rename failed');
			renaming = null;
			refreshKey += 1;
			onChanged?.();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Rename failed';
		} finally {
			renameBusy = false;
		}
	}

	function baseName(key: string): string {
		const i = key.lastIndexOf('/');
		return i >= 0 ? key.slice(i + 1) : key;
	}

	// A leading underscore on ANY path segment marks a control file: configuration
	// that supports the content rather than being content (_course.css,
	// _references.md, a _drafts/ subtree…). Students never see these — the server
	// strips them from the student list routes, so this is purely how we present
	// them to the staff who DO see them: dimmed and monospaced, so a teacher never
	// mistakes one for a lesson resource. Mirrors content.IsControlPath in Go.
	function isControlFile(key: string): boolean {
		return key.split('/').some((seg) => seg.startsWith('_'));
	}
	function human(n: number): string {
		if (n < 1024) return `${n} B`;
		if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
		if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
		return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
	}
	// Compact size for the `list` variant: whole-number value + unit (e.g. 21kB,
	// 123MB), left-padded with spaces to a fixed width so the units line up in a
	// monospace column (rendered with white-space: pre). Thresholds at 1000 keep it
	// to ≤3 digits, so the string never exceeds the pad width.
	function shortSize(n: number): string {
		let s: string;
		if (n < 1000) s = `${n}B`;
		else if (n < 1e6) s = `${Math.round(n / 1000)}kB`;
		else if (n < 1e9) s = `${Math.round(n / 1e6)}MB`;
		else s = `${Math.round(n / 1e9)}GB`;
		return s.padStart(5, ' ');
	}
	// Always YYYY-MM-DD hh:mm:ss (local time), no other punctuation.
	function fmtDate(iso: string): string {
		const d = new Date(iso);
		if (isNaN(d.getTime())) return '';
		const p = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
	}

	// objects sorted by the active column/direction.
	const sorted = $derived(
		[...objects].sort((a, b) => {
			let r = 0;
			if (sortKey === 'name') r = baseName(a.key).localeCompare(baseName(b.key));
			else if (sortKey === 'size') r = a.size - b.size;
			else r = new Date(a.last_modified).getTime() - new Date(b.last_modified).getTime();
			return r * sortDir;
		})
	);
	function setSort(k: SortKey) {
		if (sortKey === k) sortDir = (sortDir === 1 ? -1 : 1) as 1 | -1;
		else {
			sortKey = k;
			sortDir = 1;
		}
	}
	// sort arrow for a heading: ▲ asc / ▼ desc when active, else blank.
	const arrow = (k: SortKey) => (sortKey === k ? (sortDir === 1 ? ' ▲' : ' ▼') : '');
	async function copy(u: string) {
		try {
			await navigator.clipboard.writeText(u);
			copied = u;
			setTimeout(() => (copied = ''), 1500);
		} catch {
			/* clipboard blocked; ignore */
		}
	}
</script>

{#if variant === 'list'}
	<!-- Compact, read-only two-column list of the course resources. Reuses the same
	     load()/sorted/baseName logic as the full browser; no tabs, uploader,
	     actions or modals. Column 1 (size) is gated by showSize. -->
	<div class="lms-browser lms-browser-list">
		{#if error}
			<p class="error"><i class="fa-solid fa-triangle-exclamation"></i> {error}</p>
		{:else if loading && objects.length === 0}
			<p class="muted">Loading…</p>
		{:else if objects.length === 0}
			<p class="muted">Nothing here yet.</p>
		{:else}
			<table>
				<tbody>
					{#each sorted as o (o.key)}
						<tr class:is-control={isControlFile(o.key)}>
							{#if showSize}<td class="num mono">{shortSize(o.size)}</td>{/if}
							<td class="name"
								><a href={o.public_url} target="_blank" rel="noopener noreferrer"
									>{baseName(o.key)}</a
								></td
							>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
{:else}
	<div class="lms-browser">
	{#if scopes.length > 1}
		<Tabs
			tabs={scopes.map((s) => ({ value: s, label: LABEL[s] }))}
			{active}
			showLabels
			onSelect={(v) => (active = v as Scope)}
		/>
	{/if}

	{#if canWrite}
		<ResourceUploader
			{base}
			{body}
			onUploaded={() => {
				refreshKey += 1;
				onChanged?.();
			}}
		/>
	{/if}

	{#if error}
		<p class="error"><i class="fa-solid fa-triangle-exclamation"></i> {error}</p>
	{:else if loading && objects.length === 0}
		<p class="muted">Loading…</p>
	{:else if objects.length === 0}
		<p class="muted">Nothing here yet.</p>
	{:else}
		<div class="scroll">
			<table>
				<thead>
					<tr>
						<th
							class="sortable"
							aria-sort={sortKey === 'name' ? (sortDir === 1 ? 'ascending' : 'descending') : 'none'}
							onclick={() => setSort('name')}>Name{arrow('name')}</th
						>
						{#if narrow}
							<th class="actions-col"><span class="sr-only">Details</span></th>
						{:else}
							{#if showSize}
								<th
									class="sortable num"
									aria-sort={sortKey === 'size' ? (sortDir === 1 ? 'ascending' : 'descending') : 'none'}
									onclick={() => setSort('size')}>Size{arrow('size')}</th
								>
							{/if}
							<th
								class="sortable"
								aria-sort={sortKey === 'modified' ? (sortDir === 1 ? 'ascending' : 'descending') : 'none'}
								onclick={() => setSort('modified')}>Modified{arrow('modified')}</th
							>
							<th class="actions-col">Action</th>
						{/if}
					</tr>
				</thead>
				<tbody>
					{#each sorted as o (o.key)}
						<tr class:is-control={isControlFile(o.key)}>
							<td class="name">
								{#if isControlFile(o.key)}
									<i class="fa-regular fa-gear" title="Control file — not shown to students"></i>
								{:else}
									<i class="fa-regular fa-file"></i>
								{/if}
								{baseName(o.key)}
							</td>
							{#if narrow}
								<td class="actions-col">
									<button
										class="pseudo icon-btn"
										onclick={() => (details = o)}
										title="Details"
										aria-label="Details for {baseName(o.key)}"
									>
										<i class="fa-solid fa-ellipsis"></i>
									</button>
								</td>
							{:else}
								{#if showSize}<td class="num">{human(o.size)}</td>{/if}
								<td class="muted">{fmtDate(o.last_modified)}</td>
								<td class="actions-col">
									<button class="pseudo icon-btn" onclick={() => copy(o.public_url)} title="Copy URL">
										<i class="fa-solid {copied === o.public_url ? 'fa-check' : 'fa-copy'}"></i>
									</button>
									{#if canWrite}
										<button
											class="pseudo icon-btn"
											onclick={() => startRename(o)}
											title="Rename"
											aria-label="Rename {baseName(o.key)}"
										>
											<i class="fa-solid fa-pen"></i>
										</button>
										<button
											class="pseudo icon-btn del"
											onclick={() => (pendingDelete = o)}
											title="Delete"
											aria-label="Delete {baseName(o.key)}"
										>
											<i class="fa-solid fa-skull-crossbones"></i>
										</button>
									{/if}
								</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
	</div>
{/if}

{#if pendingDelete}
	<div
		class="modal-overlay"
		role="button"
		tabindex="-1"
		onclick={(e) => e.target === e.currentTarget && !deleting && (pendingDelete = null)}
		onkeydown={(e) => e.key === 'Escape' && !deleting && (pendingDelete = null)}
	>
		<div class="modal" role="dialog" aria-modal="true" tabindex="-1">
			<p class="modal-title">
				<i class="fa-solid fa-skull-crossbones"></i> Delete this file?
			</p>
			<p class="modal-name">{baseName(pendingDelete.key)}</p>
			<div class="modal-actions">
				<button class="pseudo" onclick={() => (pendingDelete = null)} disabled={deleting}>Cancel</button>
				<button class="danger-btn" onclick={del} disabled={deleting}>
					{#if deleting}<i class="fa-solid fa-spinner fa-spin"></i> {/if}Delete
				</button>
			</div>
		</div>
	</div>
{/if}

{#if renaming}
	<div
		class="modal-overlay"
		role="button"
		tabindex="-1"
		onclick={(e) => e.target === e.currentTarget && !renameBusy && (renaming = null)}
		onkeydown={(e) => e.key === 'Escape' && !renameBusy && (renaming = null)}
	>
		<div class="modal" role="dialog" aria-modal="true" tabindex="-1">
			<label class="modal-title" for="lms-rename-input">
				<i class="fa-solid fa-pen"></i> Rename file
			</label>
			<!-- svelte-ignore a11y_autofocus -->
			<input
				id="lms-rename-input"
				name="lms-rename"
				class="rename-input"
				type="text"
				autocomplete="off"
				bind:value={renameValue}
				disabled={renameBusy}
				autofocus
				onkeydown={(e) => {
					if (e.key === 'Enter') submitRename();
				}}
			/>
			<div class="modal-actions">
				<button class="pseudo" onclick={() => (renaming = null)} disabled={renameBusy}>Cancel</button>
				<button onclick={submitRename} disabled={renameBusy || !renameValue.trim()}>
					{#if renameBusy}<i class="fa-solid fa-spinner fa-spin"></i> {/if}Rename
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Narrow-screen row details: the columns hidden from the collapsed table, as
     full-width rows, with the row's actions beneath. Rename/Delete hand off to
     their own modals (closing this one first so modals never stack). -->
{#if details}
	{@const d = details}
	<div
		class="modal-overlay"
		role="button"
		tabindex="-1"
		onclick={(e) => e.target === e.currentTarget && (details = null)}
		onkeydown={(e) => e.key === 'Escape' && (details = null)}
	>
		<div class="modal" role="dialog" aria-modal="true" tabindex="-1">
			<p class="modal-title"><i class="fa-regular fa-file"></i> {baseName(d.key)}</p>
			<dl class="detail-rows">
				{#if showSize}
					<div class="detail-row"><dt>Size</dt><dd class="num">{human(d.size)}</dd></div>
				{/if}
				<div class="detail-row"><dt>Modified</dt><dd>{fmtDate(d.last_modified)}</dd></div>
			</dl>
			<div class="modal-actions">
				<button class="pseudo" onclick={() => copy(d.public_url)}>
					<i class="fa-solid {copied === d.public_url ? 'fa-check' : 'fa-copy'}"></i> Copy URL
				</button>
				{#if canWrite}
					<button
						class="pseudo"
						onclick={() => {
							details = null;
							startRename(d);
						}}
					>
						<i class="fa-solid fa-pen"></i> Rename
					</button>
					<button
						class="danger-btn"
						onclick={() => {
							details = null;
							pendingDelete = d;
						}}
					>
						<i class="fa-solid fa-skull-crossbones"></i> Delete
					</button>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	.lms-browser {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}
	.scroll {
		overflow-x: auto;
	}
	table {
		width: 100%;
		table-layout: auto;
	}
	/* cells auto-wrap; long unbroken names wrap mid-word rather than overflow. */
	td {
		white-space: normal;
		overflow-wrap: anywhere;
		vertical-align: top;
		/** override picnicss asymetric padding*/
		padding: .3em .3em .3em .6em
	}
	.num {
		font-variant-numeric: tabular-nums;
		text-align: right;
		white-space: nowrap;
	}
	/* `list` variant: compact, borderless rows; the size cell is monospace with
	   preserved leading spaces so the left-padded sizes line up. */
	.lms-browser-list table {
		width: 100%;
	}
	.lms-browser-list td {
		padding: 0.1em 0.4em;
	}
	.lms-browser-list .mono {
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
		white-space: pre;
		width: 1%; /* only as wide as the 5-char size */
	}
	.lms-browser-list .name {
		overflow-wrap: anywhere;
	}
	/* Control files (a "_"-prefixed path segment) reach staff only — students are
	   filtered server-side. Style them as configuration, not content: dimmed and
	   monospaced so they read differently at a glance and a teacher doesn't take
	   one for a lesson resource. Deliberately restrained — these are still
	   editable rows, not warnings. */
	tr.is-control .name {
		color: var(--pi-meta);
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
		font-style: italic;
	}
	tr.is-control .name i {
		opacity: 0.7;
	}
	.muted {
		color: var(--pi-meta);
	}
	.error {
		color: var(--hi1);
	}
	.sortable {
		cursor: pointer;
		user-select: none;
		white-space: nowrap;
	}
	.sortable:hover {
		color: var(--hi2);
	}
	/* Action column: only as wide as its icons, which carry half the usual padding. */
	.actions-col {
		width: 1%;
		white-space: nowrap;
		text-align: right;
	}
	.icon-btn {
		padding: 0.15rem 0.25rem;
		margin: 0;
	}
	.del i {
		color: var(--hi1);
	}
	.danger-btn {
		color: var(--hi1);
		font-weight: 600;
	}

	/* delete confirmation modal */
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
		font-weight: 600;
		margin: 0 0 0.4rem;
	}
	.modal-title i {
		color: var(--hi1);
	}
	.modal-name {
		margin: 0 0 1rem;
		color: var(--pi-meta);
		overflow-wrap: anywhere;
	}
	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.6rem;
		flex-wrap: wrap; /* narrow details modal: buttons wrap rather than overflow */
	}
	.rename-input {
		display: block;
		width: 100%;
		box-sizing: border-box;
		margin: 0 0 1rem;
	}
	/* narrow-screen details modal: each hidden column as a full-width label/value row. */
	.detail-rows {
		margin: 0.4rem 0 1rem;
	}
	.detail-row {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.4rem 0;
		border-bottom: 1px solid oklch(from var(--pi-body) l c h / 0.15);
	}
	.detail-row dt {
		font-weight: 600;
		color: var(--pi-meta);
	}
	.detail-row dd {
		margin: 0;
		text-align: right;
		overflow-wrap: anywhere;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
