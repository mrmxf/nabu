# claude-deploy.md — deploy

Detail for [CLAUDE.md](CLAUDE.md). Build is in [claude-build.md](claude-build.md).

## Today

The only working deploy is the self-extracting blob in `internal/deploy`:
`nabu snapshot` packs `pb_data` + `pb_public` into `internal/assets/embedded.tgz` and
rebuilds with `-tags deploy` as `nabu-deploy`; `nabu-deploy install` extracts it on the
host. It shells out to `scripts/buildinfo.sh`, so it needs the Go toolchain and source.
**It is being retired.** Do not extend it.

There is no `ko`, Dockerfile, compose file or Wrangler config yet. Never add `ko`.

## Target (not built yet)

Three tiers, joined by contracts defined once in `academy/contract/`:

| Tier | What | Cloudflare (first) |
|---|---|---|
| Static | catalogue, `nabu build` lesson JSON, the SPA | Pages / Workers static assets |
| Control plane | `academy/`: login/SSO, D1 main roster, catalogue, wake | Worker + D1 |
| Workload | one nabu container per course; scales to zero | Container Durable Object |

- Container: `deploy/Containerfile`, podman or docker. Entrypoint: litestream restore ->
  `nabu seed` (skipped when the content hash is unchanged) -> `litestream replicate
  -exec "nabu serve"`. State replicates to S3/R2.
- One container per COURSE, not per lesson: auth and rosters are course-wide.
- Standalone = the same `spawn-spec` with `academy: null`.
- Academy <-> course is a single-writer cryptographic sync (signed, idempotent
  documents), never a cross-system transaction. The academy writes rosters; the course
  writes progress, local-first through an outbox.
- Bulky media is not in git: `/<course>/<lesson>/media/<asset>`, from an S3 mount or a
  Worker proxy to R2/S3.

## Agent workers

Reached through `WORKER_HEALTH_URL` (+ `WORKER_DISPATCH_URL`, `WORKER_DISPATCH_TOKEN`,
optional `WORKER_ACCESS_CLIENT_ID/SECRET`, `WORKER_PROBE_TIMEOUT`,
`WORKER_DISPATCH_TIMEOUT`). A public host must be https with a token. See
[docs/secrets.md](docs/secrets.md).

## Secrets

Infisical injects them (`infisical run --env <env> -- bin/nabu serve`). Nothing secret
is ever in a committed file; config names env vars via `*_env` keys.
