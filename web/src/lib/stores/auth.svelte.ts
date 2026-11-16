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

// requestSignInCode asks Nabu to email a sign-in code (POST /api/auth/otp, see
// nabu/internal/httpapi/otp.go). identity is a username or an email; the code
// always goes to the email on record. The reply is an otpId whether or not the
// account exists, so the page cannot tell — and must not say — which it was.
export async function requestSignInCode(identity: string): Promise<string> {
	const res = await pb.send<{ otpId: string }>('/api/auth/otp', {
		method: 'POST',
		body: { identity }
	});
	return res.otpId;
}

// loginWithCode finishes a code sign-in with PocketBase's own auth-with-otp,
// which limits guesses and populates the auth store like any other sign-in.
export async function loginWithCode(otpId: string, code: string): Promise<void> {
	await pb.collection('users').authWithOTP(otpId, code);
}

// signInCodesEnabled reports whether the users collection accepts emailed codes.
export async function signInCodesEnabled(): Promise<boolean> {
	try {
		const methods = await pb.collection('users').listAuthMethods();
		return !!methods.otp?.enabled;
	} catch {
		return false;
	}
}

// roomCodesEnabled reports whether students may sign in with the code on the
// teacher's screen (GET /api/auth/room-code; see nabu/internal/httpapi/roomcode.go).
export async function roomCodesEnabled(): Promise<boolean> {
	try {
		const res = await pb.send<{ enabled: boolean }>('/api/auth/room-code', { method: 'GET' });
		return !!res.enabled;
	} catch {
		return false;
	}
}

// loginWithRoomCode signs a roster student in with their username (or email)
// and the room code. The server answers like authWithPassword ({token,
// record}), so the auth store is populated the same way.
export async function loginWithRoomCode(identity: string, code: string): Promise<void> {
	const res = await pb.send<{ token: string; record: RecordModel }>('/api/auth/room-code', {
		method: 'POST',
		body: { identity, code }
	});
	pb.authStore.save(res.token, res.record);
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
