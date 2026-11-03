import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { checkBearer } from './auth.js';
import { lookup, agentNames } from './registry.js';
import { Semaphore } from './queue.js';
import { VERSION, type DispatchRequest, type DispatchResult, type Health } from './contract.js';
import { log, tail } from './log.js';

// Default TTL cap when nabu sends ttl_s <= 0. A safety bound so the worker never runs a
// job unbounded even if the control plane omits a TTL.
const DEFAULT_TTL_S = 120;

export interface ServerOpts {
	host: string;
	port: number;
	token: string;
	capacity: number;
}

// startServer stands up the nabu worker contract: GET /healthz (open) and POST
// /v1/jobs/dispatch (bearer-guarded, TTL-bounded, concurrency-limited).
export function startServer(opts: ServerOpts): void {
	const startedAt = Date.now();
	const sem = new Semaphore(opts.capacity);

	const server = createServer((req, res) => {
		handle(req, res).catch((e) => sendJson(res, 500, { error: String(e) }));
	});

	async function handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
		if (req.method === 'GET' && req.url === '/healthz') {
			const h: Health = {
				alive: true,
				uptime_s: Math.floor((Date.now() - startedAt) / 1000),
				version: VERSION,
				capacity: opts.capacity,
				running: sem.active,
				queue_depth: sem.depth,
				agents: agentNames()
			};
			return sendJson(res, 200, h);
		}
		if (req.method === 'POST' && req.url === '/v1/jobs/dispatch') {
			return dispatch(req, res);
		}
		if (req.method === 'GET' && req.url?.startsWith('/v1/logs')) {
			return logs(req, res);
		}
		sendJson(res, 404, { error: 'not found' });
	}

	// GET /v1/logs?since=<seq>&limit=<n> — the live tail for nabu's teacher
	// dashboard. Cursor-paged: pass back `next` as `since` and no line repeats.
	// since<0 (or omitted) returns the most recent `limit` for a first paint.
	// Bearer-guarded like dispatch: log lines carry job ids and student topics.
	function logs(req: IncomingMessage, res: ServerResponse): void {
		if (!checkBearer(opts.token, req.headers['authorization'])) {
			return sendJson(res, 401, { error: 'unauthorized' });
		}
		const q = new URL(req.url ?? '', 'http://worker').searchParams;
		const since = Number(q.get('since') ?? -1);
		const limit = Number(q.get('limit') ?? 10);
		sendJson(res, 200, tail(Number.isFinite(since) ? since : -1, Number.isFinite(limit) ? limit : 10));
	}

	async function dispatch(req: IncomingMessage, res: ServerResponse): Promise<void> {
		if (!checkBearer(opts.token, req.headers['authorization'])) {
			log('warn', 'dispatch rejected: bad/missing token');
			return sendJson(res, 401, { error: 'unauthorized' });
		}
		let body: DispatchRequest;
		try {
			body = (await readJson(req)) as DispatchRequest;
		} catch {
			return sendJson(res, 400, { error: 'bad request' });
		}
		const agent = lookup(body.agent);
		if (!agent) return sendJson(res, 400, { error: `agent_unavailable: ${body.agent}` });

		const ttl = body.ttl_s && body.ttl_s > 0 ? body.ttl_s : DEFAULT_TTL_S;
		const ac = new AbortController();
		const timer = setTimeout(() => ac.abort(), ttl * 1000);
		let release: (() => void) | undefined;
		try {
			release = await sem.acquire(ac.signal); // queues here if the GPU is busy
			log('info', 'dispatch', { job: body.job_id, agent: body.agent, ttl_s: ttl });
			const html = await agent.run(body.inputs ?? {}, { jobId: body.job_id, signal: ac.signal });
			const out: DispatchResult = { result_html: html };
			sendJson(res, 200, out);
		} catch (e) {
			// 200 + error field (matches the Go worker): nabu treats a non-empty error
			// as failure, and an error containing "timeout" as a timeout event.
			const msg = ac.signal.aborted
				? 'timeout: ttl exceeded'
				: `error: ${e instanceof Error ? e.message : String(e)}`;
			sendJson(res, 200, { result_html: '', error: msg } satisfies DispatchResult);
		} finally {
			clearTimeout(timer);
			release?.();
		}
	}

	server.listen(opts.port, opts.host, () => {
		log('info', 'agent worker started', {
			addr: `${opts.host}:${opts.port}`,
			version: VERSION,
			capacity: opts.capacity,
			auth: opts.token ? 'enabled' : 'DISABLED (local dev)',
			agents: agentNames()
		});
	});
}

function sendJson(res: ServerResponse, code: number, body: unknown): void {
	const buf = Buffer.from(JSON.stringify(body));
	res.writeHead(code, { 'Content-Type': 'application/json', 'Content-Length': buf.length });
	res.end(buf);
}

function readJson(req: IncomingMessage): Promise<unknown> {
	return new Promise((resolve, reject) => {
		const chunks: Buffer[] = [];
		let size = 0;
		req.on('data', (c: Buffer) => {
			size += c.length;
			if (size > 1_000_000) {
				reject(new Error('body too large'));
				req.destroy();
				return;
			}
			chunks.push(c);
		});
		req.on('end', () => {
			try {
				resolve(JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}'));
			} catch (e) {
				reject(e);
			}
		});
		req.on('error', reject);
	});
}
