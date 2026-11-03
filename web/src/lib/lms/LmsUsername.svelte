<script lang="ts">
	import { auth } from '$lib/stores/auth.svelte';

	// Renders the signed-in user's display name, reactive to login/logout. Falls
	// back to email, then a guest placeholder. `field="email"` forces the email.
	//
	// NOTE: this is a plain component used directly (e.g. the admin dashboard
	// header). Inline lesson content no longer mounts it as an LMS widget — use
	// {{< LMS t="print" from="username" >}} (or from="email") instead.
	let { field = 'name' }: { field?: string } = $props();

	const label = $derived(
		field === 'email'
			? (auth.user?.email ?? 'guest')
			: (auth.user?.name || auth.user?.email || 'guest')
	);
</script>

<span class="username">{label}</span>

<style>
	.username {
		font-weight: 600;
	}
</style>
