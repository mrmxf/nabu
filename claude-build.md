# claude-build.md — build, test and config

Detail for [CLAUDE.md](CLAUDE.md). Deploy is in [claude-deploy.md](claude-deploy.md).

## Commands

```bash
./scripts/build.sh                     # npm install + build web -> ./pb_public, then bin/nabu
go vet ./... && go test ./...          # verify: must pass
(cd web && npm run check)              # svelte-kit sync + svelte-check; 0 errors
(cd runtime && npx tsc --noEmit)       # agent runtime typecheck
bash scripts/check-widget-docs.sh      # widget catalogue == 40-widgets.md; silent on success
./scripts/dev.sh                       # stop, rebuild, reseed, serve :8090 (Infisical if installed)
```

```bash
bin/nabu seed [--content d] [--assets d] [--schedules f] [--help-content d] [--strict]
bin/nabu serve [--http=127.0.0.1:8090] [--dir path/to/pb_data]
bin/nabu stop
bin/nabu -C ../my-course seed          # use ../my-course/nabu.yaml
bin/nabu admin validate [-q] [--deep]  # readiness gate; exits 1 on any FAIL
bin/nabu users import --from roster.yaml [--mode merge|overwrite|delete-orphans] [--course slug]
bin/nabu migrate up | down 1           # down asks for confirmation; pipe `echo y |`
```

`svelte-check` on its own reports stale route types after a route folder is renamed.
Run `npm run check`, which runs `svelte-kit sync` first.

## Facts

- Go 1.26+, Node 22+, git-lfs. Module path `github.com/mrmxf/nabu`.
- PocketBase v0.39.5 (Go), pocketbase JS SDK ^0.26, Svelte ^5.1, SvelteKit ^2.8, Vite ^6,
  PicnicCSS ^7.1, Yjs ^13.6.
- `mrmxf/util` packages used: `kfg` (config), `slogger` (logging), `buildinfo` (version).
- Version comes from git tags via `scripts/buildinfo.sh`, injected with `-ldflags`. A
  build on an exact tag is `prod`; anything else is `dev`.
- `pb_data` defaults to `./pb_data` in the launch directory; `pb_public` is ALWAYS its
  sibling. `--dir` moves both.
- The web build writes to `../pb_public` (`web/svelte.config.js`), i.e. the repo root.

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
- Help is extracted from the embedded `help.FS` to a temp dir unless `--help-content`.
- Demo accounts come from `course.demo_roster`, else `internal/demo/demo_mode.yaml`
  (neutral, enrolled in no course). Only with `demo_mode: true`.
- A superuser roster entry needs its `password_env` set, or it is skipped.

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
