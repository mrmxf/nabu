# Nabu glossary — the airtight domain vocabulary

Status: **canonical** (adopted 2026-07-22). These five terms are the single
source of truth for user-facing text, code comments, and new identifiers. The
word **"session"** was historically overloaded — it meant a student's *run*, a
scheduled *cohort window*, and an `sXgY` *tag* all at once. It now means **only
the cohort window**.

## The terms

| Term | Definition |
|---|---|
| **Course** | The content container that owns lessons — a course/subject. It is **not** a cohort (that's `enrolments`) and **not** a schedule (that's a Session). |
| **Lesson** | A named collection of slides + activities; one teaching unit inside a course. |
| **Session** | A time/geography/cohort-windowed instance of a lesson, with enroll and start windows. One lesson may have many sessions (e.g. a Tuesday and a Thursday offering). |
| **Group** | Students collaborating within **one `(lesson, session)`**. Identity is `(lesson, session, tag)` where the tag is `g1`/`g2`/…. **No student-visible group state persists across sessions** — a group lives and dies with its session, so notes/choices/agent-config never leak between two offerings. |
| **Attempt** | A student's individual, remembered **run** of a lesson within a session. A student may make several attempts of the same `(lesson, session)`; each is resumable and kept distinct. |

`sXgY` (e.g. `s7g1`) is a **group-tag naming convention** — a habit for naming a
group, not a stored session id. There is no `s7` session field anywhere.

## Why "Attempt", not "Effort"

"Attempt" is standard LMS vocabulary in both British and American English
(Moodle/Canvas "quiz attempts") and unambiguously means *one discrete run*.
"Effort" reads as *how hard you tried* in American English (an effort grade), so
it was rejected as dialect-fragile.

## Phased code mapping (the bridge)

The DB collections are **not yet renamed** — a later mechanical pass will do that.
Until then, read code through this table:

| Canonical term | Collection / field in code today | Notes |
|---|---|---|
| **Attempt** | collection `sessions`; type `SessionRec`; route `/session/[id]`; `session_id` params | a student's run |
| **Session** | collection `schedule_sessions`; relation `schedule_session`; type `ScheduleSessionRec`; config `lesson_schedules` | the cohort window |
| **Group** | collection `groups`, keyed `(lesson, session, tag)`; runtime tag `group_name` | session-scoped |
| **Lesson** | collection `lessons` | ✓ already aligned |
| **Course** | collection `courses` | ✓ already aligned |

The Yjs collaboration room is `${lesson}:${session}:${group}` — including the
Session is what keeps two offerings of the same lesson+tag isolated.

### Deferred (mechanical rename pass)

`sessions`→`attempts`, `schedule_sessions`→`sessions`, `/session/[id]`→
`/attempt/[id]`, `SessionRec`→`AttemptRec`, and every `session_id` identifier
(~360 references). Safe to do once this glossary and the session-scoped Group
model are in place.
