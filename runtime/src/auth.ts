import { timingSafeEqual } from 'node:crypto';

// checkBearer verifies the Authorization header against the shared dispatch token
// (WORKER_DISPATCH_TOKEN). An empty token means auth is disabled (local dev). The
// comparison is constant-time; the token is never logged. Mirrors the Go worker.
export function checkBearer(token: string, header: string | undefined): boolean {
	if (!token) return true; // open — local dev only
	const want = Buffer.from('Bearer ' + token);
	const got = Buffer.from(header ?? '');
	// timingSafeEqual requires equal lengths; a length mismatch is an early reject
	// but the content compare below stays constant-time for equal-length inputs.
	if (got.length !== want.length) return false;
	return timingSafeEqual(got, want);
}
