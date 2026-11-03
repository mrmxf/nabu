import { logoUrlFromPath, logoUrl } from '$lib/stores/settings.svelte';

// Context-aware branding for the view: the navbar logo and the authored
// stylesheets.
//
// The top-left logo cascades:
//   current lesson's logo  →  current course's logo  →  the configured default
// (app_config `site_logo`, seeded from nabu-config.yaml `logopath`).
//
// Stylesheets do NOT cascade by replacement — they LAYER, in this order:
//   nabu.css (always, from app.html)  →  course css  →  lesson css
// so a course re-themes nabu's base tokens in its _course.css and one lesson may
// override its course in _lesson.css. The layout emits both <link>s in that
// order; later sheets win at equal specificity, which is the whole point.
//
// Pages that display a course or lesson set the relevant values here (from the
// record's `logo` / `css` fields); the layout resets them on every navigation so
// nothing leaks across views.
export const branding = $state<{
	courseLogo: string;
	lessonLogo: string;
	courseCss: string;
	lessonCss: string;
}>({
	courseLogo: '',
	lessonLogo: '',
	courseCss: '',
	lessonCss: ''
});

// setCourseLogo/setLessonLogo take the raw record path (or ''), resolving it to a
// served URL. Pass '' to clear.
export function setCourseLogo(path: string | undefined): void {
	branding.courseLogo = logoUrlFromPath(path ?? '');
}

export function setLessonLogo(path: string | undefined): void {
	branding.lessonLogo = logoUrlFromPath(path ?? '');
}

// setCourseCss/setLessonCss take the record's `css` field. The seeder stores an
// already-resolved URL there (fileAssetSink returns "/content/<course>/…"), so —
// unlike a logo — there is no basename fallback to apply: a stylesheet is an
// href we either have or don't. Anything not absolute or remote is ignored
// rather than guessed at, so a half-migrated record can't inject a bad <link>.
function cssHref(value: string | undefined): string {
	const v = (value ?? '').trim();
	if (!v) return '';
	return v.startsWith('/') || v.startsWith('http://') || v.startsWith('https://') ? v : '';
}

export function setCourseCss(value: string | undefined): void {
	branding.courseCss = cssHref(value);
}

export function setLessonCss(value: string | undefined): void {
	branding.lessonCss = cssHref(value);
}

// resetBranding clears every override — the navbar falls back to the default
// logo and the slide body falls back to bare nabu.css.
export function resetBranding(): void {
	branding.courseLogo = '';
	branding.lessonLogo = '';
	branding.courseCss = '';
	branding.lessonCss = '';
}

// contentStylesheets returns the authored stylesheets to link, in cascade order
// (course then lesson). Deduplicated so a lesson that names the same sheet as its
// course doesn't emit it twice; empty when the view has no authored styling.
export function contentStylesheets(): string[] {
	return [...new Set([branding.courseCss, branding.lessonCss].filter(Boolean))];
}

// The bundled Nabu logo — the guaranteed final fallback. It's a static asset, so
// it's always present; the navbar therefore always shows *some* logo.
const NABU_LOGO = '/logo-nabu.svg';

// navLogoCandidates returns the logo URLs to try, in priority order:
//   lesson → course → configured default (site_logo) → bundled Nabu logo.
// The layout renders the first that loads and falls to the next on a load error,
// so a missing per-course/lesson image degrades gracefully instead of showing a
// broken or empty logo. Deduplicated; always non-empty (ends with NABU_LOGO).
export function navLogoCandidates(): string[] {
	const ordered = [branding.lessonLogo, branding.courseLogo, logoUrl('site_logo'), NABU_LOGO];
	return [...new Set(ordered.filter(Boolean))];
}
