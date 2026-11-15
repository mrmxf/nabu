---
title: Run it
params:
  readiness: production
  kind: presentation
---
Load the course into Nabu's database:

```bash
nabu seed
```

Give yourself an account. Make a file called `roster.yaml` in the course folder,
with your own email and a password of your choice:

```yaml
users:
  - uid: me
    email: me@nabu.example
    name: Me
    role: teacher
    courses: [my-course]
    password: choose-a-long-password
```

Load it, then start the server:

```bash
nabu users import --from roster.yaml
nabu serve
```

Open **<http://127.0.0.1:8090>**, sign in with that email and password, and pick
your course.

`nabu serve` also prints a link that starts `http://127.0.0.1:8090/_/#/pbinstall/`.
Open it once to create the **superuser**, the administrator account for the
database dashboard at `/_/`. You only need the dashboard to look inside the
database.

To stop the server, press **Ctrl-C** in its terminal. From another terminal in
the same folder, `nabu stop` does the same.

The database lives in `pb_data/` inside the course folder. `nabu seed` is safe to
run again whenever the content changes: it updates the database to match. `roster.yaml`
holds a password, so `nabu init` keeps it out of git. The [rosters][rosters] page
explains every field.

[rosters]: help:for-teachers#rosters
