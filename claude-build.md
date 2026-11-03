# claude-build.md — build, test and config

Detail for [CLAUDE.md](CLAUDE.md). Deploy is in [claude-deploy.md](claude-deploy.md).

## Commands

```bash
./scripts/build.sh                     # web -> webapp/dist, then bin/nabu with -tags webembed
clog build                             # the CI gate: guards, lint, test, trivy scan, then make
clog build --fast                      # make phases only (web, then Go for 6 platforms)
go vet ./... && go test ./...          # verify: must pass
(cd web && npm run check)              # svelte-kit sync + svelte-check; 0 errors
(cd runtime && npx tsc --noEmit)       # agent runtime typecheck
bash scripts/check-widget-docs.sh      # widget catalogue == 40-widgets.md; silent on success
./scripts/dev.sh                       # stop, rebuild, reseed, serve :8090 (Infisical if installed)
```

```bash
bin/nabu init <dir> [--lesson slug] [--library --module path]   # new course repo
bin/nabu seed [--content d] [--assets d] [--schedules f] [--help-content d] [--strict]
bin/nabu build [--out dist] [--public d]   # static site: web app + live lessons as JSON
bin/nabu serve [--http=127.0.0.1:8090] [--dir path/to/pb_data] [--media-dir store]
bin/nabu stop
bin/nabu -C ../my-course seed          # use ../my-course/nabu.yaml
bin/nabu admin validate [-q] [--deep]  # readiness gate; exits 1 on any FAIL
bin/nabu users import --from roster.yaml [--mode merge|overwrite|delete-orphans] [--course slug]
bin/nabu migrate up | down 1           # down asks for confirmation; pipe `echo y |`
```

`svelte-check` on its own reports stale route types after a route folder is renamed.
Run `npm run check`, which runs `svelte-kit sync` first.

## Release (clog)

- `.clog.yaml` + `.clog-version` (clog v1.3.1) + `.nvmrc` (Node 26.13.0): stacks `web` (node), `nabu` and `worker`
  (golang), a `github-release` target, and a `guards` check group.
- A `vX.Y.Z` tag runs `.github/workflows/build-release.yaml` (clog's reusable workflows,
  pinned `@v1.3.0`): `clog build prod`, then a release with `nabu-<cpu>-<os>` (incl.
  Windows), `nabu-worker-*`, `nabu-web-vX.Y.Z.tar.gz` and `checksums.txt`.
- `guards`: identities are `@nabu.example` only (not LICENSE), no ko, widget docs match.
  A guard's `catch:` must `exit 1`: clog fails a check only on a non-zero catch.
- Lint (golangci-lint + staticcheck) must stay at 0 issues; CI runs it.
- Locally, put a verified clog v1.3.0 on PATH and run with `GOWORK=off` as CI does.

## Facts

- Go 1.26+, Node 26.13.0 (pinned in `.nvmrc`), git-lfs. Module path `github.com/mrmxf/nabu`.
- PocketBase v0.39.5 (Go), pocketbase JS SDK ^0.26, Svelte ^5.1, SvelteKit ^2.8, Vite ^6,
  PicnicCSS ^7.1, Yjs ^13.6.
- `mrmxf/util` packages used: `kfg` (config), `slogger` (logging), `buildinfo` (version).
- Version comes from git tags via `scripts/buildinfo.sh`, injected with `-ldflags`. A
  build on an exact tag is `prod`; anything else is `dev`.
- `pb_data` defaults to `./pb_data` in the launch directory; `pb_public` is ALWAYS its
  sibling. `--dir` moves both.
- The web build writes to `webapp/dist` (`web/svelte.config.js`), embedded by
  `-tags webembed` (`webapp/`). The server stacks that app over `pb_public/` on disk
  (`layeredFS`): app files from the binary, seeded `/content/…` and `/data/…` from disk.

## Config resolution

1. `embedfs/nabu-config.yaml`, embedded in the binary.
2. Deep-merged with the first of `-c <file>`, `-C <dir>/nabu.yaml`, `./nabu.yaml`
   (`embedfs.EffectiveOverride`).
3. `course.*` paths resolve relative to that file (`embedfs.ResolvePath`).
4. Command-line flags win for that run.

Settings that the SPA needs are seeded into the `app_config` collection by `seed`
(`internal/seed/content.go`, `settingsWithConfig`): `app_name` from
`pocketbase.app.name`, `site_logo` from `logopath`, `demo_mode`, `images_enabled`, etc.

## Seeding

- Idempotent. Runs pending migrations first, so a brand-new `pb_data` works in one step.
- Refuses while a server is running (pid file in `pb_data`); `--force` overrides.
- Every tree is a `source.Tree`: a disk dir (labelled with its absolute path) or a tree
  baked into the binary (labelled `embed://content`, `embed://assets`, `embed://help`).
  Help is `embed://help` unless `--help-content`. Seed logs one `seed sources` line, and
  each `seeded lesson` line carries `source=`.
- Demo accounts come from `course.demo_roster`, else `internal/demo/demo_mode.yaml`
  (neutral, enrolled in no course). Only with `demo_mode: true`.
- A superuser roster entry needs its `password_env` set, or it is skipped.

## Static build (`nabu build`)

- Runs BEFORE PocketBase bootstraps (like `init`), so it never creates `pb_data`.
- Writes `/content/index.json` and `/content/<course>/<lesson>/lesson.json`, under
  `/content/` so they can never collide with an app route.
- Publishes only what the API's `liveRule` would show NOW. A future go-live is left
  out, so rebuild when it arrives. Teacher pages are never written.
- Clears `--out` only if it holds a previous build (`.nabu-build` marker).
- Web app: `--public`, then `Options.Public`, then `./pb_public`.
- To check it like Cloudflare Pages: serve `dist/` with SPA fallback (missing paths
  -> index.html, status 200) and no API, then load `/nabu/help` in headless Chrome:
  `google-chrome --headless=new --dump-dom --virtual-time-budget=8000 <url>`.

## Exit codes

`pocketbase.Execute` discards the command's error. `App.Start` wraps every `RunE`
(`captureCommandErrors`) and returns it, so a failing `seed` exits 1 and fails CI.
`prepare()` must register every command, including `serve`, before Start decides
how to dispatch. `TestPrepareRegistersServe` guards that order.

## Testing the library API

`nabu_test.go` builds an App from `fstest.MapFS` content and runs `seed` through it.
Also check from outside the module, since that is how makerlab uses it:

```bash
mkdir /tmp/lc && cd /tmp/lc     # main.go embeds content/ + nabu.yaml (see README)
printf 'module example.nabu/lc\ngo 1.26\nrequire github.com/mrmxf/nabu v0.0.0\nreplace github.com/mrmxf/nabu => %s\n' ~/gr/nabu > go.mod
GOFLAGS=-mod=mod go mod tidy && go build -o lc .
mkdir /tmp/lc-run && cp lc /tmp/lc-run && cp -r ~/gr/nabu/pb_public /tmp/lc-run/
cd /tmp/lc-run && ./lc seed && ./lc serve   # no course files on disk
```

## Testing a schema change

Seed a data dir with the PREVIOUS binary, then migrate it with the new one:

```bash
git stash; go build -o /tmp/old-nabu ./cmd/nabu; git stash pop
go build -o /tmp/new-nabu ./cmd/nabu
/tmp/old-nabu seed --dir /tmp/t/pb_data
/tmp/new-nabu migrate up --dir /tmp/t/pb_data
echo y | /tmp/new-nabu migrate down 1 --dir /tmp/t/pb_data
```

Compare a roster export (`users export --format yaml`) before and after.
