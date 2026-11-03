<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { pb } from '$lib/pocketbase';
	import type { TimerSegment, SessionTimerRec } from '$lib/types';

	// A cohort-wide synchronized countdown. Unlike CountdownTimer (which starts a
	// fresh clock in every browser), this reads one shared session_timers row so a
	// teacher and every student see the SAME remaining time. The server stores an
	// absolute end_time; we count down to it, correcting for clock skew with an
	// offset measured against the server's `now` at load. The teacher controls
	// pause/resume/extend/reset by writing the row — we follow it over realtime.
	//
	// A solo/unscheduled attempt has no cohort to share with: `ensure` reports
	// shared:false and we fall back to a private local countdown from the authored
	// duration, so the lesson still shows a timer.
	let { attemptId, timer }: { attemptId: string; timer: TimerSegment } = $props();

	// serverOffset = serverNow - clientNow, so serverNow() ≈ Date.now() + offset.
	let serverOffset = $state(0);
	let running = $state(true);
	let endTimeMs = $state(0); // absolute deadline (ms since epoch), authoritative while running
	let frozenMs = $state(0); // remaining time while paused
	let local = $state(false); // solo fallback: count down purely client-side

	let remaining = $state(0); // seconds shown (seeded from the authored duration on mount)
	let rowUnsub: (() => void) | undefined;
	let handle: ReturnType<typeof setInterval> | undefined;

	function applyRow(r: Pick<SessionTimerRec, 'running' | 'end_time' | 'remaining_ms'>) {
		running = r.running;
		endTimeMs = r.end_time ? Date.parse(r.end_time) : 0;
		frozenMs = r.remaining_ms ?? 0;
	}

	function tick() {
		let ms: number;
		if (local) {
			ms = endTimeMs - Date.now();
		} else if (running) {
			ms = endTimeMs - (Date.now() + serverOffset);
		} else {
			ms = frozenMs;
		}
		// Not clamped at zero: a running timer keeps counting into negative
		// (overtime) so everyone sees how far a lesson has overrun.
		remaining = Math.round(ms / 1000);
	}

	onMount(async () => {
		remaining = timer.seconds; // show the authored duration until the row resolves
		try {
			const res = await pb.send<{
				shared: boolean;
				id?: string;
				running?: boolean;
				end_time?: string;
				remaining_ms?: number;
				server_now?: string;
			}>('/api/session/timer/ensure', {
				method: 'POST',
				body: {
					session_id: attemptId,
					timer_id: timer.id,
					seconds: timer.seconds,
					label: timer.label
				}
			});

			if (!res.shared || !res.id) {
				// solo/unscheduled: private local countdown from the authored duration.
				local = true;
				endTimeMs = Date.now() + timer.seconds * 1000;
			} else {
				serverOffset = res.server_now ? Date.parse(res.server_now) - Date.now() : 0;
				applyRow({
					running: res.running ?? true,
					end_time: res.end_time ?? '',
					remaining_ms: res.remaining_ms ?? 0
				});
				try {
					rowUnsub = await pb.collection('session_timers').subscribe(res.id, (e) => {
						applyRow(e.record as unknown as SessionTimerRec);
					});
				} catch {
					// realtime is optional; the initial end_time still counts down correctly.
				}
			}
		} catch {
			// couldn't reach the server: degrade to a local countdown rather than nothing.
			local = true;
			endTimeMs = Date.now() + timer.seconds * 1000;
		}
		tick();
		handle = setInterval(tick, 250);
	});

	onDestroy(() => {
		clearInterval(handle);
		rowUnsub?.();
	});

	function fmt(s: number): string {
		const neg = s < 0;
		const a = Math.abs(s);
		const h = Math.floor(a / 3600);
		const m = Math.floor((a % 3600) / 60);
		const sec = a % 60;
		return `${neg ? '-' : ''}${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`;
	}

	let overtime = $derived(remaining < 0); // past zero: lesson has overrun
	let paused = $derived(!local && !running);
	let urgent = $derived(!overtime && !paused && remaining <= 60);
</script>

<div class="timer" class:urgent class:overtime class:paused>
	<span class="clock">⏱️</span>
	<span class="seg-label">{timer.label}</span>
	<span class="time">{fmt(remaining)}</span>
	{#if overtime}<span class="of">over</span>{:else if paused}<span class="of">paused</span>{/if}
</div>

<style>
	.timer {
		display: inline-flex;
		align-items: baseline;
		gap: 0.5rem;
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 0.35rem 0.8rem;
		font-variant-numeric: tabular-nums;
	}
	.clock {
		font-size: 1.1rem;
	}
	.seg-label {
		font-weight: 600;
		color: var(--pi-body);
	}
	.time {
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--hi2);
		letter-spacing: 0.02em;
	}
	.of {
		font-size: 0.75rem;
		color: var(--pi-meta);
	}
	.timer.urgent {
		background: oklch(from var(--hi1) 96% calc(c * 0.4) h);
		border-color: oklch(from var(--hi1) 88% calc(c * 0.5) h);
	}
	.timer.urgent .time {
		color: var(--hi1);
	}
	.timer.paused {
		background: oklch(from var(--pi-body) 96% 0 h);
	}
	.timer.paused .time {
		color: var(--pi-meta);
	}
	.timer.overtime {
		background: oklch(from var(--hi1) 94% calc(c * 0.5) h);
		border-color: var(--hi1);
	}
	.timer.overtime .time,
	.timer.overtime .of {
		color: var(--hi1);
	}
</style>
