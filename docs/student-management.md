# Student management

How to manage people (admins, teachers, students) at scale: bulk import/export
of users as CSV/YAML/JSON, course enrolment, rosters, and full backup/restore.

## Roles and courses

- Every user has a **role**: `admin`, `teacher`, or `student` (default `student`).
  A fourth value, `superuser`, is not a `users` role but a roster **routing hint**:
  such a row is applied to the PocketBase `_superusers` dashboard collection instead
  of `users` (see [Superusers via roster](#superusers-via-roster)).
- **Admins** manage users and rosters and can back up/restore. An admin is either
  a PocketBase superuser (the `/_/` dashboard account) or a `users` record with
  `role=admin`.
- **Teachers** and **students** are assigned to one or more **courses** via
  *enrolments*. A student's progress ("where they are") is tracked per lesson by
  `sessions.current_slide`.
- **Teachers** may **export** the roster of, and view positions for, their own
  courses — nothing else.

## Two operations: `export` (roster) vs `backup` (full)

|              | `export` / `import` (roster)                              | `backup` / `restore` (full)                                             |
| ------------ | --------------------------------------------------------- | ----------------------------------------------------------------------- |
| Purpose      | manage people; human-editable                             | lossless restore into a clean DB                                        |
| Contains     | uid, email, name, role, verified, enrolments (+ positions) | everything in export **plus** password hashes, `_superusers`, `sessions`, `activity` |
| Secrets      | none                                                      | password hashes, tokens                                                 |
| Formats      | CSV, YAML, JSON                                           | YAML, JSON (nested)                                                     |
| Who          | export: **admin + teacher**; import: **admin**            | **admin only**                                                          |

### Permissions matrix

| Operation           | superuser / admin | teacher        | student |
| ------------------- | ----------------- | -------------- | ------- |
| `export` (roster)   | ✔ all courses     | ✔ own courses  | ✗       |
| `import` (roster)   | ✔                 | ✗              | ✗       |
| `backup` (full)     | ✔                 | ✗              | ✗       |
| `restore` (full)    | ✔                 | ✗              | ✗       |
| roster / positions  | ✔ all courses     | ✔ own courses  | ✗       |

## The canonical user record

One row/object per user. **`uid` is the stable unique key used for all
matching** — a short admin-assigned handle (e.g. `s7g1-alice`). Because matching
is by `uid`, an admin can **change a user's `email` freely without breaking
anything**: the import re-matches on `uid`, and internal links (enrolments,
sessions) reference the PocketBase record id, which never changes.

| Field      | Required  | Notes |
| ---------- | --------- | ----- |
| `uid`      | yes       | **Stable unique key.** Admin-assigned handle (e.g. `s7g1-alice`); never reused. All import matching is by `uid`. |
| `email`    | on create | Unique but **mutable** — safe to change; `uid` preserves identity. |
| `name`     | on create | Display name. |
| `role`     | no        | `admin`\|`teacher`\|`student` (default `student`), or `superuser` to route the row to `_superusers`. |
| `courses`  | no        | Course **slugs** the user is enrolled in. `role_in_course` is derived from `role` (teacher→teacher, else student); admins normally have none. Ignored for superusers. |
| `password` | no        | Import-only. Sets/rotates the initial password; **never emitted on export**. |
| `password_env` | no    | Import-only. Names an env var (injected by Infisical) to read the password from when `password` is empty — so no secret is committed. **Required** for a `superuser` row. |
| `verified` | no        | Defaults to `true` on import. |

If `uid` is omitted on a first import, the row is matched by `email` and a `uid`
is generated (slug of the email local-part); subsequent exports carry the `uid`
so later renames are safe. Export adds read-only **position** data per enrolled
course when requested (see [Rosters](#rosters--a-courses-people-and-where-each-student-is)).

**CSV** (`courses` is `;`-separated because `,` is the delimiter):

```csv
uid,email,name,role,courses,password
s7g1-alice,alice@nabu.example,Alice,student,rise-summer,secret1234
s7g1-bob,bob@nabu.example,Bob,teacher,rise-summer;autumn-term,
admin-carla,carla@nabu.example,Carla,admin,,
```

**YAML** / **JSON** (same shape):

```yaml
users:
  - uid: s7g1-alice
    email: alice@nabu.example
    name: Alice
    role: student
    courses: [rise-summer]
  - uid: s7g1-bob
    email: bob@nabu.example
    name: Bob
    role: teacher
    courses: [rise-summer, autumn-term]
```

## Import modes

Matching is by `uid` (falling back to `email` only when `uid` is absent). Choose
one mode per import:

| Mode                 | Existing users in file | Their course set                    | Users *not* in the file |
| -------------------- | ---------------------- | ---------------------------------- | ----------------------- |
| `merge` (default)    | fields updated         | **added** (union)                  | untouched               |
| `overwrite`          | fields updated         | **replaced** by the file's `courses` | untouched             |
| `delete-orphans`     | fields updated         | **replaced**                       | within scope: **unenrolled**; a *student* left with no enrolments is **deleted**. Teachers, admins and superusers are never deleted. |

`delete-orphans` **scope** is the set of courses touched by the import, or a
single course when `--course`/`?class=` is given: orphans are enrolled students of
those courses absent from the file. Superusers and `role in (admin,teacher)` are
always exempt from deletion.

## Superusers via roster

A `role: superuser` row is the one way (besides `nabu seed` and PocketBase's
first-run `/_/` page) to create a dashboard admin — there is no `nabu superuser`
CLI command. Such a row:

- is applied to the `_superusers` collection, **not** `users`, and takes no
  `uid`, `courses` or enrolment;
- **must** carry `password_env` (never a literal `password`): the importer reads
  the password from that env var, injected by Infisical. Wrap the import so the
  secret is present:

  ```bash
  infisical run --env prod -- nabu users import --from roster.yaml
  ```

- is **skipped** (counted in `skipped`) when the env var is unset — an existing
  superuser is left untouched, so a box is never locked out of `/_/`.

```yaml
users:
  - email: admin@nabu.example
    role: superuser
    password_env: NABU_SUPERUSER_PW
```

## 1. From the CLI (on Nabu)

Runs with direct DB access, like `seed`. Format is inferred from the file
extension or forced with `--format`.

```bash
# roster export / import
nabu users export --out roster.csv [--role student] [--course rise-summer] [--positions]
nabu users import --from roster.yaml --mode delete-orphans --course rise-summer

# full backup / restore (admin only) — grouped under `admin`
nabu admin backup  --out backup.yaml
nabu admin restore --from backup.yaml
```

## 2. Hand-crafted file POSTed to an endpoint

Author a CSV/YAML/JSON file and POST it. `import` requires **admin** auth
(superuser **or** `role=admin`); roster `export` also accepts a **teacher**
(scoped to their courses). Format is taken from `Content-Type` (`text/csv`,
`application/yaml`, `application/json`).

```bash
# 1) authenticate (users collection, role=admin — or a teacher for export)
ATOK=$(curl -s -X POST http://HOST/api/collections/users/auth-with-password \
  -H 'Content-Type: application/json' \
  -d '{"identity":"carla@nabu.example","password":"…"}' | jq -r .token)

# 2) POST the roster file
curl -X POST 'http://HOST/api/users/import?mode=overwrite&class=rise-summer' \
  -H "Authorization: $ATOK" -H 'Content-Type: application/yaml' \
  --data-binary @roster.yaml
```

(A PocketBase superuser token from `/api/collections/_superusers/auth-with-password`
works identically.)

## 3. POSTed by an authenticated API client

Same endpoints, driven programmatically. `import` returns a JSON summary
`{ created, updated, enrolled, unenrolled, deleted, superusers, skipped, errors[] }`
(`superusers` counts `_superusers` upserted; `skipped` counts superuser rows whose
`password_env` was unset). Endpoints:

| Method + path                                             | Guard                             |
| --------------------------------------------------------- | --------------------------------- |
| `GET /api/users/export?format=&role=&class=&positions=1`  | admin, or teacher (own courses)   |
| `POST /api/users/import?mode=&class=`                     | admin                             |
| `GET /api/users/backup?format=yaml\|json`                | admin                             |
| `POST /api/users/restore`                                 | admin                             |
| `GET /api/courses/{slug}/roster`                          | admin, or teacher (own course)     |

## Rosters — a course's people and where each student is

- **Helper (Go):** `roster.CourseRoster(app, courseSlug)` returns the course, its
  teachers, and its students each with per-lesson **position**
  (`current_slide+1` of `total`, `percent`, `status`). Canonical source used by
  roster `export --positions`, the `/roster` endpoint, and the admin page.
- **Endpoint:** `GET /api/courses/{slug}/roster` returns that structure as JSON
  (a teacher is scoped to their own course).

## Full backup / restore

`backup` writes a nested YAML/JSON bundle that `restore` loads into a **clean**
DB to reproduce accounts and state:

```yaml
superusers: [{ email, password_hash }]
users:      [{ uid, email, name, role, verified, email_visibility, password_hash, enrolments: [{ course, role_in_course }] }]
sessions:   [{ user_uid, course, lesson, current_slide, status, session_name, group_name, work_state, created, updated }]
activity:   [{ user_uid, event, created }]
```

Restore matches users by `uid`, superusers by `email`, and rebuilds all links
via `uid`/slug (record ids are regenerated, but nothing references them across
the boundary). Passwords survive because the **hash** is restored (not a
re-hash), so users can log in immediately.

## What roster `export` does NOT preserve

A roster file is for managing people, not restoring a system. It **omits**:
passwords, `sessions` progress (`current_slide`/`work_state`), `activity`
history, `_superusers`, `avatar`, external-auth/MFA links, and original
timestamps. To reproduce a working system in a clean DB use **`backup`/`restore`**
(portable, user-scoped) or the whole-`pb_data` **`snapshot`/`install`** deploy
blob (full system — see [docs/user/deploy.md](user/deploy.md)).

## Safety notes

- Superusers bypass all record rules; the endpoints additionally accept
  `role=admin` (and, for `export`/roster, a scoped `role=teacher`).
- `delete-orphans` never deletes teachers, admins, or superusers, and only
  deletes a student once they have no remaining enrolments.
- Roster `export` never emits secrets; password hashes appear **only** in an
  admin `backup`.
