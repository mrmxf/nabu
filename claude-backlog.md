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
| M-05 | `docs/user/deploy.md`, `help/.../02-first-deploy.md`, `03-laptop-deploy.md` | Describe the scp + Caddy + blob flow. Rewrite around containers. |
| M-06 | `internal/content`, `internal/seed` | `content.Load` reads paths, so embedded help, content and assets (`nabu.Options`) are extracted to temp dirs at seed (`seed.extractFS`). Move the loader to `fs.FS` to drop the extraction. |
| M-07 | collections `sessions`, `schedule_sessions` | Glossary rename pending: `sessions` -> `attempts`, `schedule_sessions` -> `sessions`, `/session/[id]` -> `/attempt/[id]`. |
| M-08 | `nabu.Options.Public` | The SPA is a build output, so `go get` cannot deliver it. A library consumer copies `pb_public/` or passes `Public`. Decide: ship the built SPA as a release asset, or as an embeddable Go package generated in CI. |
| M-09 | `nabu.New` | Sets package-level state (`embedfs.OverrideYAML`, `demo.Override`, embedfs flag vars), so one App per process. Thread it through instead if a consumer ever needs two. |
| C-01 | `internal/httpapi/agentcfg_test.go`, `internal/students/backup.go` and others | Pre-existing gofmt drift. Format only files you otherwise touch. |
| C-02 | `/api/agent/health` | Requires sign-in, so a signed-out page cannot show worker status. Intentional or not: undecided. |
| C-03 | `runtime/package.json` | `private: true` until the npm package name and registry are decided. |
