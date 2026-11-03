<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { login, auth } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { passwordStrength } from 'check-password-strength';
	import { loadSettings, setting } from '$lib/stores/settings.svelte';
	import PasswordInput from '$lib/components/PasswordInput.svelte';

	// Shared with the login page so a fresh sign-up prefills next time too.
	const LAST_IDENTITY_KEY = 'nabu-last-identity';

	// Password must reach at least this strength id (check-password-strength:
	// 0 Too weak · 1 Weak · 2 Medium · 3 Strong). Medium or Strong required.
	const MIN_STRENGTH = 2;

	let username = $state('');
	let name = $state('');
	let email = $state('');
	let password = $state('');
	let confirm = $state('');
	let error = $state('');
	let busy = $state(false);

	onMount(() => {
		if (auth.user) goto('/');
		// config drives whether the email field is required (signup.require_email).
		loadSettings();
	});

	// Email is optional unless nabu-config.yaml sets signup.require_email: true
	// (seeded into the signup_require_email setting). Default (missing) => optional.
	let requireEmail = $derived(setting('signup_require_email') === 'true');

	// Live strength of the current password (empty password => a neutral 'None').
	let strength = $derived(
		password ? passwordStrength(password) : { id: -1, value: 'None' }
	);
	let strongEnough = $derived(strength.id >= MIN_STRENGTH);
	let match = $derived(confirm.length > 0 && password === confirm);

	// A valid username is the handle students also sign in with: letters, digits and
	// hyphens, no spaces (e.g. s7g1-alice).
	let usernameOk = $derived(/^[a-z0-9][a-z0-9-]*$/i.test(username.trim()));

	let canSubmit = $derived(
		usernameOk && !!name.trim() && (!requireEmail || !!email.trim()) && strongEnough && match && !busy
	);

	// bar fill + colour per strength id, for the meter under the password field.
	const METER = [
		{ label: 'Too weak', cls: 'weak' },
		{ label: 'Weak', cls: 'weak' },
		{ label: 'Medium', cls: 'ok' },
		{ label: 'Strong', cls: 'strong' }
	];

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!canSubmit) return;
		busy = true;
		const uid = username.trim();
		// Trim the password so the account is created with exactly what the (also
		// trimming) login page will submit — a stray trailing space otherwise bakes
		// into the credential and then blocks every future sign-in.
		const pw = password.trim();
		try {
			// role is forced to "student" here AND enforced by the users create rule,
			// so a tampered client can't self-grant staff access. Email is only sent
			// when supplied — the schema allows email-less accounts (login via uid).
			const data: Record<string, unknown> = {
				uid,
				name: name.trim(),
				emailVisibility: false,
				password: pw,
				passwordConfirm: confirm.trim(),
				role: 'student'
			};
			if (email.trim()) data.email = email.trim();
			await pb.collection('users').create(data);
			// sign straight in and remember the handle.
			await login(uid, pw);
			try {
				localStorage.setItem(LAST_IDENTITY_KEY, uid);
			} catch {
				/* storage may be blocked; best-effort */
			}
			goto('/');
		} catch (err) {
			error = signupError(err);
			busy = false;
		}
	}

	// Map PocketBase's field validation errors to a friendly single line.
	function signupError(err: unknown): string {
		const data = (err as { response?: { data?: Record<string, { message?: string }> } })?.response
			?.data;
		if (data) {
			if (data.uid) return 'That username is already taken — pick another.';
			if (data.email) return 'That email is already registered, or is not valid.';
			if (data.password) return data.password.message || 'Password is not acceptable.';
			const first = Object.values(data)[0]?.message;
			if (first) return first;
		}
		return err instanceof Error ? err.message : 'Could not create your account';
	}
</script>

<div class="card signup">
	<h2>Create an account</h2>
	<p class="muted">Pick a username and a strong password to join the course.</p>

	<form onsubmit={submit}>
		<label for="username">Username</label>
		<input
			id="username"
			type="text"
			bind:value={username}
			autocomplete="username"
			autocapitalize="none"
			autocorrect="off"
			spellcheck="false"
			placeholder="e.g. s7g1-alice"
			required
		/>
		{#if username && !usernameOk}
			<p class="field-hint bad">Letters, digits and hyphens only — no spaces.</p>
		{/if}

		<label for="name">Display name</label>
		<input id="name" type="text" bind:value={name} autocomplete="name" required />

		<label for="email">Email {#if !requireEmail}<small>optional</small>{/if}</label>
		<input id="email" type="email" bind:value={email} autocomplete="email" required={requireEmail} />

		<label for="password">Password</label>
		<PasswordInput id="password" bind:value={password} autocomplete="new-password" required />
		{#if password}
			<div class="meter" aria-hidden="true">
				{#each [0, 1, 2, 3] as seg (seg)}
					<span
						class="seg {seg <= strength.id ? METER[strength.id].cls : ''}"
						class:filled={seg <= strength.id}
					></span>
				{/each}
			</div>
			<p class="field-hint" class:bad={!strongEnough} class:good={strongEnough}>
				Strength: {strength.value}{strongEnough ? '' : ' — needs Medium or Strong'}
			</p>
		{/if}

		<label for="confirm">Confirm password</label>
		<PasswordInput id="confirm" bind:value={confirm} autocomplete="new-password" required />
		{#if confirm && !match}
			<p class="field-hint bad">Passwords don't match.</p>
		{/if}

		{#if error}
			<p class="error">{error}</p>
		{/if}

		<button type="submit" disabled={!canSubmit}>{busy ? 'Creating…' : 'Create account'}</button>
	</form>

	<p class="muted hint">Already have an account? <a href="/login">Sign in</a>.</p>
</div>

<style>
	.signup {
		max-width: 420px;
		margin: 2rem auto;
	}
	form label {
		display: block;
		margin-top: 0.6rem;
		font-weight: 600;
	}
	form input {
		width: 100%;
	}
	form button {
		margin-top: 1rem;
		width: 100%;
	}
	.error {
		color: var(--hi1);
		margin-top: 0.6rem;
	}
	.field-hint {
		margin: 0.25rem 0 0;
		font-size: 0.85rem;
	}
	.field-hint.bad {
		color: var(--hi1);
	}
	.field-hint.good {
		color: oklch(from var(--hi4) 42% c h);
	}
	.hint {
		margin-top: 1rem;
	}
	.meter {
		display: flex;
		gap: 0.3rem;
		margin-top: 0.4rem;
	}
	.seg {
		height: 6px;
		flex: 1;
		border-radius: 3px;
		background: oklch(from var(--hi2) 90% calc(c * 0.2) h);
	}
	.seg.filled.weak {
		background: var(--hi1);
	}
	.seg.filled.ok {
		background: oklch(from var(--hi3) 70% c h);
	}
	.seg.filled.strong {
		background: oklch(from var(--hi4) 52% c h);
	}
</style>
