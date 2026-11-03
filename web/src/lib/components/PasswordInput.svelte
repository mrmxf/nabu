<script lang="ts">
	// A native password input with a show/hide toggle pinned to the right. The
	// parent owns validation, labels and the meter — this only swaps the input
	// type between "password" and "text" so the value stays a plain two-way bind.
	let {
		value = $bindable(''),
		id,
		autocomplete = 'current-password',
		required = false
	}: {
		value?: string;
		id: string;
		autocomplete?: AutoFill;
		required?: boolean;
	} = $props();

	let show = $state(false);
</script>

<div class="pw-field">
	<input
		{id}
		type={show ? 'text' : 'password'}
		bind:value
		{autocomplete}
		{required}
		autocapitalize="none"
		autocorrect="off"
		spellcheck="false"
	/>
	<!-- tabindex=-1: the toggle is a mouse affordance, not a stop in the form's tab
	     order between the password and the next field. -->
	<button
		type="button"
		class="pw-toggle"
		aria-label={show ? 'Hide password' : 'Show password'}
		aria-pressed={show}
		title={show ? 'Hide password' : 'Show password'}
		tabindex="-1"
		onclick={() => (show = !show)}
	>
		<i class={`fa-solid ${show ? 'fa-eye-slash' : 'fa-eye'}`}></i>
	</button>
</div>

<style>
	.pw-field {
		position: relative;
	}
	.pw-field input {
		width: 100%;
		/* leave room so the value never runs under the toggle */
		padding-right: 2.5rem;
	}
	.pw-toggle {
		position: absolute;
		top: 0;
		bottom: 0;
		right: 0;
		width: 2.5rem;
		background: none;
		border: 0;
		padding: 0;
		margin: 0;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--muted, currentColor);
		opacity: 0.7;
	}
	.pw-toggle:hover {
		opacity: 1;
	}
</style>
