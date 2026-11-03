---
title: Agent servers — sizing the box that runs the model
weight: 26
params:
  readiness: draft
  kind: presentation
  audience: admin
  summary: |
    Which model to run on the worker, and how to configure the serving layer so a
    classroom of students hitting it for an hour doesn't fall over. Model choice
    matters less than the serving config — most classroom failures are cold loads
    and unbounded context, not a model that's too small.
---
This page covers the **worker box** — the machine that actually runs the model.
For how a job gets there, see [agent fundamentals](help:reference#agent-fundamentals);
for proving the path works end to end, see [agent deploy](help:reference#agent-deploy).

## local agents on an RTX-4080 (16GB)

The 4080 (and 4080 Super) has **16GB of VRAM**. That number, not the GPU's speed,
is what decides both which model you can run and how many students you can serve
at once — the model weights and the KV cache for every in-flight request all live
in the same 16GB.

### One model, not two

Run **Qwen3 14B at Q4_K_M** (~9GB). It's the strongest general model that still
leaves real headroom, and it's dependable at tool-calling and structured JSON —
which is what actually breaks agent lessons when it goes wrong. Gemma 3 12B is a
reasonable alternative if you want less verbose output.

Don't reach for a 24B or 32B model at Q4 on 16GB. They load (~14GB / ~19GB) but
leave nothing for the KV cache, so you get one slow student at a time. Capacity
you can't serve concurrently is worse than a smaller model that serves four.

Splitting the card between **two** general chat models is also a false economy:
each half is small enough to degrade exactly where the course needs it (tool
calls, following slide instructions, JSON that parses), and you've added routing
logic to teach and debug. The one split worth making is a general model plus a
tiny *non-chat* specialist — an embedding model like `nomic-embed-text` (~300MB)
if a lesson does retrieval. Different job, negligible VRAM.

### Serving config for a classroom

A dozen students for an hour is a stability problem, not a model problem. In
rough order of how likely each is to ruin the lesson:

| Setting | Value | Why |
|---|---|---|
| `OLLAMA_KEEP_ALIVE` | `-1` | Never unload. The default is 5 minutes — a lull during a discussion slide evicts the model and the next student eats a ~30s cold load. This is the number one cause of "it was fine in testing". |
| `OLLAMA_MAX_LOADED_MODELS` | `1` | Stops a second model from evicting the first and thrashing the card. |
| `num_ctx` (per request) | `8192` | Set it explicitly. KV cache cost scales with `num_ctx × num_parallel`, so an unpinned context is what turns a working box into an OOM 40 minutes in. |
| `OLLAMA_FLASH_ATTENTION` | `1` | Enables the cheaper attention path. |
| `OLLAMA_KV_CACHE_TYPE` | `q8_0` | Roughly halves KV cache memory for negligible quality loss. This is what buys the parallel slots. |
| `OLLAMA_NUM_PARALLEL` | `4` | What 16GB comfortably holds alongside a 9GB model. |

Two more things that aren't settings:

- **Pre-warm before the session.** Fire one throwaway generation at lesson start
  so student one isn't the person who discovers the load time.
- **Timeout and retry at the worker.** One wedged generation must not back up the
  queue behind it. Bounded timeout, one retry, then fail the job cleanly with a
  `slog.Error` — a student seeing "job failed, retrying" beats a spinner forever.

Twelve students are not twelve concurrent calls; they're reading slides most of
the time. Cap parallelism low and let the queue absorb the rest — a job that waits
8 seconds is fine, an OOM that kills the runtime is not.

### Images share the same card

pixie's "make an image" mode runs a **separate** txt2img server (an A1111-compatible
API), not Ollama — but on a 16GB card it competes with the LLM for the same VRAM. A
resident 9GB LLM leaves ~7GB, which co-resides with a light **SD 1.5** model but is
tight for full **SDXL** without offloading; when they don't fit, they evict each
other on every text↔image switch (a slowdown under WSL, not a crash). The sizing
call, install, and wiring are their own one-off runbook:
[docs/image-server-setup.md](../../../docs/image-server-setup.md).

### When to outgrow this

vLLM gives genuinely better throughput under concurrency, but it's a real step up
in operational complexity. Ollama behind a queue is the right answer at classroom
scale; revisit only when a single box is provably the bottleneck.
