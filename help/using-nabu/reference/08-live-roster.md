---
title: The roster secret, NABU_ROSTER_CSV (admin)
weight: 23
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    A live course on Cloudflare reads its attendance list from one secret,
    NABU_ROSTER_CSV. A copyable template, the rules for each column, how to store it
    in Infisical and hand it to the Worker, and how to check it before class.
---
This page is about the **SaaS on Cloudflare** deployment mode. On a classroom
laptop you import a roster file with `nabu users import` instead: see
[Rosters][rosters]. For the two modes, see [Deployment modes][modes].

A live course has no disk you can copy a file to. Its attendance list is a
**secret** called `NABU_ROSTER_CSV`: the whole CSV file, pasted as one multi-line
value. Each time the course container starts, it imports that list before it
serves anyone.

The list is also who may sign in. Only someone on it gets an emailed sign-in code,
and with `signup.enabled: false` in the course's `nabu.yaml` nobody can create an
account any other way.

## The template

Copy this, replace the rows with real people, and change `sample-lab` to your
course's slug: the folder name under `content/`.

```csv
email,name,role,courses
teacher@nabu.example,Teacher Name,teacher,sample-lab
alice@nabu.example,Alice Archer,student,sample-lab
betty@nabu.example,Betty Brown,student,sample-lab
ciara@nabu.example,Ciara Cole,student,sample-lab
```

That is all a sign-in needs: no passwords, because every sign-in is an emailed code.

| Column | What to put in it |
|---|---|
| `email` | Where the sign-in code goes. Each email appears once. Case does not matter. |
| `name` | What the teacher sees in the course. |
| `role` | `student` or `teacher`. Put yourself in as `teacher`, or you cannot sign in to teach. |
| `courses` | The course slug. For more than one, separate them with `;`, not `,`. |

Nabu makes each person's **username** from the part of their email before the `@`:
`alice@nabu.example` signs in as `alice`, or with the full email. If two people
share that part (`sam@a.nabu.example` and `sam@b.nabu.example`), add a `uid` column and
give each a username of your own, or the import skips the second one:

```csv
uid,email,name,role,courses
sam-a,sam@a.nabu.example,Sam A,student,sample-lab
sam-b,sam@b.nabu.example,Sam B,student,sample-lab
```

## What each import does

The container imports the list in **merge** mode every time it starts:

- a new row creates the account
- a changed name or role updates the account, and a new course enrols them in it
- a row you delete is **not** removed from the course. Remove a person in the
  dashboard at `/_/` (collection `users`) as well.

A course container sleeps when nobody has used it for a while and starts again on
the next visit, so an edited list takes effect after the next restart. To make it
take effect now, deploy the Worker again.

## Check it before class

A row with a mistake is skipped, and the container starts anyway, so a student
can be missing without anyone noticing until they cannot sign in. Try the list on
a throw-away database on your own machine first, from the course repo:

```bash
d=$(mktemp -d) && nabu --dir "$d" seed && nabu --dir "$d" users import --from roster.csv
```

It prints a summary, then one `error:` line for each skipped row:

```text
created=4 updated=0 enrolled=3 unenrolled=0 deleted=0 superusers=0 skipped=0
  error: sam@b.nabu.example: save user: uid: Value must be unique.
  error: bob@nabu.example: unknown course "no-such-course"
```

No `error:` lines means everyone is in. A wrong course slug still creates the
account but enrols it nowhere.

## Store it in Infisical

1. Open the course's folder, `/course-<slug>`, in the `prod` environment.
2. Add a secret named `NABU_ROSTER_CSV`.
3. Paste the whole CSV, header line included, as its value. Multi-line values are
   fine.

## Hand it to the Worker

The course Worker passes the secret to its container. From the course repo's
`worker/` folder, copy it across without it ever appearing on screen:

```bash
infisical secrets get NABU_ROSTER_CSV --env prod --path /course-<slug> --plain \
  | npx wrangler secret put NABU_ROSTER_CSV
```

Wrangler reads the value from its input. Then deploy the Worker again, so the
container restarts and imports the list.

A Worker secret holds up to 5 KB: about 80 rows. A bigger course needs the list
in R2 instead, which Nabu does not do yet.

[rosters]: help:for-teachers#rosters
[modes]: help:reference#deployment-modes
