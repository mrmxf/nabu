# claude-human-narrative.md — why Nabu is shaped this way

Background for [CLAUDE.md](CLAUDE.md). Nothing here is a rule. Read it when a decision
looks arbitrary and you want the reasoning, or when writing docs for people.

## Where it came from

Nabu was built inside the Secret Agents course: a summer school that teaches media
students to build and orchestrate AI agents. In November 2026 the engine moved here so
other courses could use it. The course kept its own copy and will move onto this repo
later. That history explains the leftover examples in the docs and the deprecated
`class` spellings the engine still accepts.

## Two ways to consume Nabu

- **Binary or container.** A course repo is Markdown plus a `nabu.yaml`. It never
  compiles Go. nabu-tutorial is built this way.
- **Go library.** A course imports `github.com/mrmxf/nabu`, embeds its own content, and
  builds its own binary, the way Nabu itself imports PocketBase. makerlab tests this.

Both must keep working, which is why course paths, names and rosters come from config
and never from defaults in this repo.

## Why the engine never names an agent

A course's agents are its teaching material. Secret Agents has a searcher, a sorter
and a presenter; another course will have a weather station. If the engine keys
behaviour off an agent's name, every new course needs an engine change. Keying off
authored fields (`format: image`, `icon`) keeps the engine generic.

## Why scale-to-zero shapes the design

Courses are used in bursts: a lesson, then nothing for a week. Browsing courses must
be instant and free, so it is static. Only the live parts of a lesson (groups,
realtime, agent jobs) need a container, and that container sleeps when idle. Logging in
happens at the edge so it can overlap with the container waking.

## Why a classroom laptop is a first-class mode

The SaaS on Cloudflare is the canonical way to offer a course to many cohorts. But a
lot of teaching happens in a room with unreliable Wi-Fi, for a group of students who
need to be working within minutes. There, a single binary on the teacher's laptop,
reached over the room's network, beats any cloud service: nothing to sign up for, no
uplink needed, and the data never leaves the room unless the teacher exports it.

Both modes run the same release, which keeps them honest. A feature that needs the
internet must degrade in the classroom rather than break it.

## Why sync, not transactions

The academy and a course container may each be asleep or down. A transaction needs
both up at once and would put academy latency on every slide. Instead each piece of
data has one writer: the academy owns the roster, the course owns progress. They
exchange signed, idempotent documents that can be cached and retried. Progress is
always saved locally first, so an academy outage never loses work.

## Why old spellings are still accepted

Live databases, exported rosters and backups predate the class -> course rename.
Refusing them would strand real data. Reading the old names and writing only the new
ones lets everything converge without a flag day.
