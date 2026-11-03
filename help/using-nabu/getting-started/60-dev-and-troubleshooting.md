---
title: Dev mode & troubleshooting
params:
  readiness: production
  kind: presentation
---
Instead of building each time, run both servers with live reload:

```bash
./nabu/scripts/dev.sh
# Nabu (Go/PocketBase) on :8090, Vite dev server on :5173 (proxies the API)
# open http://localhost:5173
```

**Run two instances side by side (A/B comparison)**

Give each instance its own **port** and its own **data dir** (they can't share one
`pb_data`). `pb_data` defaults to `./pb_data` in the current directory; `--port N`
is shorthand for `--http 127.0.0.1:N`:

```bash
./nabu/nabu serve --port 8090                    # instance A (default ./pb_data)
./nabu/nabu serve --port 8091 --dir pb_data_b    # instance B (./pb_data_b)
```

Stop a specific one by pointing `stop` at its data dir; add `--port` so the
fallback probe checks the right port if the pid file is missing:

```bash
./nabu/nabu stop --dir pb_data_b --port 8091
```

**Common fixes**

- **`address already in use`** — a previous server is still running. Stop it, or
  serve on another port: `./nabu/nabu serve --port 8091` (or the long form
  `--http=127.0.0.1:8091`).
- **`no pid file … but … is already in use`** — a server is holding the port but
  Nabu didn't start it (lost pid file). `stop` prints the `sudo` commands to find
  and kill it.
- **Login fails** — run `go run ./nabu seed` to (re)create the demo accounts.
- **Want a clean slate** — delete `pb_data/` (and `pb_public/`), rebuild, then
  `go run ./nabu seed` (it applies migrations automatically on the fresh dir).

The admin dashboard lives at **`/_/`** — sign in with the **superuser** to browse
the data, including the `activity` collection (who was active, and when). If you
seeded without `NABU_SUPERUSER_PW`, the first visit to `/_/` lets you create that
superuser.
