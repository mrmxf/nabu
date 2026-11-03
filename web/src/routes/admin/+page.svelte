<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { auth, isAdmin } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	// Minimal admin surface: view a course roster (people + positions) and import a
	// roster file. Full backup/restore stays CLI/API only (it carries secrets).
	interface Position {
		lesson: string;
		current_slide: number;
		total_slides: number;
		percent: number;
		status: string;
	}
	interface Member {
		uid: string;
		email: string;
		name: string;
		role: string;
		positions?: Position[];
	}
	interface Roster {
		course: string;
		teachers: Member[];
		students: Member[];
	}

	let courseSlug = $state('');
	let roster = $state<Roster | null>(null);
	let rosterError = $state('');

	let file = $state<FileList | null>(null);
	let mode = $state('merge');
	let importScope = $state('');
	let importResult = $state('');
	let importError = $state('');
	let busy = $state(false);

	onMount(() => {
		if (!auth.user) {
			goto('/login');
		} else if (!isAdmin()) {
			goto('/');
		}
	});

	// Auth header for the custom endpoints (same-origin, PocketBase token).
	const authHeaders = () => ({ Authorization: pb.authStore.token });

	async function loadRoster() {
		rosterError = '';
		roster = null;
		try {
			const res = await fetch(`/api/courses/${encodeURIComponent(courseSlug)}/roster`, {
				headers: authHeaders()
			});
			if (!res.ok) throw new Error(await res.text());
			roster = await res.json();
		} catch (err) {
			rosterError = err instanceof Error ? err.message : 'Could not load roster';
		}
	}

	function contentTypeFor(name: string): string {
		if (name.endsWith('.csv')) return 'text/csv';
		if (name.endsWith('.json')) return 'application/json';
		return 'application/yaml';
	}

	async function runImport() {
		importResult = '';
		importError = '';
		if (!file || file.length === 0) {
			importError = 'Choose a file first.';
			return;
		}
		busy = true;
		try {
			const f = file[0];
			const body = await f.text();
			const params = new URLSearchParams({ mode });
			if (importScope) params.set('course', importScope);
			const res = await fetch(`/api/users/import?${params}`, {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': contentTypeFor(f.name) },
				body
			});
			if (!res.ok) throw new Error(await res.text());
			const sum = await res.json();
			importResult = `created ${sum.created}, updated ${sum.updated}, enrolled ${sum.enrolled}, unenrolled ${sum.unenrolled}, deleted ${sum.deleted}`;
			if (sum.errors?.length) importResult += ` — ${sum.errors.length} error(s)`;
			await loadRoster();
		} catch (err) {
			importError = err instanceof Error ? err.message : 'Import failed';
		} finally {
			busy = false;
		}
	}
</script>

<h1>Admin — student management</h1>
<p class="muted">
	Full backup/restore is CLI/API only (it carries password hashes). See
	<code>docs/student-management.md</code>.
</p>

<section class="card">
	<h2>Import roster</h2>
	<p class="muted">Upload a CSV/YAML/JSON roster. Matching is by <code>uid</code>.</p>
	<div class="row">
		<input type="file" accept=".csv,.yaml,.yml,.json" bind:files={file} />
		<label>Mode
			<select bind:value={mode}>
				<option value="merge">merge</option>
				<option value="overwrite">overwrite</option>
				<option value="delete-orphans">delete-orphans</option>
			</select>
		</label>
		<label>Scope course
			<input type="text" placeholder="(optional slug)" bind:value={importScope} />
		</label>
		<button onclick={runImport} disabled={busy}>Import</button>
	</div>
	{#if importResult}<p class="ok">{importResult}</p>{/if}
	{#if importError}<p class="error">{importError}</p>{/if}
</section>

<section class="card">
	<h2>Course roster</h2>
	<div class="row">
		<input type="text" bind:value={courseSlug} placeholder="course slug" />
		<button onclick={loadRoster}>Load roster</button>
	</div>
	{#if rosterError}<p class="error">{rosterError}</p>{/if}
	{#if roster}
		<h3>Teachers</h3>
		{#if roster.teachers.length}
			<ul>
				{#each roster.teachers as t (t.uid)}<li>{t.name} <span class="muted">({t.email})</span></li>{/each}
			</ul>
		{:else}<p class="muted">None.</p>{/if}

		<h3>Students</h3>
		{#if roster.students.length}
			<div class="scroll">
				<table>
					<thead><tr><th>Student</th><th>Lesson</th><th>Position</th><th>Status</th></tr></thead>
					<tbody>
						{#each roster.students as s (s.uid)}
							{#if s.positions && s.positions.length}
								{#each s.positions as p, i (p.lesson)}
									<tr>
										<td>{i === 0 ? s.name : ''}</td>
										<td>{p.lesson}</td>
										<td>{p.current_slide} / {p.total_slides} ({p.percent}%)</td>
										<td>{p.status}</td>
									</tr>
								{/each}
							{:else}
								<tr><td>{s.name}</td><td colspan="3" class="muted">not started</td></tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
		{:else}<p class="muted">No students enrolled.</p>{/if}
	{/if}
</section>

<style>
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.8rem;
		align-items: flex-end;
	}
	.muted {
		color: var(--pi-meta);
	}
	.ok {
		color: oklch(from var(--hi4) 42% c h);
	}
	.error {
		color: var(--hi1);
	}
	.scroll {
		overflow-x: auto;
	}
	table {
		width: 100%;
	}
</style>
