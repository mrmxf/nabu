---
title: Sign in
params:
  readiness: production
  kind: presentation
---
With `demo_mode` on (the default), the seed creates these demo accounts — all
with the password `secret1234`:

| Role      | Email             | Signs in to        |
| --------- | ----------------- | ------------------ |
| Student   | `alice@nabu.example` | the student app    |
| Student   | `ben@nabu.example`   | the student app    |
| Student   | `cara@nabu.example`  | the student app    |
| Teacher   | `tom@nabu.example`   | the student app    |
| App admin | `ada@nabu.example`   | the app `/admin`   |

Signing in unlocks the action buttons you see disabled while browsing help as a
guest.

> **The PocketBase dashboard at `/_/` is separate.** Its superuser password never
> ships as a literal — the seed creates the demo superuser (`admin@nabu.example`)
> only when `NABU_SUPERUSER_PW` is set (see the previous *Run Nabu* step). Without
> it, open `/_/` once and use PocketBase's **"create your first superuser"** page.
