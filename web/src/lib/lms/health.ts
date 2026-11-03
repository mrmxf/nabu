import { pb } from '$lib/pocketbase';

// Client-side view of the agent server's health, served by nabu's cached probe
// at GET /api/agent/health (see internal/agentsvc). Mirrors agentsvc.Health.
export interface AgentHealth {
	alive: boolean;
	uptime_s: number;
	version: string;
	capacity: number;
	running: number;
	queue_depth: number;
	agents: string[];
	reason?: string;
	checked_at: string;
}

// A short client cache on top of nabu's own cache: a slide with several agent
// widgets shouldn't fire one request each. TTL is small so the offline/online
// flip feels live.
const TTL_MS = 3000;
let cache: { at: number; value: AgentHealth } | null = null;
let inflight: Promise<AgentHealth> | null = null;

function offline(reason: string): AgentHealth {
	return {
		alive: false,
		uptime_s: 0,
		version: '',
		capacity: 0,
		running: 0,
		queue_depth: 0,
		agents: [],
		reason,
		checked_at: new Date().toISOString()
	};
}

// getAgentHealth returns the worker health, coalescing concurrent callers and
// caching briefly. It never rejects: an unreachable server is a value
// (alive:false), not an error, so widgets can render an offline state uniformly.
export async function getAgentHealth(force = false): Promise<AgentHealth> {
	if (!force && cache && Date.now() - cache.at < TTL_MS) return cache.value;
	if (inflight) return inflight;
	inflight = (async () => {
		try {
			const h = await pb.send<AgentHealth>('/api/agent/health', { method: 'GET' });
			cache = { at: Date.now(), value: h };
			return h;
		} catch {
			const h = offline('could not reach nabu');
			cache = { at: Date.now(), value: h };
			return h;
		} finally {
			inflight = null;
		}
	})();
	return inflight;
}
