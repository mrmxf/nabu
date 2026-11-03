# Configuration reference

Every configurable property, with its default. The runtime values live in the
PocketBase `app_config` collection (editable at `/_/`); the ingest defaults are
seeded from `internal/seed/content.go` and can be overridden per-run by the
`seed` command flags.

## Global settings (`app_config`)

| Key | Default | Purpose |
|---|---|---|
| `site_logo` | `nabu/web/static/mi-mrmxf-c.svg` | Top-of-page site logo (served by basename). |
| `course_logo` | `nabu/web/static/rise-ccademy.png` | Course logo. |
| `lesson_logo` | `nabu/web/static/mi-bot-c.svg` | Lesson logo. |
| `auto_session_template` | `{{.YYYY}}-{{.MM}}-{{.DD}} @ {{.hh}}:{{.mm}}:{{.ss}}` | Auto session name. Placeholders: `YYYY MM DD hh mm ss USER`. |
| `auto_group_template` | `{{.USER}}-group` | Auto group name. |
| `course_numbering` | `CC` | Course display-number pattern (see below). |
| `lesson_numbering` | `CC.L` | Lesson display-number pattern. |
| `slide_numbering` | `CC.L.S` | Slide display-number pattern. |
| `renderer` | `goldmark` | Markdown renderer: `goldmark` (built-in) or `hugo` (external, not yet implemented). |
| `app_name` | `Nabu` | Name shown in the navbar and page title. Sourced from `pocketbase.app.name` in `nabu-config.yaml`. |
| `github_review_url` | `https://github.com/mrmxf/nabu` | "Review us" target for the help ShareBar. |
| `signup_require_email` | `false` | Whether `/signup` requires an email. Sourced from `signup.require_email` in `nabu-config.yaml`. |
| `demo_mode` | `true` | Whether `nabu seed` spawns the demo accounts (`rosters/demo/demo_mode.yaml`). Sourced from `demo_mode` in `nabu-config.yaml`. The login page reads it (with `/api/demo/status`) to warn when demo accounts linger while it is off. |
| `classroom_fast_login` | `false` | Whether `/login` offers the name-only classroom sign-in. Sourced from `classroom_fast_login` in `nabu-config.yaml`. **Password-less** — turn it on for a supervised lesson and off afterwards. See [classroom-fast-login.md](../classroom-fast-login.md). |

The `site_logo`, `signup_require_email`, `demo_mode` and `classroom_fast_login`
rows are seeded from `nabu-config.yaml` (`logopath`, `signup.require_email`,
`demo_mode`, `classroom_fast_login`). The content, assets and schedules paths
come from `course.*` in the config (a course's `nabu.yaml`); the `seed` command's
`--content`, `--assets`, `--schedules` and `--renderer` flags override them for
that run.

Because these are ordinary `app_config` rows, a value can also be flipped
live in the PocketBase dashboard at `/_/` without a rebuild or reseed — which is
how `classroom_fast_login` is meant to be switched on for a single lesson.

## Numbering patterns

`course_numbering`, `lesson_numbering` and `slide_numbering` share one grammar. A
pattern is literal separators plus repeated **level letters** — `C`=course,
`L`=lesson, `S`=slide. The **letter picks the level**; the **repeat count picks
the format**:

| Repeats | Format | Example |
|---|---|---|
| ×1 (`S`) | number (ordinal within parent) | `3` |
| ×2 (`SS`) | slug | `hunter` |
| ×3 (`SSS`) | title | `hunter — the search agent` |
| ×4 (`SSSS`) | number + title | `3 hunter — the search agent` |

Any other characters are literal. Example: `slide_numbering: "CC.L.S"` →
`<course-slug>.<lesson-number>.<slide-number>` → `rise-summer.1.3`.

Any level's pattern can be overridden per-item in frontmatter, e.g.
`params.slide_numbering: "S"`.

## Publishing status (two axes)

Every course, lesson and slide carries two orthogonal status axes.

**`readiness`** — authored in `params.readiness` (default `DRAFT`):

| Value | Meaning |
|---|---|
| `DRAFT` | Not published. Only visible to superusers in `/_/`. |
| `production` | Live now. |
| `deprecated` | Withdrawn. Not published. |
| `YYYY-MM-DD@HH:MM:SST±zzzz` | Scheduled: goes live at this datetime (e.g. `2026-09-01@09:00:00T+0100`). |

**`content`** — computed by the ingester, never authored:

| Value | Meaning |
|---|---|
| `OK` | Rendered cleanly. |
| `WARNINGS` | Rendered, but imperfect (missing image, unknown widget). |
| `ERRORS` | A part failed to render. |
| `CORRUPT` | Frontmatter/file could not be parsed. |

Students only ever receive slides that are **live** (`readiness` resolves to a
past go-live time) **and** healthy (`content` is not `ERRORS`/`CORRUPT`).
Superusers see everything, including drafts and broken items, in the `/_/`
dashboard.

## LMS widget catalogue

Inline `{{< LMS t="…" … >}}` widgets are defined in
`nabu/internal/content/nabu-widget-catalogue.yaml` (JSON-Schema-shaped, the single
source of truth). Current widget types: `timer`, `figure`, `note`. See the
_Authoring a lesson_ help lesson (served at `/nabu/help`, source
[nabu/help/using-nabu/authoring-lessons/](../../help/using-nabu/authoring-lessons)).

## PocketBase settings (`nabu-config.yaml` → `admin settings apply`)

Separate from the runtime `app_config` collection above, PocketBase has its own
**Settings** (application name/URL, SMTP, automatic backups). These are driven
from the `pocketbase:` block of `nabu/embedfs/nabu-config.yaml` (embedded in the
binary) and pushed into PocketBase with:

```bash
infisical run --env prod -- nabu admin settings apply --dry-run   # preview
infisical run --env prod -- nabu admin settings apply             # apply (prompts)
```

Mapped fields: `app.name/url` → Meta; `smtp.*` → SMTP; `backups.cron`,
`backups.cron_max_keep`, `backups.s3.*` → Backups. Secrets are **never** stored in
the file — `smtp.password_env` and `backups.s3.password_env` name the environment
variables (injected by Infisical) that hold the SMTP password and S3 secret key.
`nabu admin validate` checks those env vars are present when the feature is
enabled.

`settings apply` is **declarative**: it makes PocketBase match the config exactly,
so the shipped placeholder values must be edited to your real settings before
applying in production — always `--dry-run` first. It does not create the
superuser (use `nabu seed`, `nabu users import` with a `role: superuser` entry, or
PocketBase's first-run `/_/` page).
