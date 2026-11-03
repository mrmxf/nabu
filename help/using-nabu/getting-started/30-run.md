---
title: Run Nabu
params:
  readiness: production
  kind: presentation
---
Start the server:

```bash
./nabu/nabu serve
# → http://127.0.0.1:8090
```

Then, in a **second terminal**, load the demo accounts and showcase lesson (safe
to re-run any time — it is idempotent):

```bash
go run ./nabu seed
```

This seeds the demo students, teacher and app-admin. To **also** get a PocketBase
dashboard (`/_/`) login, give the superuser a password first — it is never a
baked-in literal:

```bash
NABU_SUPERUSER_PW=some-dev-password go run ./nabu seed
```

(Skip it and you can still create the first superuser from the `/_/` page later.)

Open **<http://127.0.0.1:8090>** in your browser.

To stop Nabu gracefully (it also reports how many sessions it served this run):

```bash
./nabu/nabu stop
```
