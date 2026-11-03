---
title: Secrets & environment configuration
date: 2026-07-12
status: reference
---

# Secrets & environment

How Secret Agents handles configuration and secrets: the methodology, the full list
of environment variables, how they're provisioned (Infisical), and the S3 bucket
access model.

## Methodology

- **Config lives in the environment** (12-factor). Every service reads its config
  from ENV at startup — Go via `os.Getenv`, ni6 via `process.env`. Nothing secret in
  git, nothing secret hard-coded.
- **Secrets vs config.** Most vars are non-secret (URLs, region, model, capacity). A
  small set are *secrets* (a bearer token, the S3 keys); those come from Infisical.
- **Never logged.** Secrets are read into unexported fields and never emitted. slog
  logs structured fields, never the value — e.g. `agentsvc` marks the token "never
  logged"; the worker logs `auth: enabled`, not the token.
- **Never to the browser.** nabu holds every server-side secret; the SPA only ever
  receives the signed-in user's own PocketBase token. S3 keys never leave nabu.
- **Fail safe when unset.** Missing worker/S3 config degrades gracefully (the agent
  server reports offline; resource endpoints return `503`) rather than crashing.

## Environment variables

Legend: **req** = required for that capability, **opt** = optional (default shown).
Five vars are secrets (marked 🔒); the rest are plain config.

### nabu (control plane)

| Var | Req/Opt | Default | Purpose |
|---|---|---|---|
| `WORKER_HEALTH_URL` | req (agents) | — | worker `/healthz` URL; unset ⇒ "no worker configured" |
| `WORKER_DISPATCH_URL` | opt | derived from health URL | worker dispatch endpoint (`…/healthz` → `…/v1/jobs/dispatch`) |
| `WORKER_DISPATCH_TOKEN` 🔒 | opt (req for a public worker) | — (empty ⇒ no auth) | bearer for nabu→worker; must match the worker. A public (non-tailnet, non-loopback) worker must be `https` and have a token, or nabu refuses to dispatch to it |
| `WORKER_ACCESS_CLIENT_ID` | opt | — | Cloudflare Access service-token id, sent on probe + dispatch when the worker sits behind Cloudflare Access |
| `WORKER_ACCESS_CLIENT_SECRET` 🔒 | opt | — | the matching Cloudflare Access service-token secret |
| `WORKER_PROBE_TIMEOUT` | opt | 5s | `/healthz` timeout (Go duration); allows a scale-to-zero worker to answer from a cold start |
| `WORKER_DISPATCH_TIMEOUT` | opt | 90s | one job's timeout (Go duration); keep under 100s for a worker behind Cloudflare |
| `NABU_SUPERUSER_PW` 🔒 | opt | — (empty ⇒ superuser not seeded) | dashboard `_superusers` password, read by `nabu seed` / `nabu users import` for a `role: superuser` roster entry. Seed/import-time only — not needed to `serve` |
| `AGENT_MAX_TTL` | opt | 120 | job TTL ceiling (seconds) |
| `S3_ENDPOINT` | req (resources) | — | S3 endpoint; unset ⇒ resource endpoints `503` |
| `S3_REGION` | opt | us-east-1 | any value SigV4 accepts |
| `S3_ACCESS_KEY` 🔒 | req (resources) | — | shared S3 credential key id |
| `S3_SECRET_KEY` 🔒 | req (resources) | — | shared S3 credential secret |
| `S3_PUBLIC_BASE` | opt | = `S3_ENDPOINT` | base URL for public object links |
| `S3_MAX_UPLOAD_MB` | opt | 200 | URL-import size cap |

### Worker (the Go test double, and ni6)

| Var | Req/Opt | Default | Purpose |
|---|---|---|---|
| `WORKER_ADDR` | opt | 127.0.0.1:8091 | listen address (`0.0.0.0:8091` for the tailnet) |
| `WORKER_DISPATCH_TOKEN` 🔒 | opt | — (empty ⇒ open + warn) | bearer the worker requires; must match nabu |

### ni6 (additional)

| Var | Req/Opt | Default | Purpose |
|---|---|---|---|
| `NABU_AGENT_CAPACITY` | opt | 2 | max concurrent agent runs (GPU budget) |
| `OLLAMA_HOST` | opt | http://127.0.0.1:11434 | Ollama base URL |
| `NABU_AGENT_MODEL` | opt | qwen3.5:9b | model to use |
| `NABU_AGENT_THINK` | opt | off | `1` enables reasoning tokens (slower; needs more context) |
| `NABU_AGENT_KEEP_ALIVE` | opt | 90m | model idle-unload timer; `-1` never unloads |
| `NABU_AGENT_NUM_CTX` | opt | 4096 | fallback context window |
| `NABU_AGENT_CTX_HUNTER` | opt | 2048 | hunter's context window |
| `NABU_AGENT_CTX_HATTIE` | opt | 4096 | hattie's context window |
| `NABU_AGENT_CTX_PIXIE` | opt | 8192 | pixie's context window |
| `NABU_AGENT_FAKE_MODEL` | opt | — | `1` forces the offline fallback (no Ollama) |

The runtime secrets are `WORKER_DISPATCH_TOKEN`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`;
`NABU_SUPERUSER_PW` is a seed/import-time secret (setting the dashboard superuser),
not needed while serving. Everything else is non-secret config.

## Infisical

Secrets are managed in **Infisical** (an open-source secrets manager) and injected
as ENV — nabu is never a secret authority itself, only a consumer.

Model:

- A **project** holds secrets, organised by **environment** (dev / staging / prod).
- Secrets reach a process either via the **CLI** (`infisical run -- ./nabu serve`
  populates ENV for the child) or the **Infisical Agent** (writes short-lived
  secrets to a file/env and refreshes them).
- Each host authenticates with a **machine identity** — the single bootstrap
  credential a host holds out-of-band (cloud-init / TPM). Everything else flows from
  Infisical; you can't Infisical your way out of that one root secret.
- Nothing is committed; rotation = change it in Infisical and redeploy (or let the
  agent refresh).

**Free-account fit (2026):** nabu's entire footprint is a **handful of secrets**
(the three runtime ones — `WORKER_DISPATCH_TOKEN`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`
— plus `NABU_SUPERUSER_PW` at seed time) across a couple of environments and one
small team — comfortably inside Infisical's free tier. No paid
plan is required to run Secret Agents; the free account covers the handful of
projects, secrets, and members a deployment like this needs.

## S3 bucket

### General

The resource store uses **nabu-mediated presigned access** — a provider-agnostic
model that works on any S3-compatible store:

- nabu holds **one shared credential** and **mediates every access**.
- **Uploads** — nabu mints a short-lived **presigned PUT** URL scoped to a
  server-derived key; the browser uploads directly to the bucket. The client never
  sees a key and never chooses the path.
- **URL import** — nabu fetches a URL server-side and stores it under the lesson's
  `resources/` prefix.
- **Reads** — objects are **public**, so listing returns the public URL directly (no
  presigned GET).
- **Bucket & prefix** come from the **lesson record** (`params.bucket` /
  `params.prefix`), never a client string. nabu composes the key subpath:
  - teacher → `{prefix}/resources/{safe-filename}`
  - student → `{prefix}/{session}/{group}/resource/{safe-filename}` *(student UI is a
    follow-up; the backend already permits it)*
- Teachers/students have **no S3 identities** — nabu enforces per-role access in code
  (teacher writes anywhere in the bucket; a student only under their own group
  prefix). Only uploads are presigned (10-min TTL); filenames are sanitized
  server-side (`s3svc.SafeFilename`).

### Peasoup

Peasoup is the S3-compatible object store used in production. Specifics:

- **Path-style addressing** with a custom endpoint (`S3_ENDPOINT`). Public object
  URLs are `{S3_PUBLIC_BASE}/{bucket}/{key}`.
- **Dedicated bucket per lesson** (named in `params.bucket`); the one shared
  credential spans the lesson buckets.
- Each bucket needs a **public-read policy** (so 3rd-party video tools fetch by URL)
  and **CORS** (so the browser can PUT to a presigned URL).

Example — provisioning a lesson bucket on peasoup.

ENV on nabu (secrets from Infisical):

```bash
S3_ENDPOINT=https://s3.peasoup.example
S3_REGION=us-east-1
S3_ACCESS_KEY=…            # 🔒 from Infisical
S3_SECRET_KEY=…            # 🔒 from Infisical
S3_PUBLIC_BASE=https://s3.peasoup.example
```

Public-read bucket policy (scope `Resource` to `.../resources/*` if you don't want
the whole bucket world-readable):

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Sid": "PublicReadResources",
    "Effect": "Allow",
    "Principal": "*",
    "Action": "s3:GetObject",
    "Resource": "arn:aws:s3:::LESSON-BUCKET/*"
  }]
}
```

CORS (let the SPA origin PUT to presigned URLs):

```json
[{
  "AllowedOrigins": ["https://YOUR-NABU-ORIGIN"],
  "AllowedMethods": ["PUT", "GET"],
  "AllowedHeaders": ["*"],
  "ExposeHeaders": ["ETag"],
  "MaxAgeSeconds": 3000
}]
```

The single nabu credential's policy (full object CRUD + list on the lesson buckets):

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Sid": "NabuLessonResources",
    "Effect": "Allow",
    "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject", "s3:ListBucket"],
    "Resource": ["arn:aws:s3:::LESSON-BUCKET", "arn:aws:s3:::LESSON-BUCKET/*"]
  }]
}
```

*If peasoup later supports per-identity credentials / STS*, the same model maps to
literal per-role policies: teacher = write `bucket/*`; student = write only
`bucket/{prefix}/{session}/{group}/resource/*`.
