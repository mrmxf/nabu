<script lang="ts">
	import { notifications, refreshNotifications } from './notifications.svelte';
	import { usePoll, AMBIENT_MS } from '$lib/poll';

	// The crisis area at the top of /teacher. Empty is the normal state and must read
	// as reassuring rather than broken — a teacher glancing at this mid-lesson should
	// be able to tell "all fine" from "not checked yet" at a glance.
	usePoll(refreshNotifications, AMBIENT_MS);

	let busy = $state('');

	async function run(id: string, fn: () => Promise<void>) {
		busy = id;
		try {
			await fn();
		} finally {
			busy = '';
		}
	}
</script>

<section class="notices card" aria-live="polite">
	<div class="head">
		<h2><i class="fa-solid fa-bell"></i> Notifications</h2>
		<span class="spacer"></span>
		{#if notifications.checkedAt}
			<span class="muted small">checked {notifications.checkedAt.toLocaleTimeString()}</span>
		{/if}
	</div>

	{#if notifications.error}
		<p class="row err">
			<i class="fa-solid fa-triangle-exclamation"></i>
			<span>Could not check for problems — {notifications.error}</span>
		</p>
	{:else if notifications.loading}
		<p class="muted">Checking…</p>
	{:else if notifications.items.length === 0}
		<p class="ok"><i class="fa-solid fa-circle-check"></i> No notifications</p>
	{:else}
		<ul>
			{#each notifications.items as n (n.id)}
				<li class={n.severity}>
					<i
						class={n.severity === 'error'
							? 'fa-solid fa-circle-exclamation'
							: n.severity === 'warning'
								? 'fa-solid fa-triangle-exclamation'
								: 'fa-solid fa-circle-info'}
					></i>
					<div class="body">
						<strong>{n.title}</strong>
						<span class="muted">{n.detail}</span>
					</div>
					{#if n.action}
						<button
							class="pseudo act"
							disabled={busy === n.id}
							onclick={() => run(n.id, n.action!.run)}
						>
							{busy === n.id ? '…' : n.action.label}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.notices {
		border: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
		border-radius: 8px;
		padding: 1rem 1.2rem;
		margin: 1rem 0;
	}
	.head {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
	}
	.head h2 {
		margin: 0;
		font-size: 1.1rem;
	}
	.spacer {
		flex: 1;
	}
	.muted {
		color: var(--pi-meta);
	}
	.small {
		font-size: 0.85rem;
	}
	.ok {
		color: var(--pi-meta);
		margin: 0.4rem 0 0;
	}
	.err {
		color: var(--hi1);
	}
	.row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}
	ul {
		list-style: none;
		margin: 0.5rem 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	li {
		display: flex;
		gap: 0.7rem;
		align-items: flex-start;
		padding: 0.6rem 0.8rem;
		border-radius: 6px;
	}
	li .body {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	li.error {
		background: oklch(from var(--hi1) 95% calc(c * 0.5) h);
		color: oklch(from var(--hi1) 40% c h);
	}
	li.warning {
		background: oklch(from var(--hi3) 95% calc(c * 0.5) h);
		color: oklch(from var(--hi3) 42% c h);
	}
	li.info {
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
		color: oklch(from var(--hi2) 40% c h);
	}
	.act {
		margin-left: auto;
		white-space: nowrap;
	}
</style>
