<script lang="ts">
	import { login, classroomFastLogin, auth } from '$lib/stores/auth.svelte';
	import {
		loadClassroomStatus,
		setClassroomSessionId,
		setClassroomName,
		classroomName,
		type ClassroomSession
	} from '$lib/stores/classroom.svelte';
	import { pb } from '$lib/pocketbase';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import PasswordInput from '$lib/components/PasswordInput.svelte';

	// Remembered across visits so a returning student doesn't retype their handle.
	// Only the identity (username/email) is stored — never the password.
	const LAST_IDENTITY_KEY = 'nabu-last-identity';

	let identity = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);
	let loading = $state(true);

	// Demo-account status (GET /api/demo/status). demoMode drives whether the demo
	// hints are advertised; leftoverDemo flags demo accounts still registered while
	// demo mode is off — a misconfiguration the operator should clean up.
	let demoMode = $state(true);
	let leftoverDemo = $state<string[]>([]);
	// well-known demo sign-ins from the demo roster (see /api/demo/status); demo
	// rosters conventionally share one password, so the hint shows the first.
	let demoLogins = $state<{ email?: string; uid?: string; password: string }[]>([]);

	// Classroom fast login (GET /api/classroom/status). When the flag is on this
	// page leads with a single name box; the normal password form is still one
	// click away behind "Staff sign-in". See docs/classroom-fast-login.md.
	let classroomEnabled = $state(false);
	let classroomSessions = $state<ClassroomSession[]>([]);
	let classroomSessionId = $state('');
	let studentName = $state('');
	// Which form is on screen. Defaults to the fast one whenever it is available;
	// the staff toggle flips it and it never flips back on its own.
	let fastMode = $state(false);

	// The Session a fast login would join: the picked one, else the only one.
	let pickedSession = $derived(
		classroomSessions.find((s) => s.id === classroomSessionId) ?? classroomSessions[0] ?? null
	);
	// No open Session => the name box is disabled (there is nothing to join).
	let classroomOpen = $derived(classroomEnabled && classroomSessions.length > 0);

	onMount(async () => {
		// already signed in? skip the form.
		if (auth.user) {
			goto('/');
			return;
		}
		// find out whether this is a demo deployment (before prefilling demo creds)
		// and whether a classroom fast login is on offer.
		const [demo, classroom] = await Promise.all([
			pb
				.send<{
					demo_mode: boolean;
					present: string[];
					logins?: { email?: string; uid?: string; password: string }[];
				}>('/api/demo/status', { method: 'GET' })
				.catch(() => null),
			loadClassroomStatus(true)
		]);
		if (demo) {
			demoMode = demo.demo_mode;
			leftoverDemo = demo.demo_mode ? [] : (demo.present ?? []);
			demoLogins = demo.logins ?? [];
		}
		classroomEnabled = classroom.enabled;
		classroomSessions = classroom.sessions;
		classroomSessionId = classroom.sessions[0]?.id ?? '';
		fastMode = classroom.enabled;
		studentName = classroomName();
		// prefill the last-used username/email; on a demo deployment fall back to a
		// demo account, otherwise leave it blank.
		identity =
			localStorage.getItem(LAST_IDENTITY_KEY) ||
			(demoMode ? (demoLogins[0]?.email ?? demoLogins[0]?.uid ?? '') : '');
		loading = false;
	});

	// Fast login: name only. The server resolves (or creates) the account for
	// (name, Session) and mints the token — there is no password on this path.
	async function joinCourse(e: SubmitEvent) {
		e.preventDefault();
		if (!pickedSession) return;
		error = '';
		busy = true;
		const name = studentName.trim();
		try {
			await classroomFastLogin(name, pickedSession.id);
			// hand the chosen Session to the group page, which needs its lesson/course.
			setClassroomSessionId(pickedSession.id);
			setClassroomName(name);
			goto('/classroom/group');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Could not join the lesson';
		} finally {
			busy = false;
		}
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		// Trim stray whitespace (autofill/paste/mobile keyboards often append a
		// space) — a trailing space otherwise silently fails the sign-in. Reflect
		// the trimmed identity back into the field so what's remembered is clean.
		identity = identity.trim();
		const pw = password.trim();
		try {
			await login(identity, pw);
			// remember the handle only after a successful sign-in.
			try {
				localStorage.setItem(LAST_IDENTITY_KEY, identity);
			} catch {
				/* storage may be blocked (private mode); remembering is best-effort */
			}
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Sign in failed';
		} finally {
			busy = false;
		}
	}
</script>

<div class="card login">
	{#if leftoverDemo.length > 0}
		<div class="warn" role="alert">
			<strong>⚠ Demo accounts still present.</strong>
			Demo mode is <em>off</em>, but {leftoverDemo.length} demo account{leftoverDemo.length === 1
				? ''
				: 's'} from an earlier demo seed remain registered ({leftoverDemo.join(', ')}). Remove
			them before going live — they use well-known passwords.
		</div>
	{/if}

	{#if loading}
		<p class="muted">Loading…</p>
	{:else if fastMode}
		<!-- classroomFastLogin: one field, no password. Only reachable while the
		     classroom_fast_login flag is on (the server enforces it too). -->
		<h2>Join today's lesson</h2>

		<form onsubmit={joinCourse}>
			{#if !classroomOpen}
				<p class="blocked">No course is open right now — ask your teacher.</p>
			{:else if classroomSessions.length === 1}
				<p class="chosen">{pickedSession?.lesson} — {pickedSession?.name}</p>
			{:else}
				<label for="classroom-session">Which lesson?</label>
				<select id="classroom-session" bind:value={classroomSessionId}>
					{#each classroomSessions as s (s.id)}
						<option value={s.id}>{s.lesson} — {s.name}</option>
					{/each}
				</select>
			{/if}

			<label for="student-name">Your name</label>
			<input
				id="student-name"
				type="text"
				bind:value={studentName}
				autocomplete="name"
				autocapitalize="words"
				autocorrect="off"
				spellcheck="false"
				placeholder="e.g. Sam"
				disabled={!classroomOpen}
				required
			/>

			{#if error}
				<p class="error">{error}</p>
			{/if}

			<button type="submit" disabled={busy || !classroomOpen || !studentName.trim()}>
				{busy ? 'Joining…' : 'Join'}
			</button>
		</form>

		<p class="muted staff-link">
			Teacher or admin?
			<button type="button" class="pseudo" onclick={() => ((fastMode = false), (error = ''))}>
				Staff sign-in
			</button>
		</p>
	{:else}
		<h2>Sign in</h2>
		{#if demoMode}
			<p class="muted">Use a seeded demo account to explore the course.</p>
		{/if}

		<form onsubmit={submit}>
			<label for="identity">Username or email</label>
			<!-- type=text, not email: a username (e.g. s7g1-alice) is a valid identity too. -->
			<input
				id="identity"
				type="text"
				bind:value={identity}
				autocomplete="username"
				autocapitalize="none"
				autocorrect="off"
				spellcheck="false"
				required
			/>

			<label for="password">Password</label>
			<PasswordInput
				id="password"
				bind:value={password}
				autocomplete="current-password"
				required
			/>

			{#if error}
				<p class="error">{error}</p>
			{/if}

			<button type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
		</form>

		<p class="muted signup-link">New here? <a href="/signup">Create an account</a>.</p>

		{#if classroomEnabled}
			<p class="muted staff-link">
				Here for a lesson?
				<button type="button" class="pseudo" onclick={() => ((fastMode = true), (error = ''))}>
					Join with your name
				</button>
			</p>
		{/if}
	{/if}

	<!-- demo credentials are only useful on the password form -->
	{#if demoMode && !fastMode && !loading && demoLogins.length}
		<p class="muted hint">
			Demo logins:
			{#each demoLogins as d, i (d.uid ?? d.email)}{#if i}, {/if}<code>{d.email ?? d.uid}</code>{/each}
			— password <code>{demoLogins[0].password}</code>.
		</p>
	{/if}
</div>

<style>
	.login {
		max-width: 420px;
		margin: 2rem auto;
	}
	form label {
		display: block;
		margin-top: 0.6rem;
		font-weight: 600;
	}
	form input,
	form select {
		width: 100%;
	}
	/* the single enrollable Session, shown as a statement rather than a choice */
	.chosen {
		margin: 0.3rem 0 0;
		font-weight: 600;
	}
	.blocked {
		margin: 0.3rem 0 0;
		color: var(--hi1);
	}
	/* the toggle between the classroom form and the staff password form */
	.staff-link {
		margin-top: 1rem;
	}
	.staff-link button {
		width: auto;
		margin: 0;
		padding: 0;
		font-weight: 800;
		text-decoration: underline;
	}
	form button {
		margin-top: 1rem;
		width: 100%;
	}
	.error {
		color: var(--hi1);
		margin-top: 0.6rem;
	}
	.warn {
		/* --hi3 is the semantic "warning" hue (see static/nabu.css). */
		margin-bottom: 1rem;
		padding: 0.7rem 0.9rem;
		border-radius: 6px;
		background: oklch(from var(--hi3) 95% calc(c * 0.5) h);
		color: oklch(from var(--hi3) 42% c h);
		font-size: 0.9rem;
		line-height: 1.4;
	}
	.signup-link {
		margin-top: 1rem;
	}
	.signup-link a {
		font-weight: 800;
		text-decoration: underline;
	}
	.hint {
		margin-top: 1rem;
	}
</style>
