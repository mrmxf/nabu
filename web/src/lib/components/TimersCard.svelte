<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { onDestroy, onMount } from 'svelte';
	import { handleAuthError } from '$lib/stores/auth.svelte';
	import type { SessionTimerRec } from '$lib/types';

	// Teacher control for the cohort-wide synchronized timers (session_timers). A
	// timer is created when the first student reaches a slide that declares it; here
	// the teacher pauses/resumes, adds time, or resets it — writes land on the
	// record (staff UpdateRule) and every student following it updates instantly.
	// The displayed time_left ticks locally against the shared absolute end_time.
	//
	// `bare` drops the card chrome + heading so the component can sit inside the
	// queues-dashboard accordion, which supplies both.
	let { bare = false }: { bare?: boolean } = $props();

	interface Row {
		id: string;
		label: string;
		cohort: string;
		running: boolean;
		endTimeMs: number; // absolute deadline
		remainingMs: number; // frozen while paused
		durationSeconds: number;
		addMins: number; // the value in the "add minutes" input
		saving: boolean;
	}

	let rows = $state<Row[]>([]);
	let error = $state('');
	let loading = $state(true);
	let nowMs = $state(Date.now()); // ticked so time_left re-derives every second
	let unsub: (() => void) | undefined;
	let ticker: ReturnType<typeof setInterval> | undefined;

	function toRow(rec: SessionTimerRec, prev?: Row): Row {
		return {
			id: rec.id,
			label: rec.label || rec.timer_id,
			cohort: rec.expand?.session?.name ?? '',
			running: rec.running,
			endTimeMs: rec.end_time ? Date.parse(rec.end_time) : 0,
			remainingMs: rec.remaining_ms ?? 0,
			durationSeconds: rec.duration_seconds ?? 0,
			addMins: prev?.addMins ?? 5,
			saving: false
		};
	}

	function remainingSec(r: Row): number {
		// Not clamped at zero: a running timer counts into negative (overtime).
		const ms = r.running ? r.endTimeMs - nowMs : r.remainingMs;
		return Math.round(ms / 1000);
	}

	function fmt(s: number): string {
		const neg = s < 0;
		const a = Math.abs(s);
		const h = Math.floor(a / 3600);
		const m = Math.floor((a % 3600) / 60);
		const sec = a % 60;
		return `${neg ? '-' : ''}${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`;
	}

	function clockLabel(r: Row): string {
		if (!r.running || !r.endTimeMs) return '—';
		return new Date(r.endTimeMs).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	async function load() {
		try {
			const recs = await pb
				.collection('session_timers')
				.getFullList<SessionTimerRec>({ expand: 'session', sort: 'created' });
			const byId = new Map(rows.map((r) => [r.id, r]));
			rows = recs.map((r) => toRow(r, byId.get(r.id)));
			error = '';
		} catch (err) {
			if (handleAuthError(err)) return;
			error = err instanceof Error ? err.message : 'Could not load timers';
		} finally {
			loading = false;
		}
	}

	// Apply a realtime change without discarding an in-progress "add minutes" edit.
	function applyRemote(rec: SessionTimerRec) {
		const prev = rows.find((r) => r.id === rec.id);
		if (!prev) {
			void load(); // a newly-created timer: reload to pick up the expand
			return;
		}
		rows = rows.map((r) => (r.id === rec.id ? toRow(rec, r) : r));
	}

	async function write(row: Row, data: Record<string, unknown>) {
		row.saving = true;
		rows = [...rows];
		try {
			await pb.collection('session_timers').update(row.id, data);
		} catch (err) {
			if (!handleAuthError(err)) error = err instanceof Error ? err.message : 'Could not update timer';
		} finally {
			row.saving = false;
			rows = [...rows];
		}
	}

	function toggle(row: Row) {
		if (row.running) {
			// Pause: freeze whatever is left (may be negative if already in overtime).
			write(row, { running: false, remaining_ms: row.endTimeMs - Date.now() });
		} else {
			// Resume: re-anchor the deadline to now + the frozen remaining.
			write(row, { running: true, end_time: new Date(Date.now() + row.remainingMs).toISOString() });
		}
	}

	function addTime(row: Row) {
		const deltaMs = Math.round(row.addMins * 60 * 1000);
		if (!deltaMs) return;
		if (row.running) {
			write(row, { end_time: new Date(row.endTimeMs + deltaMs).toISOString() });
		} else {
			write(row, { remaining_ms: row.remainingMs + deltaMs });
		}
	}

	function reset(row: Row) {
		const durMs = row.durationSeconds * 1000;
		write(row, {
			running: true,
			end_time: new Date(Date.now() + durMs).toISOString(),
			remaining_ms: durMs
		});
	}

	onMount(async () => {
		await load();
		ticker = setInterval(() => (nowMs = Date.now()), 1000);
		try {
			unsub = await pb.collection('session_timers').subscribe('*', (e) => {
				if (e.action === 'delete') rows = rows.filter((r) => r.id !== e.record.id);
				else applyRemote(e.record as unknown as SessionTimerRec);
			});
		} catch {
			// realtime is optional; the local ticker still counts the loaded timers down.
		}
	});

	onDestroy(() => {
		unsub?.();
		clearInterval(ticker);
	});
</script>

<div class="timers" class:card={!bare}>
	<div class="head">
		{#if !bare}<h2>Timers</h2>{/if}
		<span class="muted small">shared countdowns · every student sees the same value · live</span>
	</div>

	{#if error}
		<p class="error">{error}</p>
	{:else if loading}
		<p class="muted">Loading…</p>
	{:else if rows.length === 0}
		<p class="muted">No timers running. One starts when the first student reaches a timed slide.</p>
	{:else}
		<ul class="list">
			{#each rows as r (r.id)}
				{@const secs = remainingSec(r)}
				<li>
					<span class="label" title={r.label}>{r.label}</span>
					{#if r.cohort}<span class="cohort muted">{r.cohort}</span>{/if}
					<span
						class="time"
						class:overtime={secs < 0}
						class:urgent={secs >= 0 && r.running && secs <= 60}
						class:paused={!r.running && secs >= 0}
					>
						{fmt(secs)}
					</span>
					<span class="ends muted" title="ends at">
						{secs < 0 ? 'over' : r.running ? `ends ${clockLabel(r)}` : 'paused'}
					</span>
					<button class="pseudo" onclick={() => toggle(r)} disabled={r.saving}>
						{r.running ? '⏸ Pause' : '▶ Start'}
					</button>
					<span class="add">
						<input
							type="number"
							step="1"
							bind:value={r.addMins}
							disabled={r.saving}
							title="minutes to add (or a negative number to subtract)"
							aria-label={`minutes to adjust ${r.label} by`}
						/>
						<span class="unit muted">min</span>
						<button class="pseudo" onclick={() => addTime(r)} disabled={r.saving || !r.addMins}>
							Apply
						</button>
					</span>
					<button class="pseudo reset" onclick={() => reset(r)} disabled={r.saving}>Reset</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.card {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	.head {
		display: flex;
		align-items: baseline;
		gap: 0.6rem;
	}
	.head h2 {
		margin: 0;
	}
	.muted {
		color: var(--pi-meta);
	}
	.small {
		font-size: 0.82rem;
	}
	.error {
		color: var(--hi1);
	}
	.list {
		list-style: none;
		padding: 0;
		margin: 0.6rem 0 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.list li {
		display: grid;
		grid-template-columns: minmax(6rem, 1fr) auto auto auto auto auto auto;
		align-items: center;
		gap: 0.6rem;
	}
	.label {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.cohort {
		font-size: 0.82rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.time {
		font-variant-numeric: tabular-nums;
		font-weight: 700;
		color: var(--hi2);
	}
	.time.urgent {
		color: var(--hi1);
	}
	.time.overtime {
		color: var(--hi1);
		font-weight: 800;
	}
	.time.paused {
		color: var(--pi-meta);
	}
	.ends {
		font-size: 0.82rem;
	}
	.add {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
	}
	.add input[type='number'] {
		width: 3.5rem;
		margin: 0;
	}
	.unit {
		font-size: 0.82rem;
	}
	.reset {
		color: var(--pi-meta);
	}
</style>
