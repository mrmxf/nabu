import type { DataMapper } from '$lib/types';
import { choiceValues } from '$lib/stores/decisions.svelte';

// A resolved table: the columns to show and the selected rows.
export interface TableValue {
	columns: string[];
	rows: Record<string, unknown>[];
}

// dataValues holds each t_mapper's derived output, keyed by mapper id, computed
// live from the fetched data + the reactive choiceValues store. LmsTabulate reads
// it reactively, so it updates when the driving choice changes (incl. via realtime
// group sync). See docs/lesson-choice-values.md.
export const dataValues = $state<Record<string, TableValue>>({});

// dataCache holds fetched source JSON, keyed by the source filename. Reactive so a
// completed fetch triggers recomputation. Kept across the session (data is static).
export const dataCache = $state<Record<string, unknown>>({});

// resetDataValues clears the derived outputs (call when entering/leaving a lesson).
export function resetDataValues(): void {
	for (const k of Object.keys(dataValues)) delete dataValues[k];
}

// fetchSource loads a source's served JSON once. `source` is the authored filename
// (e.g. "chooser-lists.yaml"); the assets pipeline serves it at /data/<name>.json.
export async function fetchSource(source: string): Promise<void> {
	if (source in dataCache) return;
	dataCache[source] = null; // mark in-flight so we fetch once
	const name = source.replace(/\.(ya?ml)$/i, '');
	try {
		const res = await fetch(`/data/${name}.json`);
		if (res.ok) dataCache[source] = await res.json();
	} catch {
		/* missing data serves nothing; tabulate shows its placeholder */
	}
}

// computeMapper resolves output = data[root][chosen-slug] and stores it under the
// mapper id. Columns come from the spec or the union of the selected rows' keys
// (row fields differ per topic). Reads choiceValues + dataCache reactively.
export function computeMapper(m: DataMapper): void {
	const data = dataCache[m.source] as Record<string, unknown> | null | undefined;
	const v = choiceValues[m.from];
	const idx = v ? v.slug || v.key : '';
	const picked = idx && data ? (data[m.root] as Record<string, unknown>)?.[idx] : undefined;
	if (!Array.isArray(picked)) {
		delete dataValues[m.id];
		return;
	}
	const rows = picked as Record<string, unknown>[];
	const columns = m.columns && m.columns.length ? m.columns : unionKeys(rows);
	dataValues[m.id] = { columns, rows };
}

function unionKeys(rows: Record<string, unknown>[]): string[] {
	const seen: string[] = [];
	for (const r of rows) for (const k of Object.keys(r ?? {})) if (!seen.includes(k)) seen.push(k);
	return seen;
}
