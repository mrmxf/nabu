// Turns a failed fetch Response into a clean Error message for the UI.
//
// nabu's own error replies are short JSON ({"message": "..."}) or plain text.
// But a request can also fail at the edge — Cloudflare returns a full HTML page
// for gateway errors (502/504) and origin timeouts (524). Rendering that body
// verbatim dumps a wall of HTML at the teacher. This helper detects those cases
// and returns a friendly, actionable message instead, and only ever surfaces a
// response body when it is short, non-HTML text.

// Well-known gateway/proxy statuses that mean "nabu never answered", with a
// human explanation. 524 is Cloudflare-specific (origin timeout).
const GATEWAY_MESSAGES: Record<number, string> = {
	502: 'The server is unavailable right now (bad gateway). Please try again.',
	503: 'The server is temporarily unavailable. Please try again shortly.',
	504: 'The server timed out. The file may be too large or the source too slow — try again, or use a direct file upload.',
	524: 'The server took too long to respond and the connection timed out. The file may be too large or the source too slow — try again, or use a direct file upload.'
};

// httpError reads a non-ok Response and returns an Error suitable for display.
// Always call this instead of `throw new Error(await res.text())`.
export async function httpError(res: Response, fallback = 'Request failed'): Promise<Error> {
	const gateway = GATEWAY_MESSAGES[res.status];
	if (gateway) return new Error(gateway);

	let body = '';
	try {
		body = (await res.text()).trim();
	} catch {
		/* body unavailable */
	}

	// An HTML body means an edge/proxy page slipped through with an unexpected
	// status — never show it; fall back to a status-based message.
	const looksHtml = /^\s*(<!doctype|<html)/i.test(body);
	if (body && !looksHtml) {
		// nabu JSON errors are {"message": "..."} — prefer that field.
		try {
			const parsed = JSON.parse(body);
			if (parsed && typeof parsed.message === 'string' && parsed.message) {
				return new Error(parsed.message);
			}
		} catch {
			/* not JSON; use the raw text below */
		}
		// Cap plain-text bodies so a stray large response can't flood the UI.
		return new Error(body.length > 300 ? `${body.slice(0, 300)}…` : body);
	}

	return new Error(`${fallback} (${res.status})`);
}
