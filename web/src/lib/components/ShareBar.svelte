<script lang="ts">
	import { setting } from '$lib/stores/settings.svelte';

	// Student-facing share/outbound actions for a help page: copy the deep link,
	// share it on WhatsApp, and a "review us on GitHub" CTA. The link is the
	// current URL (including the #slide hash), so it deep-links the exact page.
	// The GitHub target is configured via the `github_review_url` app_config key
	// (the repo is on GitLab, so this is intentionally not hardcoded).
	let { title = '' }: { title?: string } = $props();

	let copied = $state(false);
	const reviewUrl = $derived(setting('github_review_url'));

	function currentUrl(): string {
		return typeof location !== 'undefined' ? location.href : '';
	}

	async function copyLink() {
		try {
			await navigator.clipboard.writeText(currentUrl());
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			/* clipboard blocked; ignore */
		}
	}

	const whatsappHref = $derived.by(() => {
		const text = `${title ? title + ' — ' : ''}${currentUrl()}`;
		return `https://wa.me/?text=${encodeURIComponent(text)}`;
	});
</script>

<div class="share">
	<span class="label"><i class="fa-solid fa-share-nodes"></i> Share this page</span>
	<button class="pseudo" onclick={copyLink} title="Copy link to this page">
		<i class="fa-solid {copied ? 'fa-check' : 'fa-link'}"></i>
		{copied ? 'Copied' : 'Copy link'}
	</button>
	<a class="pseudo button wa" href={whatsappHref} target="_blank" rel="noopener">
		<i class="fa-brands fa-whatsapp"></i> WhatsApp
	</a>
	{#if reviewUrl}
		<a class="pseudo button gh" href={reviewUrl} target="_blank" rel="noopener">
			<i class="fa-solid fa-star"></i> Review us on GitHub
		</a>
	{/if}
</div>

<style>
	.share {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
		margin-top: 1.2rem;
		padding-top: 0.8rem;
		border-top: 1px dashed oklch(from var(--hi2) 85% calc(c * 0.4) h);
	}
	.label {
		font-weight: 600;
		color: var(--pi-meta);
		margin-right: 0.2rem;
	}
	.share :global(a.button),
	.share button {
		font-size: 0.85rem;
	}
	.wa {
		color: oklch(from var(--hi4) 48% c h);
	}
	.gh {
		color: var(--nabu-hi-student, oklch(from var(--hi4) 42% c h));
	}
</style>
