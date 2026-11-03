<script lang="ts">
	// Inline figure widget mounted by the LMS hydrator from a {{< LMS t="figure" >}}
	// shortcode.
	let { src, alt = '', caption = '' }: { src: string; alt?: string; caption?: string } = $props();
</script>

<figure class="lms-figure">
	<img {src} {alt} />
	{#if caption}<figcaption>{caption}</figcaption>{/if}
</figure>

<style>
	.lms-figure {
		margin: 1rem 0;
	}
	.lms-figure img {
		max-width: 100%;
		border-radius: 8px;
	}
	.lms-figure figcaption {
		font-size: 0.85rem;
		color: var(--pi-meta);
		margin-top: 0.3rem;
	}
</style>
