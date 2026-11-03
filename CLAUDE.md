# CLAUDE.md — Nabu

A small LMS engine for hands-on, agent-driven courses. PocketBase v0.39.5 is embedded as
a Go library, so Nabu IS PocketBase: auth, SQLite, realtime and the `/_/` dashboard. It
serves a Svelte 5 + PicnicCSS SPA. Public repo; courses live in their own repos and
point Nabu at themselves with a `nabu.yaml`.

`./scripts/build.sh` makes `pb_public/` then `bin/nabu`. `bin/nabu seed` loads the
course and the embedded help; `bin/nabu serve` runs on :8090. Verify any change with
`go vet ./... && go test ./...` plus `cd web && npm run check`: all must pass.

The bundled `content/` is one sample course, `welcome-to-nabu`: one lesson, one page that
points at [nabu-tutorial](https://github.com/mrmxf/nabu-tutorial). Do not grow it;
tutorial content belongs in nabu-tutorial.

## Detail — read on demand, not every query

| Topic | File |
|---|---|
| Build, test, config, versions | [claude-build.md](claude-build.md) |
| Deploy: today and the container/Cloudflare target | [claude-deploy.md](claude-deploy.md) |
| Known issues and deferred work | [claude-backlog.md](claude-backlog.md) |
| Why Nabu is shaped this way | [claude-human-narrative.md](claude-human-narrative.md) |
| Domain vocabulary | [docs/glossary.md](docs/glossary.md) |
| Env vars and secrets | [docs/secrets.md](docs/secrets.md) |
| Human-facing docs | [README.md](README.md) |

## Layout

```
nabu.go            package nabu — the PUBLIC API: New(Options) -> *App (embeds *pocketbase.PocketBase)
cmd/nabu/          the binary: nabu.New(nabu.Options{Automigrate: isGoRun()}).Start()
cmd/nabu-worker/   stub agent worker — the worker-contract reference and test double
internal/          engine packages: content (parse/render), seed, httpapi, students,
                   agentsvc (worker client), demo (neutral roster), roster, lmsfs, s3svc
migrations/        Go schema migrations; append-only
embedfs/           embedded nabu-config.yaml + typed kfg loader (course.*, pocketbase.*)
help/              Nabu's own help: a course, //go:embed'ed into the binary
content/           the bundled sample course
web/               SvelteKit SPA; builds to ../pb_public
runtime/           TypeScript agent runtime (@mrmxf/nabu-runtime); courses add agents
docs/              engine docs; docs/user/ is operator-facing
scripts/           build.sh, dev.sh, buildinfo.sh, check-widget-docs.sh
```

`ls internal/` is current; a table of packages is not.

## Rules

### Vocabulary
- Use the glossary terms exactly: course, lesson, session, group, attempt, slide.
- **Course** is the content container (`content/<course>/<lesson>/<slide>.md`). It was
  `class` until 2026-11; "class" survives only in the CSS/HTML sense and `classroom_*`.
- **Session** is the cohort window, collection `schedule_sessions`. **Attempt** is a
  student's run, collection `sessions`. The collections are not renamed yet — the
  glossary is the bridge. Do not "fix" one without the planned rename pass.
- A **deployment** is a repo + its container: normally one course plus the help.

### Course-neutral engine
- No course names in engine code, config, tests or comments. Agents are named by the
  course's content; the engine keys behaviour off authored fields, never agent names.
  Example: an image job is any agent whose authored `format` is `image`.
- Course-specific values (logo, app name, demo roster, paths) come from the course's
  `nabu.yaml`, never from a default in this repo.

### Identities
- Sample identities only: `alice@nabu.example`, hosts `*.nabu.example`, `s3.nabu.example`.
- The one exception is `LICENSE`, which keeps the author's real email.

### Library API
- `package nabu` (root) is the only public package; everything else is `internal/`.
  Anything a course needs must be reachable through `Options` or the embedded
  `*pocketbase.PocketBase` — never by importing `internal/`.
- Precedence for embedded trees: command-line flag > `Options` field > config path.
  Baked `ConfigYAML` sits UNDER any run-time config file.
- `New` sets package-level state (`embedfs.OverrideYAML`, `demo.Override`): one App per
  process. Tests that call `New` must reset them in `t.Cleanup`.
- Never default `Automigrate` on: under `go run` in a course repo it would write
  migration files into that repo. Only `cmd/nabu` turns it on.
- Test library behaviour from an external module too (`replace` to this repo), from an
  empty directory: no course files on disk. See claude-build.md.

### Sources and logs
- Read course files only through `source.Tree` (`internal/source`): `fs.ReadFile`,
  `fs.ReadDir`, `fs.Stat` on `tree.FS`, with slash paths relative to the tree root.
  Never `os.*` or `filepath.Join` inside the loader.
- Name files in logs and content issues with `tree.Where(rel)`: an absolute disk path,
  or `embed://<tree>/…` for a tree baked into the binary.
- `embed://` is a LABEL, never a path to open. Do not invent `/embed/…` paths: they read
  like real directories and can collide with one.

### Config
- The embedded `embedfs/nabu-config.yaml` holds defaults. A course overrides with
  `nabu.yaml`, found via `-c <file>`, `-C/--course-dir <dir>`, or `./nabu.yaml`.
- `course.*` paths are relative to the file that sets them — use `embedfs.ResolvePath`.
- A flag given on the command line beats the config. Check `Flags().Changed` or an empty
  default, never a non-empty default that masks the config.
- Global flags must not collide with subcommand flags: `--course` is a course SLUG on
  `users import/export`, which is why the directory flag is `--course-dir`.

### Schema and migrations
- Never edit an existing migration: they replay history on every live database.
  Change the schema with a new `migrations/1751NNNNNN_*.go`.
- A rename migration rewrites field names, rules and indexes in ONE save per collection,
  or PocketBase rejects an index naming a missing column. See `1751000028_*`.
- Every migration has a working down. Test both directions on a database seeded by the
  previous binary, not only on a fresh one.
- Automigrate is on only under `go run`; a released binary has no source tree.

### Renames and old spellings
- Accept the old spelling on input, never write it. Roster `classes:`, backup `class`,
  frontmatter `class_numbering`, `--class` and `?class=` all still read; exports write
  only the new names. Fold old fields into new ones at decode time, then clear them.

### Agents
- The worker contract is `GET /healthz` + `POST /v1/jobs/dispatch` (bearer, relative TTL).
  `cmd/nabu-worker` and `runtime/` both implement it; change all three together.
- Transport policy (`agentsvc.checkTransport`): loopback, tailnet, LAN and single-label
  hosts may use http. A public host needs https, and dispatch to it needs
  `WORKER_DISPATCH_TOKEN`. Refuse at startup with an `slog.Error` that names the fix.
- Keep the dispatch timeout under 100s: Cloudflare's edge cuts longer requests at 524.

### Go
- Effective Go and godoc on every exported name. Return `(T, error)`; never panic or
  `os.Exit` outside `main`. Wrap with `fmt.Errorf("context: %w", err)`.
- Logging is `log/slog` via `github.com/mrmxf/util/slogger`: Info for state changes,
  Debug for tracing, Warn for errors the task survives, Error (with a fix) for ones it
  doesn't. No `fmt.Println` for progress, no emoji in messages, no swallowed errors.
- Reach for `github.com/mrmxf/util` (kfg, slogger, buildinfo) before new plumbing.
- Keep queueing, grouping, sorting and output deterministic.

### Web
- Idiomatic Svelte 5 runes (`$state`, `$derived`, `$props`), typed props, PicnicCSS.
- `class=` and `class:` are CSS. They are never the domain concept.

### Content and widgets
- Authors touch frontmatter and `{{< LMS t="..." >}}` shortcode params. Nothing else.
- `internal/content/nabu-widget-catalogue.yaml` is the widget contract.
  `scripts/check-widget-docs.sh` fails unless `help/using-nabu/authoring-lessons/40-widgets.md`
  matches it — change both together.
- Content tests load `internal/content/testdata/course`, never a real course.

### Docs
- Markdown with reference-style links in a footer, mermaid where it helps.
- Audience: a beginner with no domain experience. British English.
- No links out of the repo except absolute public URLs. Mark a not-yet-shipped file
  as "not yet in this repo" rather than linking a private repo.
