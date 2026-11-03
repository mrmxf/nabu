# Agent runtime config — who decides the model knobs

> **Status:** design, agreed 2026-07-20. Implementation staged (see [Staging](#staging)).
> The wire contract does **not** change in stage 1.

Model-runtime settings (context window, reasoning, keep-alive) started life as ni6
env vars. That was right for bring-up and wrong for a classroom: a teacher whose
lesson is running slow has no way to trade answer quality for speed, and the values
differ per box. This records where each knob lives and who may move it.

## The measurement that shapes the design

Changing `num_ctx` **reloads the model**. Observed on a 4080 / `qwen3.5:9b`:
`ollama ps` reports `CONTEXT` tracking each request, and `SIZE` moves with it — a
few seconds per change.

Consequences, and they drive everything below:

- **Per-agent context is expensive.** hunter/hattie/pixie on 2048/4096/8192 makes an
  orca or buzz run reload *twice mid-pipeline*.
- **Live tuning stalls the course.** A teacher moving a slider mid-lesson pays the
  reload on whoever is next in the queue.

So: one context value for all three agents, applied at session start, not live.

## Confirmed decisions

### 1. Precedence chain

```
config default → per-agent config override → lesson frontmatter → session (teacher) → worker clamp
```

One-directional. Each layer may narrow what the one above allows; none may widen it.
The worker clamp is last and absolute.

A teacher's change is **session-scoped** and resets to the lesson default on the next
run. (Boring and predictable; revisit only if teachers ask for persistence.)

### 2. A single context selector — 2k / 4k / 8k

Config and the teacher dashboard expose **one** value covering all three agents. Not
per-agent: that's the reload cost above, and a teacher has no basis to reason about
hattie's window versus pixie's.

- **The default is the maximum** (8k). Lessons start at best quality.
- The teacher can only move **down** — 8k → 4k → 2k — trading answer quality for
  speed when things feel laggy.
- **The teacher can never exceed the config ceiling.** The selector's top entry is
  whatever config permits; there is no path to a larger value from the UI.

### 3. `keep_alive` and `capacity` are admin knobs

They stay in `admin.ni6.*` and off the teacher dashboard entirely. A teacher has no
basis to choose 90m over 30m — it's a question about GPU sharing and box uptime, not
pedagogy. Teacher sees quality; admin sees resources.

### 4. The worker clamps, and does not trust

Whatever arrives, ni6 owns the GPU and enforces the ceiling. `NABU_AGENT_CAPACITY` concurrent
runs × context × model size must fit VRAM: on a 16GB 4080 with a 5.9GB model there is
room for capacity 2 at 8k, and not much more. A bug in the dashboard asking for 32k
must be clamped, not honoured — an OOM takes the whole lesson down mid-lesson.

UI validation is not enforcement. Clamp server-side, and **log when the clamp bites**
so a silently-reduced context is visible rather than mysterious.

### 5. ni6 keeps its own local config

Because the worker runs on different hardware per deployment (a 4080 here, something
else next term), ni6 holds platform-level defaults locally rather than receiving
everything from nabu. Whether that stays env vars or becomes a config file is open —
env is what exists and works today.

### 6. The wire contract does not change

`DispatchRequest` (`job_id`, `agent`, `inputs`, `ttl_s`) stays as it is, mirrored in
[contract.ts](../runtime/src/contract.ts) and Go's `internal/agentsvc`, and implemented by
both ni6 and `secret-agent-ai/worker`. **Performance tuning is a bonus, not a
necessity** — it must not hold up the contract or the second worker.

## Open question — how a teacher's value reaches the worker

Decisions 1–2 describe a chain ending at the worker; decision 6 closes the only
obvious pipe. Three ways out, unresolved:

| | Approach | Cost |
|---|---|---|
| **A** | Reuse `inputs` (already `Record<string, unknown>`) — nabu puts context there | No contract change at all, but conflates agent inputs with runtime params |
| **B** | Add optional `model_opts?: Record<string, unknown>` | Additive and backward-compatible; provider-neutral (a Bedrock adapter maps it); but it *is* a contract edit in two languages |
| **C** | Defer — ni6 tunes itself locally, nabu sends nothing | Zero cost now; no teacher control until A or B lands |

**Recommended: C now, B when the dashboard is built.** B keeps the contract
provider-neutral, which matters because
[agent fundamentals](../help/using-nabu/reference/10-agent-fundamentals.md)
commits to surviving a port to Bedrock. Avoid A — reusing `inputs` for runtime
parameters is the kind of shortcut that is very hard to unpick later.

## Staging

**Stage 1 — now, no contract change.** ni6 defaults all three agents to the same
context (8k, the max) so multi-agent runs never reload mid-pipeline. Context, think
and keep-alive stay ni6-local env. `NABU_AGENT_CTX_MAX` clamp added.

**Stage 2 — config layer.** `admin.ni6.*` in `config/nabu-config.yaml` carries the
ceiling and defaults; nabu resolves the chain down to lesson frontmatter. Values are
still fixed per lesson — no live control.

**Stage 3 — teacher dashboard.** The 2k/4k/8k selector, session-scoped, applied at
session start. Requires the delivery decision above.

Ship stage 1 and 2 first. You may find context is set once per lesson and never
touched, in which case stage 3 is a dashboard nobody needs.

## Related

- [lesson-choice-values.md](lesson-choice-values.md) — the existing lesson-scoped value
  mechanism. **Reuse its shape for the frontmatter layer**; a second, differently-shaped
  override chain is the thing to regret.
- [config-env-sentinel.md](config-env-sentinel.md) — planned `ENV` sentinel for config values.
- [secrets.md](secrets.md) — the ni6 env table these knobs currently live in.
