<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { onMount } from 'svelte';

	// The room code for each course with an open Session (GET /api/room-codes,
	// staff only; see nabu/internal/httpapi/roomcode.go). Students on the roster
	// sign in with their username and this code. It changes every 10 minutes, so
	// the card refreshes itself; "Full screen" puts it on the classroom display.
	type RoomCode = {
		course_id: string;
		course: string;
		sessions: string[];
		code: string;
		expires: string;
	};

	let enabled = $state(false);
	let courses = $state<RoomCode[]>([]);
	let loaded = $state(false);
	let failed = $state(false);
	let now = $state(Date.now());
	let box = $state<HTMLElement | null>(null);

	async function load() {
		try {
			const res = await pb.send<{ enabled: boolean; courses: RoomCode[] }>('/api/room-codes', {
				method: 'GET'
			});
			enabled = res.enabled;
			courses = res.courses;
			failed = false;
		} catch {
			failed = true;
		} finally {
			loaded = true;
		}
	}

	// minutes:seconds until a code changes
	function left(expires: string): string {
		const s = Math.max(0, Math.round((Date.parse(expires) - now) / 1000));
		return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
	}

	function fullScreen() {
		box?.requestFullscreen?.().catch(() => {});
	}

	onMount(() => {
		load();
		const tick = setInterval(() => {
			now = Date.now();
			// fetch the next code as soon as the shown one changes
			if (courses.some((c) => Date.parse(c.expires) <= now)) load();
		}, 1000);
		const poll = setInterval(load, 30_000);
		return () => {
			clearInterval(tick);
			clearInterval(poll);
		};
	});
</script>

{#if loaded && enabled}
	<section class="card room" bind:this={box}>
		<header>
			<h2><i class="fa-solid fa-key"></i> Room code</h2>
			<button type="button" class="pseudo" onclick={fullScreen}>
				<i class="fa-solid fa-expand"></i> Full screen
			</button>
		</header>
		{#if failed}
			<p class="error">Could not load the room code. It will try again shortly.</p>
		{:else if courses.length === 0}
			<p class="muted">No Session is open right now, so there is no code to show.</p>
		{:else}
			<p class="how">
				Sign in at <strong>{location.host}</strong> with your username and this code:
			</p>
			{#each courses as c (c.course_id)}
				<div class="course">
					{#if courses.length > 1}<h3>{c.course}</h3>{/if}
					<p class="code" aria-label="room code">{c.code.slice(0, 3)} {c.code.slice(3)}</p>
					<p class="muted">
						{c.sessions.join(' · ')} — the code changes in {left(c.expires)}
					</p>
				</div>
			{/each}
		{/if}
	</section>
{/if}

<style>
	.room header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.room .code {
		font-size: 4rem;
		font-weight: 700;
		letter-spacing: 0.15em;
		font-variant-numeric: tabular-nums;
		margin: 0.2em 0;
	}
	.room:fullscreen {
		display: flex;
		flex-direction: column;
		justify-content: center;
		align-items: center;
		background: var(--background, #fff);
		text-align: center;
	}
	.room:fullscreen .code {
		font-size: 22vmin;
	}
	.room:fullscreen .how {
		font-size: 4vmin;
	}
	.room:fullscreen header button {
		display: none;
	}
</style>
