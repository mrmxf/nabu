// Small formatting helpers shared by agents. Agent results are HTML fragments
// nabu stores as result_html and the LmsAgent widget renders.

export function str(v: unknown): string {
	if (v === null || v === undefined) return '';
	return String(v);
}

// num coerces a config value to a finite number, falling back to dflt.
export function num(v: unknown, dflt: number): number {
	const n = Number(v);
	return Number.isFinite(n) ? n : dflt;
}

// bool coerces a config value (bool | "true"/"1"/"yes" | number) to a boolean.
export function bool(v: unknown, dflt: boolean): boolean {
	if (typeof v === 'boolean') return v;
	if (typeof v === 'number') return v !== 0;
	if (typeof v === 'string') return ['true', '1', 'yes', 'on'].includes(v.trim().toLowerCase());
	return dflt;
}

// capWords trims s to at most n words, appending an ellipsis when it had to cut.
// Enforces the controller's extra-instruction budget (default 300 words).
export function capWords(s: string, n: number): string {
	const words = s.trim().split(/\s+/).filter(Boolean);
	if (words.length <= n) return s.trim();
	return words.slice(0, n).join(' ') + ' …';
}

export function esc(s: string): string {
	return s
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;');
}

// textToHtml turns a model's plain-text reply into safe HTML: a bullet list if it
// looks like bullets, otherwise paragraphs. Everything is escaped.
export function textToHtml(text: string): string {
	const lines = text
		.split(/\r?\n/)
		.map((l) => l.trim())
		.filter(Boolean);
	if (lines.length === 0) return '';
	const bullety = lines.length > 1 && lines.every((l) => /^([-*•]|\d+[.)])\s+/.test(l));
	if (bullety) {
		return `<ul>${lines.map((l) => `<li>${esc(l.replace(/^([-*•]|\d+[.)])\s+/, ''))}</li>`).join('')}</ul>`;
	}
	return lines.map((l) => `<p>${esc(l)}</p>`).join('');
}

// offlineNote flags results produced by the deterministic fallback (no live model).
export function offlineNote(live: boolean): string {
	return live ? '' : `<p><em>Offline sample — start Ollama for live results.</em></p>`;
}
