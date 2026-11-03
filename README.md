# Nabu

> **Nabu: the god who took notes when everyone else was making a mess.**
> Babylon's patron of scribes, wisdom and all the boring-but-essential stuff that
> keeps civilisation running. Nabu LMS: small, sharp, and quietly bossing knowledge.

Nabu is a small learning-management engine for hands-on courses where students
work through lessons slide by slide, collaborate in groups, and run AI agents. It
embeds [PocketBase][pocketbase] **v0.39.5** as a Go library (auth, database,
realtime, superuser dashboard) and serves a **Svelte 5** single-page app.

A course is a folder of Markdown files, kept in its own repository. Nabu reads
that course and drives students through it. The full tutorial, which teaches you
to build a Nabu course by building one, lives in [nabu-tutorial][tutorial].

## Two ways to run a course

- **Online, as a service on Cloudflare** (canonical): course pages from the CDN, sign-in
  at the edge, and one small container per course that sleeps when nobody is using it.
- **In a classroom, on a laptop:** one `nabu` file on the teacher's laptop, students on
  the room's Wi-Fi. No internet needed, and the data stays in the room.

Both run the same release. Container and Cloudflare deployment are still being built;
today Nabu runs as a single binary.

## Prerequisites

- Go 1.26+
- Node 22+
- git-lfs (images and media in this repo are stored with LFS)

## Quick start

```bash
./scripts/build.sh            # web -> ./pb_public, then ./bin/nabu
./bin/nabu seed               # the sample course + Nabu's built-in help
./bin/nabu serve              # http://127.0.0.1:8090
./bin/nabu stop               # (from another terminal) graceful stop
./bin/nabu build --out dist   # static site: guest lessons work with no server
./bin/nabu init ../my-course  # start a new course repo (--library for a Go binary)
```

Open **http://127.0.0.1:8090/nabu/help**: no sign-in is needed for the help or
the bundled `welcome-to-nabu` course.

`stop` finds the running server through a pid file in `pb_data`, reports how many
sessions it served, and shuts it down gracefully.

### Point Nabu at a course

A course repository carries a `nabu.yaml` that says where its files are and
overrides any of Nabu's defaults (see [embedfs/nabu-config.yaml][config]):

```yaml
# nabu.yaml, at the root of a course repo; paths are relative to this file
course:
  content: content                 # content/<course>/<lesson>/<slide>.md
  assets: assets
  schedules: content/lesson-schedules.yaml
  demo_roster: rosters/demo.yaml   # optional
pocketbase:
  app:
    name: My Course
```

Nabu finds it via `-c <file>`, `-C <course dir>`, or `./nabu.yaml`:

```bash
./bin/nabu -C ../my-course seed
./bin/nabu -C ../my-course serve
```

Flags such as `seed --content <dir>` override the file for one run.

### Superuser and demo logins

The PocketBase dashboard (`/_/`) superuser password is never a baked-in literal.
Give it one at seed time, or create the first superuser at `/_/` instead:

```bash
NABU_SUPERUSER_PW=some-dev-password ./bin/nabu seed
```

With `demo_mode: true`, the seed also creates demo accounts from the course's
`course.demo_roster`. Without one, it uses the course-neutral default in
[internal/demo/demo_mode.yaml][demo], whose accounts (`alice@nabu.example`,
`ben@nabu.example`, …, password `secret1234`) enrol in no course. The login page
lists whichever demo sign-ins are active.

### Dev mode

```bash
./scripts/dev.sh
# stop any running Nabu, rebuild, reseed, then serve on :8090 with secrets from
# Infisical (if installed) and a worker assumed on 127.0.0.1:8091
```

## Use Nabu as a Go library

A course can build its own self-contained binary by importing Nabu, the way Nabu
imports PocketBase. Everything the course needs is baked in with `//go:embed`:

```go
//go:embed all:content
var content embed.FS

//go:embed nabu.yaml
var config []byte

func main() {
	course, _ := fs.Sub(content, "content")
	app := nabu.New(nabu.Options{Content: course, ConfigYAML: config})
	// app embeds *pocketbase.PocketBase: add commands, hooks or routes here
	if err := app.Start(); err != nil {
		os.Exit(1)
	}
}
```

`./my-course seed` then `./my-course serve` work with no course files on disk. A
config file given at run time (`-c`, `-C`, `./nabu.yaml`) still overrides the
baked one. The web app is a build output, not part of the Go module: each release
will attach it as `nabu-web-vX.Y.Z.tar.gz`, fetched with `nabu web fetch` (planned)
and passed as `Options.Public`. Until then, put a built `pb_public/` beside the
data dir. See the [package docs][godoc] for every option.

## Agents

Lessons can dispatch jobs to an **agent worker** over a small HTTP contract
(`GET /healthz`, `POST /v1/jobs/dispatch`). Point Nabu at one with
`WORKER_HEALTH_URL`. A worker on loopback or the tailnet may use plain http. A
public host (e.g. on Cloudflare or Scaleway) must use https and a
`WORKER_DISPATCH_TOKEN`, optionally behind Cloudflare Access. See
[docs/secrets.md][secrets].

- [cmd/nabu-worker][worker] is a canned stub worker: the contract reference and a
  test double.
- [runtime/][runtime] is the TypeScript agent runtime a course builds its real
  agents on: register agents, then call `serveFromEnv()`.

## Layout

```
nabu.go            package nabu: the public library API (New, Options, App)
cmd/nabu/          the nabu binary: a thin wrapper over package nabu
cmd/nabu-worker/   stub agent worker (contract reference)
internal/          engine packages (content, seed, httpapi, students, agentsvc, …)
migrations/        Go schema migrations
embedfs/           embedded default config (nabu-config.yaml)
help/              Nabu's own help, a course embedded in the binary
content/           the bundled sample course (welcome-to-nabu)
web/               SvelteKit + Svelte 5 + PicnicCSS SPA (builds to ../pb_public)
runtime/           TypeScript agent runtime
docs/              engine documentation
scripts/           build.sh, dev.sh, buildinfo.sh, check-widget-docs.sh
```

`pb_data` and its sibling `pb_public` default to the directory Nabu is launched
from; override with `--dir`.

## Licence

BSD-3-Clause, see [LICENSE][license].

[pocketbase]: https://pocketbase.io
[godoc]: https://pkg.go.dev/github.com/mrmxf/nabu
[tutorial]: https://github.com/mrmxf/nabu-tutorial
[config]: embedfs/nabu-config.yaml
[demo]: internal/demo/demo_mode.yaml
[secrets]: docs/secrets.md
[worker]: cmd/nabu-worker/
[runtime]: runtime/
[license]: LICENSE
