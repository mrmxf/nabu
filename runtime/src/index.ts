// The nabu agent runtime — the generic core of a worker that speaks the nabu
// worker contract (/healthz, /v1/jobs/dispatch). A course supplies its agents and
// registers them before serving:
//
//   import { register, toAgent, serveFromEnv } from '@mrmxf/nabu-runtime';
//   register(toAgent(['hello'], 'hello', async (inputs, ctx) => ...));
//   serveFromEnv();
//
// Env (read by serveFromEnv):
//   WORKER_ADDR            listen addr (host:port), default 127.0.0.1:8091
//   WORKER_DISPATCH_TOKEN  shared bearer; empty => open (local dev only)
//   NABU_AGENT_CAPACITY           max concurrent agent runs (GPU budget), default 2
//   model/ollama.ts and model/image.ts document their own OLLAMA_* / NABU_AGENT_* knobs.
import { startServer } from './server.js';
import { modelInfo } from './model/ollama.js';
import { imageInfo } from './model/image.js';
import { log } from './log.js';

export * from './contract.js';
export * from './registry.js';
export * from './server.js';
export * from './queue.js';
export * from './auth.js';
export * from './log.js';
export * from './util.js';
export * from './model/ollama.js';
export * from './model/image.js';

/** parseAddr splits a host:port listen address, defaulting to 127.0.0.1:8091. */
export function parseAddr(addr: string): { host: string; port: number } {
	const i = addr.lastIndexOf(':');
	if (i < 0) return { host: '127.0.0.1', port: Number(addr) || 8091 };
	const host = addr.slice(0, i) || '127.0.0.1';
	return { host, port: Number(addr.slice(i + 1)) || 8091 };
}

/**
 * serveFromEnv logs the model configuration and starts the contract server with
 * settings read from the environment. Register the course's agents first.
 */
export function serveFromEnv(): void {
	const { host, port } = parseAddr(process.env.WORKER_ADDR ?? '127.0.0.1:8091');
	const capacity = Math.max(1, Number(process.env.NABU_AGENT_CAPACITY) || 2);
	const mi = modelInfo();
	log('info', 'model config', {
		host: mi.host,
		model: mi.model,
		forceFallback: mi.forceFallback,
		think: mi.think,
		keepAlive: mi.keepAlive
	});
	const ii = imageInfo();
	log('info', 'image config', {
		host: ii.host,
		steps: ii.steps,
		sampler: ii.sampler,
		forceFallback: ii.forceFallback
	});
	startServer({ host, port, token: process.env.WORKER_DISPATCH_TOKEN ?? '', capacity });
}
