// The three documentation personas. `audience` frontmatter on a lesson/slide names
// one of these (or "all"); the icon + highlight colour drive PersonaBadge and the
// help index filter. Colours are the --nabu-hi-* tokens in static/nabu.css.

export type Persona = 'student' | 'teacher' | 'admin';

export interface PersonaInfo {
	id: Persona;
	label: string;
	icon: string; // FontAwesome 7 course
	colorVar: string; // CSS custom property
}

export const personas: PersonaInfo[] = [
	{ id: 'student', label: 'Student', icon: 'fa-user-graduate', colorVar: '--nabu-hi-student' },
	{ id: 'teacher', label: 'Teacher', icon: 'fa-chalkboard-user', colorVar: '--nabu-hi-teacher' },
	{ id: 'admin', label: 'Admin', icon: 'fa-user-gear', colorVar: '--nabu-hi-admin' }
];

const byId: Record<string, PersonaInfo> = Object.fromEntries(personas.map((p) => [p.id, p]));

// resolve returns the PersonaInfo for an audience value, or null for "all"/unknown.
export function resolvePersona(audience: string | undefined): PersonaInfo | null {
	return audience ? (byId[audience] ?? null) : null;
}

// matchesFilter reports whether an item's audience should show under a chosen
// persona filter. "all" content always shows; an empty filter shows everything.
export function matchesFilter(audience: string, filter: Persona | ''): boolean {
	if (!filter) return true;
	return audience === filter || audience === 'all';
}
