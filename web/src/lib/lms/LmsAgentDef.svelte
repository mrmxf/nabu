<script lang="ts">
	import { choiceValues } from '$lib/stores/decisions.svelte';
	import type { AgentJob } from '$lib/types';

	// Inline echo of one field of a config-driven agent's CHOSEN profile, mounted by
	// the LMS hydrator from a {{< LMS t="agentdef" id="…" field="…" >}} shortcode. The
	// active profile follows the group's live choice (e.g. the controller kind
	// pip|orca|buzz), read reactively from the shared choiceValues store, so the shown
	// text switches when the group changes controller. Renders a hint until the choice
	// is made. See model.AgentJob.Profiles + LmsAgent's dispatchAgent resolution.
	let { job, field = 'system' }: { job: AgentJob; field?: string } = $props();

	// resolveKey mirrors LmsAgent.resolveAgentToken: a {{choice:ID}} in the agent
	// field resolves to the option KEY (pip|orca|buzz), which is the profiles map key.
	function resolveKey(s: string): string {
		return s.replace(/\{\{\s*(?:choice|field):([^}]+?)\s*\}\}/g, (_, id) => {
			const v = choiceValues[id.trim()];
			return v ? (v.key ?? v.slug ?? v.label ?? '') : '';
		});
	}

	const kind = $derived(resolveKey(job.agent));
	const text = $derived(job.profiles?.[kind]?.[field] ?? '');
</script>

{#if text}
	<pre class="agentdef">{text}</pre>
{:else}
	<p class="agentdef-empty">Pick your controller above to see its definition.</p>
{/if}

<style>
	.agentdef {
		margin: 0;
		white-space: pre-wrap;
		word-break: break-word;
		font-family: var(--nabu-font-mono, ui-monospace, monospace);
		font-size: 0.85rem;
		line-height: 1.5;
	}
	.agentdef-empty {
		margin: 0;
		color: var(--pi-meta);
		font-style: italic;
	}
</style>
