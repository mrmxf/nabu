---
title: Prerequisites
params:
  readiness: production
  kind: presentation
---
You need two tools installed:

- **Go** 1.26 or newer — <https://go.dev/dl/>
- **Node** 22 or newer (includes `npm`) — <https://nodejs.org>

Check them:

```bash
go version     # go1.26+
node --version # v22+
```

You do **not** need to install PocketBase separately — it is embedded in Nabu
and fetched automatically as a Go dependency.
