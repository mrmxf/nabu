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


**Bulky media (video, audio)**

Large files do not belong in git. Put them in the media store, which is usually an
S3 bucket mounted read-only, laid out as `<course>/<lesson>/media/<file>`. Then
point at them with `media:`:

```markdown
![Intro clip](media:intro.mp4)
<video src="media:demo.mp4" poster="media:demo.jpg" controls></video>
```

Nabu rewrites `media:intro.mp4` to `/content/<course>/<lesson>/media/intro.mp4`,
which `nabu serve --media-dir <store>` (or `course.media` in `nabu.yaml`) serves.
New courses from `nabu init` already keep `content/*/*/media/` out of git.

**A static copy for any web host**

`nabu build --out dist` writes the web app plus every **live** lesson as plain
files. Guest lessons then work on any static host (Cloudflare Pages, GitHub Pages)
with no Nabu server. Drafts, lessons scheduled for later, and teacher pages are
never published, so rebuild when a scheduled lesson goes live. Signed-in features
(sessions, groups, agents) still need `nabu serve`.
