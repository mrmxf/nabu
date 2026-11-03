<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { httpError } from '$lib/httpError';

	// Uploads a resource to S3 two ways: a file from the user's machine (presigned
	// PUT direct to the bucket, single or multipart) or a URL (imported server-side
	// by nabu). nabu derives the bucket/prefix and mints the key; this component
	// never sees S3 credentials. It is endpoint-driven so it serves both the staff
	// course-resource routes and the student group/own routes: `base` is the route
	// prefix (e.g. /api/resources or /api/student/resources) and `body` is merged
	// into every request (e.g. { lesson_id } or { session_id, scope }). Calls
	// onUploaded so the host can refresh its listing.
	let {
		base,
		body: extra,
		disabled = false,
		accept = '',
		allowPaste = false,
		onUploaded
	}: {
		base: string;
		body: Record<string, unknown>;
		disabled?: boolean;
		// optional file-type filter for the file input (e.g. ".mp4" or "video/*").
		accept?: string;
		// when true, offer a "Paste into file" box (filename + textarea) so a student
		// can hand in text they copied from an agent (e.g. a summary) without saving
		// it to their machine first.
		allowPaste?: boolean;
		onUploaded?: () => void;
	} = $props();

	// Files at or above this size upload via S3 multipart (resumable, per-part
	// retry) instead of a single presigned PUT; PART_SIZE is each chunk. S3's
	// minimum part size is 5 MiB (last part exempt); 64 MiB keeps part counts low
	// (a 1 GB file is 16 parts) while staying well under the 10000-part limit.
	const MULTIPART_THRESHOLD = 64 * 1024 * 1024;
	const PART_SIZE = 64 * 1024 * 1024;
	const PART_RETRIES = 3;

	let files = $state<FileList | null>(null);
	let url = $state('');
	let pasteName = $state('');
	let pasteText = $state('');
	let busy = $state(false);
	let error = $state('');
	let done = $state('');
	let progress = $state(0); // 0–100, shown during a multipart upload

	const authHeaders = () => ({ Authorization: pb.authStore.token });

	async function uploadFile() {
		if (!files?.length) return;
		const f = files[0];
		busy = true;
		error = '';
		done = '';
		progress = 0;
		try {
			await putFile(f);
			done = `Uploaded ${f.name}`;
			files = null;
			onUploaded?.();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Upload failed';
		} finally {
			busy = false;
			progress = 0;
		}
	}

	// Shared upload path for both a picked File and a pasted blob: small files take a
	// single presigned PUT, large ones go multipart.
	async function putFile(f: File) {
		const contentType = f.type || 'application/octet-stream';
		if (f.size >= MULTIPART_THRESHOLD) {
			await uploadMultipart(f, contentType);
		} else {
			await uploadSingle(f, contentType);
		}
	}

	// Best-effort MIME type from the filename so pasted HTML/JSON/Markdown are stored
	// (and later served) with a sensible content type; anything else is plain text.
	function pasteContentType(name: string): string {
		const ext = name.slice(name.lastIndexOf('.') + 1).toLowerCase();
		switch (ext) {
			case 'html':
			case 'htm':
				return 'text/html';
			case 'json':
				return 'application/json';
			case 'md':
			case 'markdown':
				return 'text/markdown';
			case 'csv':
				return 'text/csv';
			default:
				return 'text/plain';
		}
	}

	async function pasteFile() {
		const name = pasteName.trim();
		if (!name || !pasteText) return;
		busy = true;
		error = '';
		done = '';
		progress = 0;
		try {
			const f = new File([pasteText], name, { type: pasteContentType(name) });
			await putFile(f);
			done = `Saved ${name}`;
			pasteName = '';
			pasteText = '';
			onUploaded?.();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Save failed';
		} finally {
			busy = false;
			progress = 0;
		}
	}

	// Small files: one presigned PUT straight to S3.
	async function uploadSingle(f: File, contentType: string) {
		const pres = await fetch(`${base}/presign`, {
			method: 'POST',
			headers: { ...authHeaders(), 'Content-Type': 'application/json' },
			body: JSON.stringify({ ...extra, filename: f.name, content_type: contentType })
		});
		if (!pres.ok) throw await httpError(pres, 'Upload failed');
		const { put_url, key } = await pres.json();
		const put = await fetch(put_url, {
			method: 'PUT',
			headers: { 'Content-Type': contentType },
			body: f
		});
		if (!put.ok) throw new Error(`storage upload failed (${put.status})`);
		// nabu never sees this direct PUT — tell it so it can index the object
		// (replaces the old fresh=1 bucket re-scan).
		await fetch(`${base}/confirm`, {
			method: 'POST',
			headers: { ...authHeaders(), 'Content-Type': 'application/json' },
			body: JSON.stringify({ ...extra, key, size: f.size, content_type: contentType })
		}).catch(() => {});
	}

	// Large files: S3 multipart. nabu starts the upload and signs each part; the
	// browser PUTs parts directly to S3 (sequential, per-part retry), then nabu
	// completes. Any failure after start aborts the upload so no orphan parts linger.
	async function uploadMultipart(f: File, contentType: string) {
		const created = await fetch(`${base}/multipart/create`, {
			method: 'POST',
			headers: { ...authHeaders(), 'Content-Type': 'application/json' },
			body: JSON.stringify({ ...extra, filename: f.name, content_type: contentType })
		});
		if (!created.ok) throw await httpError(created, 'Upload failed');
		const { upload_id, key } = await created.json();

		try {
			const total = Math.ceil(f.size / PART_SIZE);
			const parts: { part_number: number; etag: string }[] = [];
			for (let n = 1; n <= total; n++) {
				const blob = f.slice((n - 1) * PART_SIZE, n * PART_SIZE);
				const etag = await uploadPart(key, upload_id, n, blob, contentType);
				parts.push({ part_number: n, etag });
				progress = Math.round((n / total) * 100);
			}
			const completed = await fetch(`${base}/multipart/complete`, {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': 'application/json' },
				body: JSON.stringify({
					...extra,
					key,
					upload_id,
					parts,
					size: f.size,
					content_type: contentType
				})
			});
			if (!completed.ok) throw await httpError(completed, 'Upload failed');
		} catch (e) {
			// Best-effort cleanup; ignore its result so the original error surfaces.
			await fetch(`${base}/multipart/abort`, {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...extra, key, upload_id })
			}).catch(() => {});
			throw e;
		}
	}

	// Uploads one part to a freshly-signed URL, retrying transient failures. Returns
	// the part's ETag, which the browser can read because the bucket CORS exposes it.
	async function uploadPart(
		key: string,
		uploadId: string,
		partNumber: number,
		blob: Blob,
		contentType: string
	): Promise<string> {
		let lastErr: unknown;
		for (let attempt = 1; attempt <= PART_RETRIES; attempt++) {
			try {
				const signed = await fetch(`${base}/multipart/sign`, {
					method: 'POST',
					headers: { ...authHeaders(), 'Content-Type': 'application/json' },
					body: JSON.stringify({ ...extra, key, upload_id: uploadId, part_number: partNumber })
				});
				if (!signed.ok) throw await httpError(signed, 'Upload failed');
				const { put_url } = await signed.json();
				const put = await fetch(put_url, {
					method: 'PUT',
					headers: { 'Content-Type': contentType },
					body: blob
				});
				if (!put.ok) throw new Error(`part ${partNumber} failed (${put.status})`);
				const etag = put.headers.get('ETag');
				if (!etag) throw new Error(`part ${partNumber} returned no ETag`);
				return etag;
			} catch (e) {
				lastErr = e;
			}
		}
		throw lastErr instanceof Error ? lastErr : new Error(`part ${partNumber} failed`);
	}

	async function importUrl() {
		if (!url) return;
		busy = true;
		error = '';
		done = '';
		try {
			const res = await fetch(`${base}/import-url`, {
				method: 'POST',
				headers: { ...authHeaders(), 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...extra, url })
			});
			if (!res.ok) throw await httpError(res, 'Import failed');
			done = 'Imported from URL';
			url = '';
			onUploaded?.();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Import failed';
		} finally {
			busy = false;
		}
	}
</script>

<div class="uploader">
	<div class="row">
		<label class="field">
			<span><i class="fa-solid fa-file-arrow-up"></i> From your machine</span>
			<input type="file" bind:files disabled={disabled || busy} accept={accept || undefined} />
		</label>
		<button onclick={uploadFile} disabled={disabled || busy || !files?.length}>
			<i class="fa-solid fa-upload"></i> Upload
		</button>
	</div>

	<div class="row">
		<label class="field grow">
			<span><i class="fa-solid fa-link"></i> From a URL</span>
			<input
				type="url"
				placeholder="https://…"
				bind:value={url}
				disabled={disabled || busy}
			/>
		</label>
		<button onclick={importUrl} disabled={disabled || busy || !url}>
			<i class="fa-solid fa-cloud-arrow-down"></i> Import
		</button>
	</div>

	{#if allowPaste}
		<div class="paste">
			<label class="field grow">
				<span><i class="fa-solid fa-paste"></i> Paste into file</span>
				<input
					type="text"
					placeholder="filename e.g. agent-notes.md"
					bind:value={pasteName}
					disabled={disabled || busy}
				/>
			</label>
			<textarea
				class="paste-body"
				rows="6"
				placeholder="Paste text here (e.g. an agent's output)…"
				bind:value={pasteText}
				disabled={disabled || busy}
			></textarea>
			<button
				class="paste-save"
				onclick={pasteFile}
				disabled={disabled || busy || !pasteName.trim() || !pasteText}
			>
				<i class="fa-solid fa-floppy-disk"></i> Save
			</button>
		</div>
	{/if}

	{#if busy}<p class="muted">
			<i class="fa-solid fa-spinner fa-spin"></i>
			{#if progress > 0}Uploading… {progress}%{:else}Working…{/if}
		</p>{/if}
	{#if done}<p class="ok"><i class="fa-solid fa-check"></i> {done}</p>{/if}
	{#if error}<p class="error"><i class="fa-solid fa-triangle-exclamation"></i> {error}</p>{/if}
</div>

<style>
	.uploader {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.row {
		display: flex;
		gap: 0.7rem;
		align-items: flex-end;
		flex-wrap: wrap;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-weight: normal;
	}
	.field.grow {
		flex: 1;
		min-width: 14rem;
	}
	.field.grow input {
		width: 100%;
	}
	.paste {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.paste-body {
		width: 100%;
		box-sizing: border-box;
		resize: vertical;
		font-family: var(--pi-mono, monospace);
		font-size: 0.85rem;
	}
	.paste-save {
		align-self: flex-start;
	}
	.muted {
		color: var(--pi-meta);
		margin: 0;
	}
	.ok {
		color: oklch(from var(--hi4) 42% c h);
		margin: 0;
	}
	.error {
		color: var(--hi1);
		margin: 0;
	}
</style>
