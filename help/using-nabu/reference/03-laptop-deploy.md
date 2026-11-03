---
title: Laptop deployment (classroom on localhost)
weight: 18
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    Run the whole of nabu on a tech-savvy teacher's laptop and let students in the
    room log in over the same Wi-Fi. Fast, self-contained, no cloud — with the
    security trade-offs that come with it. Mac and Windows variants included.
---
This is the **laptop / classroom** path: a tech-savvy teacher (wearing an admin
hat) runs nabu on their own machine and students connect from their devices over
the local Wi-Fi. It's the quickest way to run a live session with no cloud, no
Caddy, no DNS. For a real hosted install use
[first deployment](help:reference#first-deploy) instead.

## ⚠ Security precautions — read first

Running this means **binding nabu to your local network so other devices can reach
it**. That is fine for a classroom on a network you trust, and risky anywhere else.

- **Trusted Wi-Fi only.** Use the school/venue network you control. **Never** on
  café / airport / hotel Wi-Fi — everyone on that network could reach your laptop.
- **Do not expose it to the internet.** No router port-forwarding, no public IP,
  no ngrok. LAN only.
- **It's plain HTTP.** Logins cross the Wi-Fi unencrypted (there's no TLS on the
  laptop). Acceptable for a short course on a trusted network; don't use real/reused
  passwords for student accounts.
- **Change the superuser password** (`/_/`) before anyone connects, and don't ship
  real secrets (S3 keys, worker tokens) to a laptop you carry around.
- **Student data lives in `pb_data` on your laptop.** Treat it per your school's
  policy and delete it when the session's done.
- **Allow the firewall prompt for _Private_ networks only** (see §4), and **stop
  nabu after the lesson** (`./nabu/nabu stop`).

## 0 · Prerequisites

- Go 1.26+, Node 20+, git, and a clone of the repo.
- Your laptop and the students' devices on the **same Wi-Fi**.
- **Windows:** run **natively** (PowerShell) for this — students can reach a native
  bind directly. Running inside WSL2 hides behind a NAT and needs extra
  `netsh interface portproxy` / mirrored-networking to be LAN-reachable.

## 1 · Build

```bash
# macOS / Linux
./nabu/scripts/build.sh          # SPA -> ./pb_public, then the Go binary
```

```powershell
# Windows (PowerShell) — the two steps build.sh runs, natively
cd nabu\web; npm ci; npm run build; cd ..\..
go build -o nabu\nabu.exe .\nabu
```

## 2 · Seed content + accounts

```bash
./nabu/nabu seed                       # showcase content + demo accounts
#   or point at your own courses:
./nabu/nabu seed --content /path/to/courses
```

Migrations apply automatically. The superuser (`admin@nabu.example`) is created only
when `NABU_SUPERUSER_PW` is set at seed time — otherwise **create the first
superuser** from the `/_/` page on first visit. Optionally import a course roster so
students have logins (`password:` is import-only):

```bash
./nabu/nabu users import --from /path/to/rosters/my-course.yaml --mode merge
```

(On Windows use `.\nabu\nabu.exe` in place of `./nabu/nabu`.)

## 3 · Find your laptop's Wi-Fi IP

Students will browse to `http://<this-ip>:8090`.

```bash
# macOS
ipconfig getifaddr en0            # en0 is usually Wi-Fi; try en1 if blank
```

```powershell
# Windows
ipconfig                          # read "IPv4 Address" under "Wireless LAN adapter Wi-Fi"
```

```bash
# Linux
hostname -I | awk '{print $1}'
```

## 4 · Run it, bound to the LAN

Bind to all interfaces (`0.0.0.0`) so the room can reach it — this is the one line
that differs from a private local run:

```bash
# macOS / Linux
./nabu/nabu serve --http 0.0.0.0:8090          # uses ./pb_data (the launch dir)
```

```powershell
# Windows
.\nabu\nabu.exe serve --http 0.0.0.0:8090 --dir nabu\pb_data
```

The first time, your OS asks to allow incoming connections:

- **macOS:** System Settings → Network → Firewall → allow `nabu` to receive
  connections.
- **Windows:** the Defender Firewall pop-up → tick **Private networks** only, then
  Allow.

## 5 · Students log in

Give the room the URL and their credentials:

> **http://&lt;your-ip&gt;:8090**  (e.g. `http://192.168.1.42:8090`)

They sign in and go. You administer at `http://<your-ip>:8090/_/` (or
`http://127.0.0.1:8090/_/` on your own machine).

## 6 · After the lesson

```bash
./nabu/nabu stop                  # graceful stop (reports sessions served)
```

Then handle `./pb_data` per your data policy (keep for the next lesson, or
delete). If you want the agents too, run a local worker and point
`WORKER_HEALTH_URL` at `http://127.0.0.1:8091/healthz` — but for a plain laptop
course the simulated/offline paths are usually enough. Validate anything you're
unsure of with the
[agent deploy runbook](help:reference#agent-deploy); secrets
guidance is in `docs/secrets.md`.
