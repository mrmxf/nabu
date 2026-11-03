import { pb } from '$lib/pocketbase';

// Client half of "classroomFastLogin" — the name-only classroom sign-in (see
// nabu/internal/httpapi/classroom.go and docs/classroom-fast-login.md). The
// login page reads the status to decide whether to show the fast form, and the
// group page reads back which Session was picked. The server is authoritative
// for both the flag and the enrollable set; this module only caches its answer.

// One currently-enrollable Session, flattened with its lesson/course (mirrors the
// Go classroomSession struct).
export interface ClassroomSession {
	id: string; // schedule_sessions id
	name: string; // teacher-facing Session label
	slug: string; // uid-safe form — the suffix on every account for this Session
	course_id: string;
	course: string; // course title
	lesson_id: string;
	lesson: string; // lesson title
}

export interface ClassroomStatus {
	enabled: boolean;
	sessions: ClassroomSession[];
}

// Which Session the student picked at login. sessionStorage, not localStorage:
// it belongs to this tab's sign-in, and a new lesson next week must not inherit it.
const SESSION_KEY = 'nabu-classroom-session';
// The name they typed, remembered so a returning student in the same room
// doesn't retype it. Never a credential — there is no password on this path.
const NAME_KEY = 'nabu-classroom-name';

// Cached because both the login page and the group page ask for it; `reload`
// forces a refetch (the enrolment window can close between the two).
let cached: ClassroomStatus | null = null;

export async function loadClassroomStatus(reload = false): Promise<ClassroomStatus> {
	if (cached && !reload) return cached;
	try {
		cached = await pb.send<ClassroomStatus>('/api/classroom/status', { method: 'GET' });
	} catch {
		// Best-effort: an unreachable endpoint must fall back to the normal login
		// form rather than stranding everyone on a broken fast-login panel.
		cached = { enabled: false, sessions: [] };
	}
	return cached;
}

// Storage may be blocked (private mode / locked-down school browsers); every
// accessor below degrades to "not remembered" rather than throwing.
export function setClassroomSessionId(id: string): void {
	try {
		sessionStorage.setItem(SESSION_KEY, id);
	} catch {
		/* remembering is best-effort */
	}
}

export function classroomSessionId(): string {
	try {
		return sessionStorage.getItem(SESSION_KEY) ?? '';
	} catch {
		return '';
	}
}

export function setClassroomName(name: string): void {
	try {
		localStorage.setItem(NAME_KEY, name);
	} catch {
		/* remembering is best-effort */
	}
}

export function classroomName(): string {
	try {
		return localStorage.getItem(NAME_KEY) ?? '';
	} catch {
		return '';
	}
}
