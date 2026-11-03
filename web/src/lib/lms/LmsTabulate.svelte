<script lang="ts">
	import { dataValues, type TableValue } from '$lib/stores/dataMappers.svelte';

	// Renders a t_mapper's derived rows as a table, mounted by the LMS hydrator from
	// a {{< LMS t="tabulate" from="…" >}} shortcode. Reads the shared dataValues
	// store reactively, so it updates live as the driving choice changes. Columns
	// are the mapper's (or inferred from the rows). Shows a hint until a value exists.
	let { from }: { from: string } = $props();

	let table = $derived<TableValue | undefined>(dataValues[from]);

	function cell(v: unknown): string {
		return v === null || v === undefined ? '' : String(v);
	}
</script>

{#if table && table.rows.length}
	<div class="tabulate">
		<table>
			<thead>
				<tr>{#each table.columns as c (c)}<th>{c}</th>{/each}</tr>
			</thead>
			<tbody>
				{#each table.rows as row, i (i)}
					<tr>{#each table.columns as c (c)}<td>{cell(row[c])}</td>{/each}</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p class="tabulate-empty">Make a choice above to see the list.</p>
{/if}

<style>
	.tabulate {
		overflow-x: auto;
		margin: 0.6rem 0;
	}
	.tabulate table {
		width: 100%;
		border-collapse: collapse;
	}
	.tabulate th,
	.tabulate td {
		text-align: left;
		padding: 0.35rem 0.6rem;
		border-bottom: 1px solid oklch(from var(--hi2) 88% calc(c * 0.35) h);
	}
	.tabulate th {
		font-weight: 600;
		border-bottom-width: 2px;
	}
	.tabulate-empty {
		color: var(--pi-meta);
		font-style: italic;
	}
</style>
