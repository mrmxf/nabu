# Classroom fast login

A **stopgap** sign-in for live lessons. While it is switched on and a Session is
open, a student types only their name and is in the lesson two clicks later.

The normal path costs a classroom far too much time: sign in with a password, or
sign up with username + name + email + password + confirm, then pick a course, a
lesson, a session and a group. In a room of sixteen-to-twenty-six-year-olds on
school Wi-Fi that is several minutes of teaching lost before anyone sees a slide.

> **This is not a security feature and it is off by default.** Read
> [the tradeoff](#the-tradeoff) before turning it on.

## What a student sees

1. They open the site and land on `/login`.
2. One box: **Your name**. (Plus a course dropdown, but only if more than one
   Session is open at that moment.)
3. **Join** → straight to `/classroom/group`: pick a group, or start one.
4. **Start the lesson** → the lesson runner.

A `Teacher or admin?` link on the same page reveals the normal username/password
form, so staff sign in exactly as before.

## Turning it on

The flag is the `app_config` row `classroom_fast_login`. Two ways to set it:

- **For one lesson (no rebuild)** — open the PocketBase dashboard at `/_/`, find
  the `app_config` collection, set the `classroom_fast_login` row's `value` to
  `true`. Set it back to `false` when the lesson ends.
- **As a deployment default** — set `classroom_fast_login: true` in
  `nabu-config.yaml` and run `nabu seed`.

It also needs a **Session that is enrollable right now**: a `schedule_sessions`
row whose `enroll_from`/`enroll_to` window contains the current time (an empty
bound is open-ended). With no such Session the name box is disabled and the page
says *"No course is open right now — ask your teacher."* — so the login closes
itself when the course does, without anyone remembering to switch the flag back.

## Accounts it creates

| Field | Value |
|---|---|
| `uid` | `<name-slug>-<session-slug>`, e.g. `sam-tuesday-newsroom` |
| `name` | the name slug, e.g. `sam` — display name and username are the same |
| `email` | empty (optional since migration `1751000017_optional_email.go`) |
| `password` | a random 32-hex-char value nobody ever sees |
| `role` | `student` |
| `verified` | `true` |

The student is also enrolled in the Session's course, so they appear on the
teacher's roster and in `nabu users export`.

**Why the `-<session>` suffix.** It scopes the account to one cohort window, so
two students called Sam in two different Sessions get two independent accounts
and never collide — and a fast-login uid can never be the same string as a real,
password-protected account.

**Why a random password.** PocketBase forces the password field on an auth
collection to be required with a minimum length, so a literally blank password
cannot be stored. The student is never asked for one; the side effect is that
these accounts are unreachable from the normal password form, which is the safer
outcome.

**Typing the same name again** returns the same account — same uid, same work,
nothing duplicated. The group page notices an attempt already running for that
(lesson, Session) and offers **Continue** rather than silently starting a second
one.

## The tradeoff

With no password, **anyone who can reach the site while the flag is on can join
the open Session under any name, including a classmate's**. That is inherent to
"username only, no password"; it is the cost of the time saved, and it is why
this is a supervised-classroom tool rather than a deployment default.

What the server enforces regardless:

- **Off by default.** A missing or unreadable flag row means off.
- **Only while a Session is enrollable.** The posted `session_id` is checked
  against the live enrollable set on every request, not just in the UI — a stale
  browser tab cannot enrol into a window that has closed.
- **Never staff.** If the uid resolves to an account whose role is not
  `student`, the request is refused. A student typing `teacher-tom` gets a new
  student account `teacher-tom-<session>`; the real `teacher-tom` is untouched
  and unreachable by this path.

Switch it off after the lesson. The accounts it created remain — they are
ordinary student accounts, exportable and deletable through
`nabu users export` / the roster tools (see [student-management.md](student-management.md)).

## Implementation

| Piece | Where |
|---|---|
| Handlers, enrollable-Session resolution, uid scheme | [`nabu/internal/httpapi/classroom.go`](../internal/httpapi/classroom.go) |
| Route registration (both public) | [`nabu/internal/httpapi/routes.go`](../internal/httpapi/routes.go) |
| Config flag | `nabu/embedfs/nabu-config.yaml`, `nabu/internal/seed/content.go` |
| Client auth call + status cache | `nabu/web/src/lib/stores/auth.svelte.ts`, `classroom.svelte.ts` |
| Login page | `nabu/web/src/routes/login/+page.svelte` |
| Group step | `nabu/web/src/routes/classroom/group/+page.svelte` |

Two public endpoints (the caller is signed out, so they cannot be guarded):

- `GET /api/classroom/status` → `{enabled, sessions[]}`. Reports
  `{enabled:false, sessions:[]}` while the flag is off, so a normal deployment
  leaks no schedule data to anonymous callers.
- `POST /api/classroom/login` with `{username, session_id}` → the same
  `{token, record}` body `authWithPassword` returns. It mints the token with
  `apis.RecordAuthResponse`, so the existing `OnRecordAuthRequest("users")` hook
  still records the login in the `activity` collection.

Vocabulary here follows [the glossary](glossary.md): a **Session** is the cohort
window (`schedule_sessions`), an **Attempt** is a student's run (the `sessions`
collection), a **Group** is keyed by `(lesson, session, tag)`.
