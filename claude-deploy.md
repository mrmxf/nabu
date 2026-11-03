# claude-deploy.md — deploy

Detail for [CLAUDE.md](CLAUDE.md). Build is in [claude-build.md](claude-build.md).

## Today

The only working deploy is the self-extracting blob in `internal/deploy`:
`nabu snapshot` packs `pb_data` + `pb_public` into `internal/assets/embedded.tgz` and
rebuilds with `-tags deploy` as `nabu-deploy`; `nabu-deploy install` extracts it on the
host. It shells out to `scripts/buildinfo.sh`, so it needs the Go toolchain and source.
**It is being retired.** Do not extend it.

There is no `ko`, Dockerfile, compose file or Wrangler config yet. Never add `ko`.

## Two deployment modes (target; not built yet)

Every deploy doc and help page must say which mode it is about, and link to the
modes help lesson (`help/using-nabu/reference/05-deployment-modes.md`, planned).

| | Canonical SaaS on Cloudflare | Interactive classroom on a laptop |
|---|---|---|
| Who | a course offered online to many cohorts | a teacher running a live lesson in a room |
| Web app | static tier (Pages / CDN) | embedded in the `nabu` release binary |
| Login | academy Worker + D1: SSO, main roster | native PocketBase login + classroom fast login |
| State | one container per COURSE, scales to zero, Litestream -> R2 | `pb_data` on the laptop |
| Network | internet | room Wi-Fi/LAN, `serve --http 0.0.0.0:8090`; no internet needed |
| Agents | remote worker: https + token (+ Cloudflare Access) | loopback/LAN worker, or `sim_output_fallback` jobs offline |
| Spawn | academy writes the `spawn-spec` | `academy: null`, or none |
| Afterwards | progress already synced via the outbox | export results; optionally push the outbox later |

A self-hosted container (podman/docker anywhere) is the SaaS image run with
`academy: null`. It is supported, but it is not a third design.

### SaaS shape

Three tiers, joined by contracts defined once in `academy/contract/`:

| Tier | What | Cloudflare |
|---|---|---|
| Static | catalogue, `nabu build` lesson JSON, the web app | Pages / Workers static assets |
| Control plane | `academy/`: login/SSO, D1 main roster, catalogue, wake | Worker + D1 |
| Workload | one nabu container per course; scales to zero | Container Durable Object |

- Container: `deploy/Containerfile`, podman or docker. Entrypoint: litestream restore ->
  `nabu seed` (skipped when the content hash is unchanged) -> `litestream replicate
  -exec "nabu serve"`. State replicates to S3/R2.
- One container per COURSE, not per lesson: auth and rosters are course-wide.
- Academy <-> course is a single-writer cryptographic sync (signed, idempotent
  documents), never a cross-system transaction. The academy writes rosters; the course
  writes progress, local-first through an outbox.
- Bulky media is not in git: `/<course>/<lesson>/media/<asset>`, from an S3 mount or a
  Worker proxy to R2/S3.

### Classroom shape

One file: the release `nabu` binary for the teacher's OS (linux/darwin/windows,
amd64/arm64), with the web app embedded. `nabu seed`, then
`nabu serve --http 0.0.0.0:8090`; students open the laptop's LAN IP. It must work with
the Wi-Fi uplink off. When no worker is reachable, agent jobs authored with
`sim_output_fallback: true` show the slide's `sim_output`; others report the worker
offline, so classroom content should set the fallback. The transport policy already
allows http to LAN and loopback workers.

## Delivering the web app (backlog M-08, decided: B + F)

- Release CI builds the web app ONCE per `v*` tag. It is never committed.
- Release binaries and the container image embed it with a build tag (`webembed`)
  over a folder generated only in CI.
- Each GitHub release attaches `nabu-web-vX.Y.Z.tar.gz` + `.sha256`. Library courses
  run `nabu web fetch [dir]`, which picks the version from build info or `go.mod` and
  verifies the checksum, then pass it as `nabu.Options.Public`.
- In SaaS the browser loads it from the static tier (F); the container serves `/api`
  and the live parts.
- Fallback: `pb_public/` beside the data dir, which is the dev path.
- Rejected:
  - committing the build into git: history bloat, and no LFS, because the Go proxy
    does not resolve LFS
  - building from source in every course: Node, and LFS pointers in the module cache
  - downloading at server start: runtime network, supply chain, cold start

## Agent workers

Reached through `WORKER_HEALTH_URL` (+ `WORKER_DISPATCH_URL`, `WORKER_DISPATCH_TOKEN`,
optional `WORKER_ACCESS_CLIENT_ID/SECRET`, `WORKER_PROBE_TIMEOUT`,
`WORKER_DISPATCH_TIMEOUT`). A public host must be https with a token. See
[docs/secrets.md](docs/secrets.md).

## Secrets

Infisical injects them (`infisical run --env <env> -- bin/nabu serve`). Nothing secret
is ever in a committed file; config names env vars via `*_env` keys.
