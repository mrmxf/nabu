// Static lesson content written by `nabu build` (internal/seed/build.go):
//   /content/index.json                       — published courses + lessons
//   /content/<course>/<lesson>/lesson.json    — one lesson with its slides
//
// Pages that only SHOW content (the help/guest viewer and its index) try these
// first, so they work on a static host with no Nabu server at all, then fall back
// to PocketBase. Anything per-student (sessions, group notes, jobs) stays on
// PocketBase: static slide ids are "<course>/<lesson>/<slug>", not record ids.
//
// A missing file is NOT a 404 on most hosts: single-page-app fallback serves
// index.html with status 200. So only a JSON response counts as found.
import type { CourseRec, LessonRec, SlideRec } from '$lib/types';

interface StaticCourse {
	slug: string;
	title: string;
	summary: string;
	order: number;
	logo: string;
	css: string;
	guest_access: boolean;
	help: boolean;
	lessons?: StaticLesson[];
}

interface StaticLesson {
	slug: string;
	title: string;
	summary: string;
	order: number;
	audience: string;
	logo: string;
	css: string;
}

interface StaticLessonDoc {
	course: StaticCourse;
	lesson: StaticLesson;
	slides: Omit<SlideRec, 'lesson' | 'readiness' | 'content'>[];
}

// fetchJSON returns the parsed body, or null when the file is absent (any non-2xx
// or a non-JSON body such as the SPA fallback page).
async function fetchJSON<T>(url: string): Promise<T | null> {
	try {
		const res = await fetch(url, { headers: { Accept: 'application/json' } });
		if (!res.ok || !(res.headers.get('content-type') ?? '').includes('json')) return null;
		return (await res.json()) as T;
	} catch {
		return null;
	}
}

function toCourse(c: StaticCourse): CourseRec {
	return {
		id: c.slug,
		slug: c.slug,
		title: c.title,
		order: c.order,
		summary: c.summary,
		guest_access: c.guest_access,
		logo: c.logo,
		css: c.css
	};
}

function toLesson(courseSlug: string, l: StaticLesson): LessonRec {
	return {
		id: `${courseSlug}/${l.slug}`,
		course: courseSlug,
		slug: l.slug,
		title: l.title,
		order: l.order,
		summary: l.summary,
		bucket: '',
		prefix: '',
		audience: l.audience,
		logo: l.logo,
		css: l.css
	};
}

/** The published catalogue, or null when this deployment has no static build. */
export async function staticCatalogue(): Promise<{ courses: CourseRec[]; lessons: LessonRec[] } | null> {
	const idx = await fetchJSON<{ courses: StaticCourse[] }>('/content/index.json');
	if (!idx) return null;
	return {
		courses: idx.courses.map(toCourse),
		lessons: idx.courses.flatMap((c) => (c.lessons ?? []).map((l) => toLesson(c.slug, l)))
	};
}

/** One published lesson with its slides, or null when there is no static copy. */
export async function staticLesson(
	courseSlug: string,
	lessonSlug: string
): Promise<{ course: CourseRec; lesson: LessonRec; slides: SlideRec[] } | null> {
	const doc = await fetchJSON<StaticLessonDoc>(
		`/content/${encodeURIComponent(courseSlug)}/${encodeURIComponent(lessonSlug)}/lesson.json`
	);
	if (!doc) return null;
	const lesson = toLesson(doc.course.slug, doc.lesson);
	return {
		course: toCourse(doc.course),
		lesson,
		slides: doc.slides.map((s) => ({ ...s, lesson: lesson.id, readiness: 'production', content: 'OK' }))
	};
}
