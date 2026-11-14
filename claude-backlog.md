# claude-backlog.md — known issues and deferred work

Not read every query. Read it before touching a listed file, or when picking up debt work.

Severity: **B** blocking / incorrect output · **M** moderate debt · **C** cosmetic

## Found 2026-11-03 (repo split from secret-agents); every item re-checked 2026-11-14

| # | Where | Issue |
|---|---|---|
| M-01 | `help/`, `docs/` | Checked 2026-11-14: 90 lines (was ~230) still use `rise-summer` / `secret-agents` examples and hunter/hattie/pixie walk-throughs; most in `for-teachers/20-rosters.md` (26), `docs/user/deploy.md` (18), `11-agent-deploy.md`, `docs/student-management.md`, `docs/image-server-setup.md`. Rewrite when the authoring lessons move to nabu-tutorial; code comments are already neutral. |
| M-02 | `help/using-nabu/` | Moving to nabu-tutorial: `authoring-lessons/*`, `for-teachers/30-control-files`, `reference/uploading-content`, `reference/00-render-pipeline`. Engine help stays. |
| M-03 | `help/.../11-agent-deploy.md`, `troubleshooting-agents.md`, `docs/image-server-setup.md` | Reference Tailscale ACL/tags/hosts templates and systemd/start scripts marked "not yet in this repo". They belong in `deploy/`. |
| M-04 | `internal/deploy`, `internal/assets` | The snapshot/install blob is to be retired in favour of containers. |
| M-05 | `docs/user/deploy.md`, `help/.../02-first-deploy.md`, `03-laptop-deploy.md` | Describe the scp + Caddy + blob flow. Rewrite per M-10: 02 = SaaS (Cloudflare, plus a self-hosted container variant), 03 = classroom laptop (release binary). |
| ~~M-06~~ | `internal/content`, `internal/seed` | FIXED 2026-11-03 — the loader reads `source.Tree` (fs.FS + label); embedded trees are no longer extracted to temp dirs, and logs name `embed://…` or absolute disk paths. |
| M-07 | collections `sessions`, `schedule_sessions` | Glossary rename pending: `sessions` -> `attempts`, `schedule_sessions` -> `sessions`, `/session/[id]` -> `/attempt/[id]`. |
| M-08 | `nabu web fetch` | MOSTLY DONE 2026-11-03: `webembed` (webapp/), cross-platform release binaries incl. Windows, and `nabu-web-vX.Y.Z.tar.gz` in `checksums.txt` via clog (`.clog.yaml`); the github-release deployer refuses a missing asset. Remaining: the `nabu web fetch` command for library courses. |
| M-09 | `nabu.New` | Sets package-level state (`embedfs.OverrideYAML`, `demo.Override`, embedfs flag vars), so one App per process. Thread it through instead if a consumer ever needs two. |
| M-10 | `help/using-nabu/reference/` | REQUIRED: a new `05-deployment-modes.md`, "SaaS on Cloudflare or a classroom laptop?". It covers the mode table for beginners, when to choose each, offline behaviour, where student data lives, how classroom results reach the SaaS later, and a mermaid diagram per mode. Every deploy page links to it. |
| ~~M-11~~ | `web/static/who-is-nabu.png` | FIXED 2026-11-03 — it was unreferenced. It is now a 768px JPEG (236 KB) in the "Who is Nabu?" help lesson, so the web app bundle drops from 6.7 MB to ~0.7 MB. |
| M-12 | group notes (`pbProvider.noteText`) | Keyed by the PocketBase slide RECORD id. Ids are stable across re-seeds (upsert by slug) but not across databases, e.g. a fresh SaaS container that re-seeds. Re-key by `<course>/<lesson>/<slug>` (the static id) with a migration, so the run page can also read static content. |
| ~~M-13~~ | release tags | FIXED 2026-11-14: `v0.1.0` (2026-11-03) and `v0.1.2` are released; `v0.1.1` is a tag with no release (CI could not log in to Infisical). Library courses can `go get github.com/mrmxf/nabu@v0.1.2`. |
| M-14 | static tier | A scheduled go-live cannot be enforced by a static site; `nabu build` leaves future lessons out. SaaS CI must rebuild at go-live (e.g. a scheduled build). |
| C-04 | `nabu init` | Writes no CI or Cloudflare templates yet: they arrive with steps 5–6. |
| M-15 | `web/src/app.html` | The web app loads FontAwesome and speed-highlight from the jsdelivr CDN. In classroom mode with no internet, icons and code highlighting are missing, which breaks the "classroom works offline" rule. Bundle them with the app (npm deps) instead. |
| ~~M-16~~ | clog `node` recipe | FIXED 2026-11-03 (util install v0.20.0, clog v1.3.1): unpacking Node 26 over a runner's Node mixed two npm versions, and `npm ci` died with "Class extends value undefined". The recipe now removes the old npm and corepack first; reproduced and verified with the real 22.21.0 and 26.13.0 tarballs. nabu pins Node 26.13.0 in `.nvmrc`. |
| ~~C-05~~ | `nabu --version` | FIXED 2026-11-14: printed PocketBase's "version (untracked)". `RootCmd.Version` is now `nabu.Version()`: tag, mode, short commit, date from clog's ldflags (`v0.1.2 (prod, b54a6fb, 2026-11-14)`), plus the nabu module version in a course binary. `TestVersion` guards it. |
| M-17 | nabu-tutorial | REQUIRED: the tutorial's "Deploy your course" lessons must include the canonical secrets and Cloudflare permissions (`help/using-nabu/reference/06-cloudflare-account.md`), the wrangler install (`07-install-wrangler.md`) and the check `scripts/cf-token-check.sh`, adapted to a course repo's own Infisical folder. Keep one source: the tutorial links or copies, and any permission change is made here first. |
| C-01 | 3 Go files (`gofmt -l`), e.g. `internal/httpapi/agentcfg_test.go`, `internal/students/backup.go` | Pre-existing gofmt drift. Format only files you otherwise touch. |
| C-02 | `/api/agent/health` | Requires sign-in, so a signed-out page cannot show worker status. Intentional or not: undecided. |
| C-03 | `runtime/package.json` | `private: true` until the npm package name and registry are decided. |
