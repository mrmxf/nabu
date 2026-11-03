import { redirect } from '@sveltejs/kit';
import { ensureAuth } from '$lib/stores/auth.svelte';

// Single-page app: no server-side rendering or prerendering. PocketBase serves
// the built index.html for every route and the client takes over.
export const ssr = false;
export const prerender = false;

// Auth guard, re-run on every navigation (it reads `url`). It validates the
// stored token against the server, so an expired token or a restarted server
// bounces the user to /login instead of showing a page that then fails to save.
export async function load({ url }) {
	const onLogin = url.pathname === '/login';
	// Signed-out visitors may reach the sign-in and sign-up pages, and the public
	// help lessons (/nabu/help — content is public-read and the viewer enforces
	// guest_access per course) without being bounced to /login.
	const onAuthPage = onLogin || url.pathname === '/signup';
	const isPublic = onAuthPage || url.pathname.startsWith('/nabu/help');
	const authed = await ensureAuth();

	if (!authed && !isPublic) {
		throw redirect(307, '/login');
	}
	// Already signed in? The auth pages have nothing to offer — send them home.
	if (authed && onAuthPage) {
		throw redirect(307, '/');
	}
	return {};
}
