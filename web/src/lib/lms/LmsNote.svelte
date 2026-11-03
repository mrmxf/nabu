<script lang="ts">
	// Inline callout widget mounted by the LMS hydrator from a {{< LMS t="note" >}}
	// shortcode.
	let { tone = 'info', text = '' }: { tone?: string; text?: string } = $props();
</script>

<div class="lms-note" class:info={tone === 'info'} class:tip={tone === 'tip'} class:warning={tone === 'warning'}>
	{text}
</div>

<style>
	.lms-note {
		margin: 1rem 0;
		padding: 0.6rem 0.9rem;
		border-radius: 8px;
		border-left: 3px solid var(--hi2);
		background: oklch(from var(--hi2) 96% calc(c * 0.4) h);
	}
	.lms-note.tip {
		border-left-color: oklch(from var(--hi4) 42% c h);
		background: oklch(from var(--hi4) 96% calc(c * 0.4) h);
	}
	.lms-note.warning {
		border-left-color: var(--hi1);
		background: oklch(from var(--hi1) 96% calc(c * 0.4) h);
	}
</style>
