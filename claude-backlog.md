# claude-backlog.md — known issues and deferred work

Not read every query. Read it before touching a listed file, or when picking up debt work.

Severity: **B** blocking / incorrect output · **M** moderate debt · **C** cosmetic

## Found 2026-11-03 (repo split from secret-agents)

| # | Where | Issue |
|---|---|---|
| M-01 | `help/`, `docs/` | About 230 lines still use `rise-summer` / `secret-agents` examples and hunter/hattie/pixie walk-throughs. Rewrite when the authoring lessons move to nabu-tutorial; code comments are already neutral. |
| M-02 | `help/using-nabu/` | Moving to nabu-tutorial: `authoring-lessons/*`, `for-teachers/30-control-files`, `reference/uploading-content`, `reference/00-render-pipeline`. Engine help stays. |
| M-03 | `help/.../11-agent-deploy.md`, `troubleshooting-agents.md`, `docs/image-server-setup.md` | Reference Tailscale ACL/tags/hosts templates and systemd/start scripts marked "not yet in this repo". They belong in `deploy/`. |
| M-04 | `internal/deploy`, `internal/assets` | The snapshot/install blob is to be retired in favour of containers. |
| M-05 | `docs/user/deploy.md`, `help/.../02-first-deploy.md`, `03-laptop-deploy.md` | Describe the scp + Caddy + blob flow. Rewrite per M-10: 02 = SaaS (Cloudflare, plus a self-hosted container variant), 03 = classroom laptop (release binary). |
| ~~M-06~~ | `internal/content`, `internal/seed` | FIXED 2026-11-03 — the loader reads `source.Tree` (fs.FS + label); embedded trees are no longer extracted to temp dirs, and logs name `embed://…` or absolute disk paths. |
| M-07 | collections `sessions`, `schedule_sessions` | Glossary rename pending: `sessions` -> `attempts`, `schedule_sessions` -> `sessions`, `/session/[id]` -> `/attempt/[id]`. |
| M-08 | release CI, `nabu web fetch` | DECIDED 2026-11-03: B + F (see claude-deploy.md). To build: a `webembed` build tag over a CI-generated folder; cross-platform release binaries; `nabu-web-vX.Y.Z.tar.gz` + `.sha256` on each release; a `nabu web fetch` command; a release fails if any artefact is missing. |
| M-09 | `nabu.New` | Sets package-level state (`embedfs.OverrideYAML`, `demo.Override`, embedfs flag vars), so one App per process. Thread it through instead if a consumer ever needs two. |
| M-10 | `help/using-nabu/reference/` | REQUIRED: a new `05-deployment-modes.md`, "SaaS on Cloudflare or a classroom laptop?". It covers the mode table for beginners, when to choose each, offline behaviour, where student data lives, how classroom results reach the SaaS later, and a mermaid diagram per mode. Every deploy page links to it. |
| M-11 | `web/static/who-is-nabu.png` | 6 MB of the 6.7 MB web app bundle (the app code is 764 KB). Convert to WebP at the displayed size before the first release. |
| M-12 | group notes (`pbProvider.noteText`) | Keyed by the PocketBase slide RECORD id. Ids are stable across re-seeds (upsert by slug) but not across databases, e.g. a fresh SaaS container that re-seeds. Re-key by `<course>/<lesson>/<slug>` (the static id) with a migration, so the run page can also read static content. |
| M-13 | release tags | No `v*` tag exists yet. Library courses get pseudo-versions (`go get …@latest`), and `nabu web fetch` (M-08) has nothing to fetch until the first release. |
| M-14 | static tier | A scheduled go-live cannot be enforced by a static site; `nabu build` leaves future lessons out. SaaS CI must rebuild at go-live (e.g. a scheduled build). |
| C-04 | `nabu init` | Writes no CI or Cloudflare templates yet: they arrive with steps 5–6. |
| C-01 | `internal/httpapi/agentcfg_test.go`, `internal/students/backup.go` and others | Pre-existing gofmt drift. Format only files you otherwise touch. |
| C-02 | `/api/agent/health` | Requires sign-in, so a signed-out page cannot show worker status. Intentional or not: undecided. |
| C-03 | `runtime/package.json` | `private: true` until the npm package name and registry are decided. |
