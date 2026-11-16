---
title: Rosters
params:
  readiness: production
  audience: teacher
  kind: presentation
---
A **roster** is a plain, human-editable file that lists the people in one or more
courses — who they are, what role they have, and which courses they belong to.
You author it in a spreadsheet or text editor, hand it to Nabu, and Nabu creates
or updates the accounts to match. Export it back at any time to see where every
student has got to.

A roster is for **managing people**, not for backing up the system. It never
contains passwords once exported, no lesson progress, and no secrets — so it is
safe to email, edit, and check into a shared drive. (For a lossless copy of a
whole system, that is a `backup`, not a roster — see the bottom of this page.)

## The roster pattern

Every roster is a list of users. Each user is one row (CSV) or one list item
(YAML/JSON), and every user has:

| Field      | Required   | What it does |
| ---------- | ---------- | ------------ |
| `uid`      | recommended | **The stable unique key.** A short handle you assign (e.g. `s7g1-alice`). Everything matches on `uid`, so you can change someone's email later without breaking their account. Never reuse a `uid`. |
| `email`    | on create  | Unique, but **safe to change** — the `uid` preserves identity. |
| `name`     | on create  | The display name students and teachers see. |
| `role`     | no         | `student` (default), `teacher`, or `admin`. |
| `courses`  | no         | The course **slugs** the person is enrolled in. Their role *in* the course follows their `role` (a teacher teaches, everyone else learns). |
| `password` | no         | Import only — sets or rotates the initial password. **Never written out on export.** |
| `verified` | no         | Defaults to `true` on import. |

**The one rule worth remembering:** matching is by `uid`. If you leave `uid`
out, Nabu matches on `email` instead and generates a `uid` for you from the
email (the part before the `@`). The next export then carries that `uid`, so any
later rename is safe. Give people a `uid` up front and you never have to think
about this again.

## A minimal viable roster

The smallest roster that does something useful: an email, a name, and the course
to put people in. Everyone here becomes a **student** (the default role) in the
`sample-lab` course. Because no `uid` is given, Nabu generates one for each from
the email — `alice`, `betty`, `ciara`, `donald`.

**CSV** — note `courses` is separated with `;` because `,` is the column
delimiter:

```csv
email,name,courses
alice@nabu.example,Alice,sample-lab
betty@nabu.example,Betty,sample-lab
ciara@nabu.example,Ciara,sample-lab
donald@nabu.example,Donald,sample-lab
```

The same thing in **YAML**:

```yaml
users:
  - email: alice@nabu.example
    name: Alice
    courses: [sample-lab]
  - email: betty@nabu.example
    name: Betty
    courses: [sample-lab]
  - email: ciara@nabu.example
    name: Ciara
    courses: [sample-lab]
  - email: donald@nabu.example
    name: Donald
    courses: [sample-lab]
```

That is enough to create four student accounts and enrol them. Everything else
takes a sensible default: `role` is `student`, `verified` is `true`, and each
`uid` is generated from the email.

## A full roster, every option

Here is the same course with every field filled in — including two **teachers**,
Xavier and Yaja. Teachers get `role: teacher`; being listed against a course
enrols them as that course's teacher rather than as a learner. Note that Xavier
teaches two courses (`sample-lab` and `autumn-term`), and everyone has an
explicit `uid` so you can rename their email freely later.

**CSV**:

```csv
uid,email,name,role,courses,password,verified
s7g1-alice,alice@nabu.example,Alice,student,sample-lab,secret1234,true
s7g1-betty,betty@nabu.example,Betty,student,sample-lab,secret1234,true
s7g1-ciara,ciara@nabu.example,Ciara,student,sample-lab,secret1234,true
s7g1-donald,donald@nabu.example,Donald,student,sample-lab,secret1234,true
s7g1-xavier,xavier@nabu.example,Xavier,teacher,sample-lab;autumn-term,secret1234,true
s7g1-yaja,yaja@nabu.example,Yaja,teacher,sample-lab,secret1234,true
```

**YAML**:

```yaml
users:
  - uid: s7g1-alice
    email: alice@nabu.example
    name: Alice
    role: student
    courses: [sample-lab]
    password: secret1234
    verified: true
  - uid: s7g1-betty
    email: betty@nabu.example
    name: Betty
    role: student
    courses: [sample-lab]
    password: secret1234
    verified: true
  - uid: s7g1-ciara
    email: ciara@nabu.example
    name: Ciara
    role: student
    courses: [sample-lab]
    password: secret1234
    verified: true
  - uid: s7g1-donald
    email: donald@nabu.example
    name: Donald
    role: student
    courses: [sample-lab]
    password: secret1234
    verified: true
  - uid: s7g1-xavier
    email: xavier@nabu.example
    name: Xavier
    role: teacher
    courses: [sample-lab, autumn-term]
    password: secret1234
    verified: true
  - uid: s7g1-yaja
    email: yaja@nabu.example
    name: Yaja
    role: teacher
    courses: [sample-lab]
    password: secret1234
    verified: true
```

> **Passwords are import-only.** They set the *initial* password when the account
> is created (or rotate it on a later import). When you **export** a roster, the
> `password` column comes back empty — Nabu never writes secrets into a roster.

## Signing in with an emailed code

Students do not need a password at all. Leave the `password` column out, and the
login page offers **Email me a sign-in code**: a student types their username or
email, and Nabu emails a 6-digit code to the **email in the roster**, never to the
typed text. The code works for 10 minutes, and a sign-in lasts 9 hours.

So the roster is also the list of who may sign in. Nobody else gets a code, and the
page looks the same either way, so it does not reveal who is on the list. The
smallest roster for this is a CSV with two columns, plus the course:

```csv
email,name,role,courses
sam.lee@nabu.example,Sam Lee,student,sample-lab
```

Nabu makes each username from the email (`sam-lee` here). Sending the codes needs a
mail provider in the course's `nabu.yaml`; Nabu sends through the provider's API
(Brevo or Mailtrap), with the API key in an environment variable:

```yaml
mail:
  provider: brevo                 # or mailtrap
  api_key_env: BREVO_API_KEY      # the env var holding the key, never the key
  sender_name: Sample Lab
  sender_address: lab@nabu.example
  sender_address_env: NABU_SENDER_EMAIL   # optional: its value replaces sender_address
signup:
  enabled: false                  # no "Create an account": the roster is the only way in
```

`nabu doctor` fails if the key is missing. Codes need the internet, so a classroom
laptop with no uplink uses `nabu classroom` (name-only sign-in) instead. On a live
course on Cloudflare the roster is a secret: see [The roster secret][live-roster].

## Signing in with a room code

Some school and university mail systems hold back emailed codes, so they never
reach the student. A **room code** avoids email. Turn it on in the course's
`nabu.yaml`:

```yaml
room_code_login: true
```

While a Session is open, the **Teacher** page shows a 6-digit code for each course,
in large type, with a **Full screen** button for the classroom display. A student
types their username (or email) and that code, and is signed in.

- Only people on the roster, enrolled in that course as students, can use it.
  Nobody else gets in, and no account is created.
- Teachers and admins cannot use it: they sign in with an emailed code or a
  password.
- The code changes every 10 minutes, and the previous one still works, so a
  student who is slow to type is not caught out. Five wrong tries in a row and a
  name must wait 5 minutes.
- It works only while one of the course's Sessions is open.

The tradeoff: anyone who can see the screen and knows a classmate's username can
sign in as them. Show the code in the room only, never in a recording or a chat.

## Importing a roster

Only an **admin** can import a roster (it creates and changes accounts). You
choose one **mode** per import, which decides what happens to people's course
enrolments:

| Mode                 | People in the file                | People *not* in the file |
| -------------------- | --------------------------------- | ------------------------ |
| `merge` (default)    | fields updated; courses **added** (union) | left untouched |
| `overwrite`          | fields updated; courses **replaced** by the file | left untouched |
| `delete-orphans`     | fields updated; courses **replaced** | within scope: **unenrolled**; a *student* with no courses left is **deleted** |

`delete-orphans` is the one to handle with care: it removes students who are
enrolled in the course but missing from your file. Its **scope** is the courses
the file touches (or a single `--course` you name). **Teachers, admins, and
superusers are never deleted** — only students, and only once they have no
courses left.

From the CLI on the Nabu host:

```bash
# create/update everyone in the file, adding course enrolments (safe default)
nabu users import --from roster.yaml

# make sample-lab exactly match the file — drop students who aren't in it
nabu users import --from roster.yaml --mode delete-orphans --course sample-lab
```

## Exporting a roster — and seeing where students are

**Admins can export any course; a teacher can export their own courses** and
nothing else. Export gives you the roster back as CSV, YAML, or JSON, and — when
you ask for positions — adds a read-only column for **where each student has got
to** in every lesson (which slide, a percentage, and a status).

```bash
# export just the sample-lab roster, with positions, as CSV
nabu users export --out roster.csv --course sample-lab --positions
```

The live view of a course — its teachers, its students, and each student's
position — is also available as a page in Nabu and over the API at
`GET /api/courses/{slug}/roster` (a teacher is scoped to their own course).

## When you need more than a roster

A roster deliberately **leaves out** passwords, lesson progress, activity
history, avatars, and superuser accounts. That is what makes it safe to share.

If your goal is to reproduce a *working system* — accounts that can log in
immediately, with progress intact — you want a full **`backup`** (admin only),
which restores password hashes and session state into a clean database. Rosters
move *people* between courses; backups move a *system* between machines.

[live-roster]: help:reference#live-roster
