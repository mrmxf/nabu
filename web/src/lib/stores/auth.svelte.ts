import { pb } from '$lib/pocketbase';
import { goto } from '$app/navigation';
import type { RecordModel } from 'pocketbase';

// Reactive auth state backed by the PocketBase auth store. Components read
// auth.user; it updates whenever the token changes (login/logout/refresh).
export const auth = $state<{ user: RecordModel | null }>({
	user: pb.authStore.record ?? null
});

pb.authStore.onChange(() => {
	auth.user = pb.authStore.record ?? null;
});

// identity is a username (uid, e.g. s7g1-alice) or an email — both are configured
// as passwordAuth identity fields on the users collection.
export async function login(identity: string, password: string): Promise<void> {
	await pb.collection('users').authWithPassword(identity, password);
}

// classroomFastLogin signs a student in by name alone via POST
// /api/classroom/login (see nabu/internal/httpapi/classroom.go). The server
// creates the account on first use and mints the token, returning the same
// {token, record} body authWithPassword does — so the auth store is populated
// identically and everything downstream is unaware this path exists. Only works
// while the classroom_fast_login flag is on and the Session is enrollable;
// otherwise the request throws like any other failed sign-in.
export async function classroomFastLogin(username: string, sessionId: string): Promise<void> {
	const res = await pb.send<{ token: string; record: RecordModel }>('/api/classroom/login', {
		method: 'POST',
		body: { username, session_id: sessionId }
	});
	pb.authStore.save(res.token, res.record);
}

export function logout(): void {
	pb.authStore.clear();
}

// True only when there is a locally-valid (non-expired) token.
export function isAuthed(): boolean {
	return pb.authStore.isValid;
}

// True when the signed-in user has the app-level admin role. (PocketBase
// superusers manage via /_/; the SPA admin page is for role=admin users.)
export function isAdmin(): boolean {
	return auth.user?.role === 'admin';
}

// True for staff — teachers and admins. Gates the teacher dashboards; mirrors the
// server-side requireStaff check.
export function isStaff(): boolean {
	return auth.user?.role === 'teacher' || auth.user?.role === 'admin';
}

// Confirm we are still signed in: the token must be locally valid AND accepted
// by the server (which catches a restarted/reset server that invalidated it).
// Clears auth on failure. Used by the root layout guard on every page load.
export async function ensureAuth(): Promise<boolean> {
	if (!pb.authStore.isValid) {
		return false;
	}
	try {
		await pb.collection('users').authRefresh();
		return true;
	} catch {
		logout();
		return false;
	}
}

// If an error is an auth failure (or the token is no longer valid), sign out and
// bounce to the login page. Returns true when it handled the error so callers can
// stop. Use in catch blocks around API mutations.
export function handleAuthError(err: unknown): boolean {
	const status = (err as { status?: number } | null)?.status;
	if (!pb.authStore.isValid || status === 401 || status === 403) {
		logout();
		goto('/login');
		return true;
	}
	return false;
}
