---
title: Publishing and applying changes
params:
  readiness: production
  kind: presentation
---
**Publishing**

- New slides default to `readiness: DRAFT` — invisible to students until you set
  `production` or a go-live timestamp.
- A broken file is recorded as `content: CORRUPT` and skipped, without aborting
  the rest of the ingest.

**Applying your changes**

Re-run the seed to ingest edits:

```sh
go run ./nabu seed
```

That's it — your new slides appear the next time a student opens the lesson.

**Before you deploy to a running instance**

After editing content or rosters, the one command to run is:

```sh
nabu admin validate -q
```

It is **silent when everything is fine** and **noisy only on error** — a clean
(no-output, exit 0) run is your go/no-go gate before pushing. It catches the
common traps early: a new lesson that names an S3 bucket with no credentials set,
content that fails to parse, or a missing superuser. Fix any printed FAIL lines,
re-run until silent, then deploy.

