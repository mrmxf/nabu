<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { httpError } from '$lib/httpError';
	import { isStaff } from '$lib/stores/auth.svelte';
	import ResourceUploader from '$lib/components/ResourceUploader.svelte';

	// A focused student uploader, mounted by the LMS `uploader` widget. Unlike the
	// full `browser` widget it targets a single scope and shows only the student's
	// own uploads there (name, size, delete) — the minimal "hand something in" UI a
	// lesson asks for. Upload plumbing is reused from ResourceUploader (presign /
	// multipart / confirm); this component adds the scoped list.
	//
	// Scope is the student's own work by default, or their group's shared work. Both
	// go through the session-keyed /api/student/resources routes, so a student never
	// touches the course-only endpoints.
	type Scope = 'student' | 'group';

	let {
		scope = 'student',
		accept = '',
		label = '',
		groupTag = '',
		sessionId = '',
		disabled = false
	}: {
		scope?: Scope;
		accept?: string;
		label?: string;
		// The session's group name, shown in the assets box title for group scope.
		groupTag?: string;
		sessionId?: string;
		disabled?: boolean;
	} = $props();

	// Title of the box that surrounds the file list: the group's name for group
	// scope (falling back to "Group" when no name is set yet), else the student's
	// own work.
	const boxTitle = $derived(
		scope === 'group' ? `${groupTag || 'Group'} assets` : 'My assets'
	);

	// A signed-out guest (help preview) or a caller with no session can't upload.
	const canUpload = $derived(!disabled && !!sessionId && !isStaff());
	const body = $derived<Record<string, unknown>>({ session_id: sessionId, scope });
	const listUrl = $derived(
		`/api/student/resources?session_id=${encodeURIComponent(sessionId)}&scope=${scope}`
	);

	interface Obj {
		key: string;
		size: number;
		last_modified: string;
		public_url: string;
	}
	let objects = $state<Obj[]>([]);
	let loading = $state(false);
	let error = $state('');
	let refreshKey = $state(0);
	let pendingDelete = $state<Obj | null>(null);
	let deleting = $state(false);
	let renaming = $state<Obj | null>(null); // row being renamed in the modal
	let renameValue = $state('');
	let renameBusy = $state(false);

	const authHeaders = () => ({ Authorization: pb.authStore.token });

	async function load() {
		if (!sessionId) {
			objects = [];
			return;
		}
		loading = true;
		error = '';
		try {
			const res = await fetch(listUrl, { headers: authHeaders() });
			if (!res.ok) throw await httpError(res, 'Could not list your uploads');
			objects = (await res.json()).objects ?? [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not list your uploads';
			objects = [];
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void refreshKey;
		void sessionId;
		void scope;
		load();
	});

	async function del() {
		const o = pendingDelete;
		if (!o) return;
		deleting = true;
		error = '';
		try {
			const res = await fetch('/api/student/resources/delete', {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...body, key: o.key })
			});
			if (!res.ok) throw await httpError(res, 'Delete failed');
			pendingDelete = null;
			refreshKey += 1;
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
			const res = await fetch('/api/student/resources/rename', {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...body, key: o.key, new_name: name })
			});
			if (!res.ok) throw await httpError(res, 'Rename failed');
			renaming = null;
			refreshKey += 1;
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
	function human(n: number): string {
		if (n < 1024) return `${n} B`;
		if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
		if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
		return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
	}
</script>

<div class="lms-uploader">
	{#if label}<p class="label">{label}</p>{/if}

	{#if canUpload}
		<ResourceUploader {body} base="/api/student/resources" {accept} allowPaste onUploaded={() => (refreshKey += 1)} />
	{:else if isStaff()}
		<p class="muted">The student uploader is disabled in the teacher view.</p>
	{:else if !sessionId}
		<p class="muted">Start a session to upload your work.</p>
	{/if}

	<section class="assets-box">
		<h4 class="assets-title">{boxTitle}</h4>
		{#if error}
			<p class="error"><i class="fa-solid fa-triangle-exclamation"></i> {error}</p>
		{:else if loading && objects.length === 0}
			<p class="muted">Loading…</p>
		{:else if objects.length === 0}
			<p class="muted">No assets yet</p>
		{:else}
			<ul class="files">
				{#each objects as o (o.key)}
					<li>
						<a href={o.public_url} target="_blank" rel="noopener">
							<i class="fa-regular fa-file"></i> {baseName(o.key)}
						</a>
						<span class="size">{human(o.size)}</span>
						{#if canUpload}
							<button
								class="pseudo rename"
								title="Rename"
								aria-label="Rename {baseName(o.key)}"
								onclick={() => startRename(o)}
							>
								<i class="fa-solid fa-pen"></i>
							</button>
							<button
								class="pseudo del"
								title="Delete"
								aria-label="Delete {baseName(o.key)}"
								onclick={() => (pendingDelete = o)}
							>
								<i class="fa-solid fa-skull-crossbones"></i>
							</button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</div>

{#if pendingDelete}
	<div
		class="modal-overlay"
		role="button"
		tabindex="-1"
		onclick={(e) => e.target === e.currentTarget && !deleting && (pendingDelete = null)}
		onkeydown={(e) => e.key === 'Escape' && !deleting && (pendingDelete = null)}
	>
		<div class="modal" role="dialog" aria-modal="true" tabindex="-1">
			<p class="modal-title"><i class="fa-solid fa-skull-crossbones"></i> Delete this file?</p>
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
			<label class="modal-title" for="lms-uploader-rename-input">
				<i class="fa-solid fa-pen"></i> Rename file
			</label>
			<!-- svelte-ignore a11y_autofocus -->
			<input
				id="lms-uploader-rename-input"
				name="lms-uploader-rename"
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

<style>
	.lms-uploader {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.label {
		margin: 0;
		font-weight: 600;
	}
	.muted {
		color: var(--pi-meta);
		margin: 0;
	}
	.error {
		color: var(--hi1);
		margin: 0;
	}
	/* Box surrounding the file list, titled with the group name (or "My assets"). */
	.assets-box {
		border: 1px solid oklch(from var(--pi-body) l c h / 0.2);
		border-radius: 8px;
		padding: 0.7rem 0.9rem;
	}
	.assets-title {
		margin: 0 0 0.5rem;
		font-size: 0.95rem;
		font-weight: 600;
	}
	.assets-box .muted {
		margin: 0;
	}
	.files {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.files li {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.files a {
		overflow-wrap: anywhere;
	}
	.size {
		color: var(--pi-meta);
		font-variant-numeric: tabular-nums;
		font-size: 0.85rem;
		white-space: nowrap;
	}
	.rename {
		padding: 0.1rem 0.3rem;
		margin: 0 0 0 auto;
	}
	.del {
		padding: 0.1rem 0.3rem;
		margin: 0;
	}
	.del i {
		color: var(--hi1);
	}
	.danger-btn {
		color: var(--hi1);
		font-weight: 600;
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
	}
	.rename-input {
		display: block;
		width: 100%;
		box-sizing: border-box;
		margin: 0 0 1rem;
	}
</style>
