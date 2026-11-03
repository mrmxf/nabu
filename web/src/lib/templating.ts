// Render the {{.XXX}} placeholder templates stored in settings
// (auto_session_template, auto_group_template).
//
// Supported placeholders: YYYY MM DD hh mm ss (from `now`), USER, GROUPNAME.

export function renderTemplate(
	tpl: string,
	opts: { user?: string; group?: string; now?: Date } = {}
): string {
	if (!tpl) return '';
	const now = opts.now ?? new Date();
	const pad = (n: number) => String(n).padStart(2, '0');
	const vars: Record<string, string> = {
		YYYY: String(now.getFullYear()),
		MM: pad(now.getMonth() + 1),
		DD: pad(now.getDate()),
		hh: pad(now.getHours()),
		mm: pad(now.getMinutes()),
		ss: pad(now.getSeconds()),
		USER: opts.user ?? '',
		GROUPNAME: opts.group ?? ''
	};
	return tpl.replace(/\{\{\.(\w+)\}\}/g, (m, name) => vars[name] ?? m);
}
