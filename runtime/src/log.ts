// Structured logging with an in-memory tail. Every line goes to the console (the
// operator's terminal) AND into a bounded ring buffer that GET /v1/logs serves, so
// a teacher can watch what the agents are actually doing from the dashboard during
// a lesson. One stream, two sinks — the same design as nabu's slogger.
//
// This buffer is deliberately in-memory and lossy: it is a live view for a running
// lesson, not an audit trail. Durable history is nabu's job.

export type Level = 'info' | 'warn' | 'error';

export interface Entry {
	seq: number;
	ts: string;
	level: Level;
	msg: string;
	[k: string]: unknown;
}

// Ring capacity. ~500 lines covers a lesson's worth of dispatches at a glance.
const CAP = Number(process.env.NABU_AGENT_LOG_BUFFER) || 500;

let seq = 0;
const ring: Entry[] = [];

// log records one structured event. Fields are merged flat into the entry so the
// console line and the dashboard row are the same shape.
export function log(level: Level, msg: string, fields: Record<string, unknown> = {}): void {
	seq += 1;
	const e: Entry = { seq, ts: new Date().toISOString(), level, msg, ...fields };
	ring.push(e);
	if (ring.length > CAP) ring.shift();

	const line = JSON.stringify(e);
	if (level === 'error') console.error(line);
	else if (level === 'warn') console.warn(line);
	else console.info(line);
}

export interface Tail {
	entries: Entry[];
	next: number; // pass back as `since` on the next poll
	latest: number; // highest seq held; latest - next = how far behind the reader is
	missed: number; // entries lost between `since` and the oldest retained line
}

// tail returns up to `limit` entries with seq > since, oldest-first.
//
// The cursor is what makes polling idempotent: pass `next` back as `since` and a
// line is never delivered twice. `since < 0` means "I'm starting fresh, give me the
// most recent `limit`" — the first paint of a dashboard.
//
// `missed` is non-zero when the ring wrapped past the reader's cursor. The UI must
// surface that as a gap; silently skipping lost lines would be its own quiet
// failure, which is the very thing this endpoint exists to prevent.
export function tail(since: number, limit: number): Tail {
	const lim = Math.max(1, Math.min(limit, 200));

	if (since < 0) {
		const entries = ring.slice(-lim);
		return {
			entries,
			next: entries.length ? entries[entries.length - 1].seq : seq,
			latest: seq,
			missed: 0
		};
	}

	const oldest = ring.length ? ring[0].seq : seq + 1;
	const missed = since + 1 < oldest ? oldest - since - 1 : 0;
	const entries = ring.filter((e) => e.seq > since).slice(0, lim);
	return {
		entries,
		next: entries.length ? entries[entries.length - 1].seq : since,
		latest: seq,
		missed
	};
}
