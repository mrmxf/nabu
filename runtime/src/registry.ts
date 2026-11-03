import { textToHtml, offlineNote } from './util.js';
import type { Reply } from './model/ollama.js';

// AgentCtx carries per-job context into an agent: the job id (for logs) and the
// abort signal (fires on TTL). Agents must pass the signal to generate().
export interface AgentCtx {
	jobId: string;
	signal: AbortSignal;
}

// Think is an agent's core: prompt the model, return text + whether it was live.
// Keeping this separate from HTML rendering lets controllers compose agents by
// their text output while single-agent dispatch renders HTML.
export type Think = (inputs: Record<string, unknown>, ctx: AgentCtx) => Promise<Reply>;

// Agent is a dispatchable unit. `names` are the aliases nabu may send (and that
// /healthz advertises); `run` returns the result HTML.
export interface Agent {
	names: string[];
	label: string;
	run: (inputs: Record<string, unknown>, ctx: AgentCtx) => Promise<string>;
}

// toAgent wraps a Think into an Agent whose run renders the reply as HTML. This
// is the seam for "decompose later": more agents = more Thinks, same interface.
export function toAgent(names: string[], label: string, think: Think): Agent {
	return {
		names,
		label,
		async run(inputs, ctx) {
			const r = await think(inputs, ctx);
			return `<div class="agent-out"><p><strong>${label}</strong></p>${textToHtml(r.text)}${offlineNote(r.live)}</div>`;
		}
	};
}

const byName = new Map<string, Agent>();
const advertised: string[] = [];

export function register(agent: Agent): void {
	for (const n of agent.names) {
		byName.set(n, agent);
		if (!advertised.includes(n)) advertised.push(n);
	}
}

export function lookup(name: string): Agent | undefined {
	return byName.get(name);
}

export function agentNames(): string[] {
	return [...advertised];
}
