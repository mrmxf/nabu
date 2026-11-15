# Plan: build, release and deploy tooling (clog + util)

**Modes:** both. Each item says which deployment mode it serves: canonical SaaS on
Cloudflare, or the interactive classroom on a laptop (see [claude-deploy.md][deploy]).

**Status:** moved here on 2026-11-14 from the original split plan (section 4b, decided
2026-11-03). U1–U3 are done, U9, `nabu web fetch`, the `nabu init` clog templates, `nabu classroom` and `nabu doctor` are done, and U4–U8 and `nabu media check` are open. How nabu
builds and releases today is in [claude-build.md][build].

## Decision

nabu builds, releases and deploys with [clog][clog] and [util][util] rather than a
second pipeline. clog is a thin CLI over util: new behaviour goes upstream into util,
tagged per module (`<module>/vX.Y.Z`), then clog is released and nabu bumps
`.clog-version`.

**Rules for working upstream:**

- No build logic in clog's Go; new behaviour goes in util.
- Check builds with `GOWORK=off`, as CI does.
- Ask before every tag, push or release in util or clog.
- util stays course-agnostic. nabu never shells into clog at run time; it only ships
  `.clog.yaml` and workflows as templates.
- No duplicated konfig (`util/ci/DUPLICATION.md`): nabu layers its `.clog.yaml` over
  util's embedded konfig and never copies it.

## util/clog additions

Generic DevOps, upstream in util, each with its own module tag, in priority order.

| # | Addition | Why a course developer cares | Mode | Shown in | Status |
|---|---|---|---|---|---|
| U1 | **`node` stack + `bc-npm` worker** (`npm ci && npm run build`, Node pinned by `.nvmrc`) | the web app, and any course with a JS widget, builds in the same flow as Go | both | nabu release | **done** (util `ci/v0.27.0`, `embedfs/v0.20.0`) |
| U2 | **`bc-golang` options:** build tags (e.g. `webembed`), a pre-make step (the web build first), Windows targets (amd64/arm64), `.tar.gz`/`.zip` archives | classroom laptops are often Windows or macOS: one download per OS | classroom | nabu release | **done** (clog v1.3.0) |
| U3 | **Install recipes:** `wrangler` (pinned, not bare `npx`), `gitleaks`, `trufflehog`, `tailscale`, `nabu` (from GitHub releases, sha256-checked against `checksums.txt`) | `clog Install nabu` is the one-line laptop setup the tutorial teaches | both | nabu-tutorial "get started" | **done** (util `install/v0.19.0`; Node fix in `install/v0.20.0`, backlog M-16) |
| U4 | **`gitlab-release` and `gitlab-pages` deployers** (already declared as target kinds, with no deployer) via `glab` | private courses on GitLab get the same release and Pages path as public ones | both | a private GitLab course | open |
| U5 | **`bucket` deployer** (S3 API: R2, Scaleway Object Storage, any S3) with sync, delete-extraneous and dry-run | pushing `media/` to the media store is one command, not a hand-run `aws s3 cp` | SaaS | nabu-tutorial (R2) | open |
| U6 | **`cloudflare-worker` deployer** (`wrangler deploy` for a Worker + Container Durable Object + routes), finishing clog's `doc/plan-cloudflare-helpers.md` (`clog CF ensure …`) | the SaaS workload tier and the academy Worker deploy the way Pages does | SaaS | nabu-tutorial SaaS demo; academy | open |
| U7 | **`scaleway-container` deployer** (Serverless Containers via `scw`: min-scale 0, env, secrets, health probe) | a European scale-to-zero alternative to Cloudflare Containers, for a course that must stay in the EU | SaaS (self-hosted variant) | a private GitLab course | open |
| U8 | **`classroom-kit` artifact:** a zip per OS with the binary, the course (or a library-built course binary), a `start-classroom` script, README and checksums, attached to the release | a teacher downloads one file, double-clicks, and teaches with no terminal | classroom | nabu-tutorial releases | open |
| U9 | **Fixes from clog's own backlog that bite course developers** (below) | fewer opaque CI failures; supply-chain hygiene | both | everywhere | **done**: released in clog v1.3.5 (util slogger v0.12.0, kfg v0.13.0, ci v0.29.0, embedfs v0.21.0) |

**U9 in detail** (clog `claude-backlog.md` numbers). All fixed and released in clog
v1.3.5 on 2026-11-14:

- ~~M-01~~: `kodata` is out of the CI artifact uploads (GitHub and GitLab templates),
  after checking that no consumer still publishes there.
- ~~M-03~~: a failed Infisical OIDC login now names the token's `sub`, `aud` and `ref`
  (never the token itself). The new `clog CI oidc` prints a CI job's claims and the
  Subjects to bind; on a laptop it reads each identity's bound Subject and checks it.
  This is the bug that bit nabu on 2026-11-14, when a bound subject started with a space.
- ~~M-04~~: the debug log is per user, `/tmp/clog-<user>-YYYY-MM-DD.log`.
- ~~sha256 checks for `clog Install` downloads~~: done in util `install/v0.19.0`.
- ~~M-05~~: the github-release deployer's `git ls-remote` tag check retries a network
  blip; a missing tag still fails at once.
- ~~M-06~~: `clog Source project config` is a no-op in a repo with no `project.config`,
  so the `bc-*` workers' `eval` no longer hits a bash syntax error and keeps its colours.
- ~~M-07~~: **an invalid `.clog.yaml` fails loudly**, naming the file and the YAML error,
  instead of clog quietly falling back to the built-in config.
- M-08 (found on the way, open): user-level config (`$HOME/.clog.yaml`) is never loaded.

## nabu-side additions

Course-aware, so they belong in nabu, not util.

- ~~**`nabu classroom`**~~ (**done 2026-11-15**, `internal/classroom`; `--export` writes the
  roster on exit, `nabu init` adds an always-open practice Session) (classroom): binds `0.0.0.0`, turns on classroom fast login for
  the session, and prints the LAN URLs plus a QR code for students. It warns if any
  agent job lacks `sim_output_fallback` while no worker is reachable, and offers
  `users export` on exit. It is the target of U8's `start-classroom` script.
- ~~**`nabu doctor [--mode saas|classroom|selfhost]`**~~ (**done 2026-11-15**,
  `internal/admin/doctor.go`; selfhost takes `--image` until an image is published) (both): `admin validate` plus
  readiness checks per mode, silent when ready:
  - **SaaS:** static build present, Cloudflare token, media bucket reachable, worker on
    https with a token.
  - **Classroom:** web app embedded, LAN interface found, offline fallbacks present.
  - **Self-hosted:** container image present, data volume writable.
- ~~**`nabu web fetch`**~~ (both; backlog M-08): **done 2026-11-14**. **`nabu media check`** (SaaS): every
  `media:` reference resolves in the store (a local mount, or the `resource_objects`
  index).
- **`nabu init` writes clog files** (**done 2026-11-14**, except the reusable course
  workflows: the pipeline calls clog's reusable workflows directly): `.clog.yaml` with the course's stacks and targets,
  `.clog-version`, and `.github/workflows/course.yaml` (public) or `.gitlab-ci.yml`
  (private). Each is a ~20-line call into reusable course workflows shipped by nabu:
  `build` = `nabu build` + validate + scans; `deploy` = Pages and/or container and/or
  classroom kit, per mode. Backlog C-04.

## Order

1. ~~U1, U2, U3, then nabu's own `.clog.yaml` release~~ (done: `v0.1.0`, `v0.1.2`).
2. ~~`nabu web fetch`~~ (done 2026-11-14).
3. ~~The `nabu init` clog templates~~ (done 2026-11-14); reusable course workflows still open.
4. U8, then ~~`nabu classroom` + `nabu doctor`~~ (done 2026-11-15).
5. U4 and U5, then U6 and U7 alongside the SaaS build steps in [plan-saas.md][saas].
6. ~~U9~~ (clog v1.3.5, 2026-11-14).

Each util change: tests in util, a module tag, a clog release, then nabu bumps
`.clog-version`.

## Verification

- A tag in nabu runs `clog build prod` in CI and produces Linux, macOS and Windows
  binaries (amd64 and arm64) with the web app embedded, `checksums.txt`,
  `nabu-web-vX.Y.Z.tar.gz` and a ghcr image.
- `clog CI probe` passes.
- `clog Install nabu` on a clean laptop fetches and checks the release.
- A course scaffolded with `nabu init` deploys to Cloudflare Pages from CI with no
  hand-written workflow.
- `nabu classroom` on a laptop with the Wi-Fi uplink off serves a second device.
- A private GitLab course's pipeline publishes a GitLab release and a Scaleway container.

[build]: ../claude-build.md
[clog]: https://github.com/mrmxf/clog
[deploy]: ../claude-deploy.md
[saas]: plan-saas.md
[util]: https://github.com/mrmxf/util
