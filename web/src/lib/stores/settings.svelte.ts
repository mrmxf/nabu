import { pb } from '$lib/pocketbase';

interface SettingRec {
	key: string;
	value: string;
}

// Global config from the `settings` collection, loaded once. Reactive so
// components re-render when it arrives.
export const settings = $state<{ map: Record<string, string>; loaded: boolean }>({
	map: {},
	loaded: false
});

let loading: Promise<void> | null = null;

export function loadSettings(): Promise<void> {
	if (settings.loaded) return Promise.resolve();
	if (!loading) {
		loading = pb
			.collection('app_config')
			.getFullList<SettingRec>()
			.then((rows) => {
				settings.map = Object.fromEntries(rows.map((r) => [r.key, r.value]));
				settings.loaded = true;
			})
			.catch(() => {
				// settings are best-effort; fall back to empty on failure.
				settings.loaded = true;
			});
	}
	return loading;
}

export function setting(key: string): string {
	return settings.map[key] ?? '';
}

// Whether the image agents' "make an image" (Stable Diffusion) surface is exposed. Sourced
// from the app_config `images_enabled` row (seeded from nabu-config.yaml
// `ai.stable-diffusion.enabled`). Off unless explicitly "true" — so every image
// surface (the image slide, the teacher image-budget card, the "make an
// image" widget) stays hidden until the image back-end is turned on. Mirrored by
// the job API, which refuses image submits while disabled.
export function imagesEnabled(): boolean {
	return setting('images_enabled') === 'true';
}

// Resolve a logo value to a served URL. Two shapes are supported:
//   • an already-served path — starts with "/" (e.g. a copied bundle asset
//     "/content/my-course/logo.png") or "http" — used verbatim;
//   • a repo path or bare filename ("…/web/static/x.svg", "logo-nabu.svg") —
//     mapped to its basename served from the site root (/x.svg).
// Returns '' when unset. Shared by settings logos and per-course/lesson overrides.
export function logoUrlFromPath(value: string): string {
	if (!value) return '';
	if (value.startsWith('/') || value.startsWith('http://') || value.startsWith('https://')) {
		return value;
	}
	const base = value.split('/').pop();
	return base ? `/${base}` : '';
}

// logoUrl resolves a named settings logo (site_logo / course_logo / lesson_logo).
export function logoUrl(key: string): string {
	return logoUrlFromPath(setting(key));
}
