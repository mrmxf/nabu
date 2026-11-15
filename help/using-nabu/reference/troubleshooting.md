---
title: Troubleshooting
weight: 90
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    What to do when Nabu will not start, will not let you sign in, or does not
    show your changes. Start with nabu doctor.
---
Mode: both. For agent jobs, see [troubleshooting agents][agents].

## Start with `nabu doctor`

```bash
nabu doctor                    # the checks every mode shares
nabu doctor --mode classroom   # or saas, or selfhost
```

It prints nothing when everything is ready. Otherwise each line names the
problem and a `fix`. Add `-v` to see the passing checks too. Run it from the
course folder, the one holding `nabu.yaml`.

## Common problems

**`address already in use`.** Another server is using the port, often a Nabu you
started earlier. Stop it with `nabu stop`, or use another port:
`nabu serve --port 8091`.

**`no pid file … but … is already in use`.** Something holds the port, but Nabu
did not start it, or lost track of it. `nabu stop` prints the commands to find
and stop it.

**`stop the server before seeding`.** `nabu seed` refuses while a server is
running, because both would write to the database at once. Run `nabu stop`, then
seed, then serve again.

**`content error(s)`.** A slide cannot be read. The line above names the file
and the problem, usually the frontmatter between the `---` lines. Fix it and run
`nabu seed` again.

**My changes do not show.** Content reaches the database only through
`nabu seed`. Stop the server, seed, serve, then reload the page.

**Sign-in fails.** Check the account exists: `nabu users export` lists everyone.
Import or fix it with `nabu users import --from roster.yaml`. The
[rosters][rosters] page explains the file.

**Students cannot reach the classroom.** Check they are on the same Wi-Fi, that
you allowed the firewall prompt for *private* networks, and that
`nabu doctor --mode classroom` finds a LAN address. Some guest networks keep
devices apart; use a phone hotspot instead.

**Fast login offers no Session.** A Session is open only inside its enrolment
dates. Check `content/lesson-schedules.yaml` (a Session with no `enroll` dates is
always open), then run `nabu seed`.

**I want a clean start.** Stop the server and delete `pb_data/`. The next
`nabu seed` builds a fresh database. Export anything you need first:
`nabu users export --positions --out results.yaml`.

## Two copies side by side

To compare two versions, give each its own port and its own database folder:

```bash
nabu serve --port 8090                     # the first, using ./pb_data
nabu serve --port 8091 --dir pb_data_b     # the second, using ./pb_data_b
nabu stop --dir pb_data_b --port 8091      # stop the second
```

## Still stuck?

- `nabu <command> --help` explains every command and flag.
- `nabu --version` names the release; quote it when you ask for help.
- The database dashboard at `/_/` shows every record, including the `activity`
  collection (who was active, and when).

[agents]: help:reference#troubleshooting-agents
[rosters]: help:for-teachers#rosters
