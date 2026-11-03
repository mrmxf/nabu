import { log } from '../log.js';

// Model client over Ollama's HTTP API, with a deterministic fallback so the runtime runs
// and is testable without a GPU. Real inference lights up when OLLAMA_HOST points
// at a running Ollama with NABU_AGENT_MODEL pulled. LangGraph controllers and the agents
// both call generate().

const OLLAMA_HOST = process.env.OLLAMA_HOST ?? 'http://127.0.0.1:11434';
const MODEL = process.env.NABU_AGENT_MODEL ?? 'qwen3.5:9b';
const FORCE_FALLBACK = process.env.NABU_AGENT_FAKE_MODEL === '1';

// Qwen3-family models emit reasoning tokens before their answer, which dominates
// latency on short outputs like a bullet list. Measured on a 4080 with
// qwen3.5:9b, a short search-style agent took 4.1s with thinking off and 28s with it on — and at the
// default 4096 context it spent the WHOLE window reasoning and returned nothing.
// So reasoning is off unless asked for: set NABU_AGENT_THINK=1 to turn it back on, and
// raise that agent's context if you do. Models without thinking ignore the field.
const THINK = process.env.NABU_AGENT_THINK === '1';

// Fallback context window for callers that don't specify one. num_ctx must cover
// prompt + reply; too small silently truncates the answer to nothing.
const DEFAULT_NUM_CTX = Number(process.env.NABU_AGENT_NUM_CTX) || 8192;

// Hard ceiling. The worker owns the GPU, so it enforces the limit no matter what a
// caller asks for: NABU_AGENT_CAPACITY concurrent runs x this context x the model weights
// all share one card, and an over-large request OOMs the GPU mid-lesson. Clamping
// is logged, because a silently shrunk context looks like a model quality problem.
const NUM_CTX_MAX = Number(process.env.NABU_AGENT_CTX_MAX) || 8192;

function clampCtx(want: number): number {
	if (want <= NUM_CTX_MAX) return want;
	log('warn', 'num_ctx clamped', { requested: want, max: NUM_CTX_MAX });
	return NUM_CTX_MAX;
}

// How long Ollama keeps the model resident AFTER THE LAST REQUEST — it's an idle
// timer, reset by every call, not a total lifetime. Ollama's own default is 5m,
// which evicts the model during a quiet stretch (a discussion slide, a break) and
// makes the next student pay a ~30s cold load. Sent per-request so the runtime holds the
// model regardless of how the Ollama service was configured. "-1" never unloads.
const KEEP_ALIVE = process.env.NABU_AGENT_KEEP_ALIVE ?? '90m';

export interface Reply {
	text: string;
	live: boolean; // false => produced by the offline fallback
}

// GenOpts lets each agent size its own request. Agents fed upstream material
// (another agent's output) need more room than one working from a bare topic.
export interface GenOpts {
	numCtx?: number;
}

// generate sends a system+user prompt to Ollama and returns the reply. On any
// connection error (Ollama down / model missing) it returns a deterministic
// fallback so the pipeline still completes — UNLESS the abort signal fired, in
// which case it's a TTL timeout and the error propagates.
export async function generate(
	system: string,
	user: string,
	signal: AbortSignal,
	opts: GenOpts = {}
): Promise<Reply> {
	if (FORCE_FALLBACK) return { text: fallback(user), live: false };
	try {
		const res = await fetch(`${OLLAMA_HOST}/api/chat`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				model: MODEL,
				stream: false,
				think: THINK,
				options: { num_ctx: clampCtx(opts.numCtx ?? DEFAULT_NUM_CTX) },
				keep_alive: KEEP_ALIVE,
				messages: [
					{ role: 'system', content: system },
					{ role: 'user', content: user }
				]
			}),
			signal
		});
		if (!res.ok) throw new Error(`ollama ${res.status}`);
		const data = (await res.json()) as { message?: { content?: string } };
		const text = data.message?.content?.trim();
		if (!text) throw new Error('empty ollama response');
		return { text, live: true };
	} catch (err) {
		if (signal.aborted) throw err; // TTL abort — let dispatch report a timeout
		// NEVER fail quietly here. The fallback returns plausible-looking canned
		// text, so without this line a dead Ollama, a missing model, or a context
		// too small to hold the answer all look like the agent simply worked.
		const reason = err instanceof Error ? err.message : String(err);
		log('warn', 'model call failed — serving offline fallback', {
			model: MODEL,
			host: OLLAMA_HOST,
			reason,
			hint: reason.includes('empty')
				? 'model produced no content: raise the context window, or set NABU_AGENT_THINK=0 so reasoning cannot consume it'
				: 'check Ollama is running and NABU_AGENT_MODEL is pulled (ollama list)'
		});
		return { text: fallback(user), live: false };
	}
}

// fallback is a canned, deterministic stand-in. It echoes the ask so the pipeline
// shape (and the controllers composing agents) is observable without a model.
function fallback(user: string): string {
	const topic = user.replace(/^[^:]*:\s*/, '').slice(0, 120);
	return [
		`- Key point about ${topic} (illustrative)`,
		`- Supporting detail for ${topic} (illustrative)`,
		`- A caveat to check on ${topic} (illustrative)`
	].join('\n');
}

export function modelInfo(): {
	host: string;
	model: string;
	forceFallback: boolean;
	think: boolean;
	keepAlive: string;
} {
	return { host: OLLAMA_HOST, model: MODEL, forceFallback: FORCE_FALLBACK, think: THINK, keepAlive: KEEP_ALIVE };
}
