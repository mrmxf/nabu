// The nabu worker contract, mirrored from the Go side (internal/agentsvc). the runtime is
// a drop-in for secret-agent-ai/worker: same wire shapes, so nabu, the loopback
// test, and the health test plan all apply unchanged.

export const VERSION = 'nabu-runtime-0.1.0';

// DispatchRequest is what nabu POSTs to /v1/jobs/dispatch. ttl_s is a relative
// deadline enforced on the worker's own clock (skew-free).
export interface DispatchRequest {
	job_id: string;
	agent: string;
	inputs: Record<string, unknown>;
	ttl_s: number;
}

// DispatchResult is the synchronous response. A non-empty error marks failure;
// nabu maps an error containing "timeout" to a timeout event.
export interface DispatchResult {
	result_html: string;
	error?: string;
}

// Health is the /healthz body. agents[] is the presence contract nabu checks
// before dispatching.
export interface Health {
	alive: boolean;
	uptime_s: number;
	version: string;
	capacity: number;
	running: number;
	queue_depth: number;
	agents: string[];
}
