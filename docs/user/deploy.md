# Deploy the demo to an online server

This walks you from your **local machine** to a **live HTTPS URL** anyone can
click. The trick: Nabu can bake its whole database and web UI
*inside the binary*, so deploying is really just "copy one file up, let it unpack
itself, and put Caddy in front for TLS".

If you only want to run the demo on your own laptop, follow the *Getting
started* help lesson (served at `/nabu/help`, source
[nabu/help/using-nabu/getting-started/](../../help/using-nabu/getting-started))
instead — this guide is about putting it online.

## How it works

The whole cycle is seven steps — the first five are just getting the demo how
you want it locally, then two to ship it:

```
 LOCAL                                        SERVER
 1 clone  2 build (lean nabu)                 ┌────────────────────────────┐
 3 edit   4 seed/import  5 test        scp    │ install → serve :8090      │
 6 snapshot → nabu-deploy ───────────────────→│            ▲               │
   (db + UI baked in)                         │   Caddy ───┘ (HTTPS :443)  │
                                              └────────────────────────────┘  7 deploy
```

1. **clone** the repo.
2. **build** — `./nabu/scripts/build.sh` compiles the **lean** `nabu/nabu` (web
   UI + Go engine, *no* embedded data — so the dev binary stays small).
3. **edit** your content tree and user roster.
4. **seed & import** — `nabu seed`, then `nabu users import` your roster.
5. **test** — `nabu serve` and click through until you're happy.
6. **snapshot** — `nabu snapshot` bakes the current `pb_data` + `pb_public` into a
   separate **`nabu-deploy`** binary, then moves the archive to `/tmp` so nothing
   lingers in the tree. Only `nabu-deploy` carries the data.
7. **deploy** — `scp nabu-deploy` up, `install` unpacks the embedded data, `serve`
   runs it on loopback, and **Caddy** fronts it with HTTPS.

> **Why two binaries?** `nabu` is the lean dev binary (no embed). `nabu-deploy` is
> the shippable one with `pb_data`/`pb_public` baked in. Keeping the embed out of
> normal builds is what stops the working tree and dev binary from bloating.

## Prerequisites

**On your local machine** (to build):

- **Go** 1.26+ and **Node** 22+ — same as the *Getting started* help lesson.

**On the server** (a small Linux VM is plenty — e.g. a Scaleway instance):

- SSH access (you can `ssh user@your-server`).
- A **domain name** pointing at the server's public IP (an `A` record), e.g.
  `demo.example.com`. Caddy needs this to issue a TLS certificate.
- **Caddy** installed — <https://caddyserver.com/docs/install>. On Debian/Ubuntu
  it installs as a systemd service and listens on ports 80 and 443.
- Ports **80** and **443** open to the internet; port **8090 closed** (only Caddy
  should be reachable — the app stays on loopback).

## 1. Build, seed, test locally

From the repository root on your local machine:

```bash
./nabu/scripts/build.sh            # step 2: web UI + lean nabu/nabu
# step 3: edit your content tree and roster
go run ./nabu seed                 # step 4: demo content + accounts
go run ./nabu users import --from roster.yaml   # (optional) your real roster
./nabu/nabu serve                  # step 5: test at http://127.0.0.1:8090
```

> **The dashboard superuser is not baked in unless you supply its password.** The
> demo superuser's password comes from `NABU_SUPERUSER_PW` (via Infisical), never a
> committed literal. Seed under Infisical to create it —
> `infisical run --env prod -- go run ./nabu seed` — otherwise you ship the demo
> *students* but no `/_/` admin, and create the first superuser on the server (see
> [Sign in](#sign-in-and-verify)).

## 2. Snapshot into `nabu-deploy`

Stop the dev server first (a clean DB copy), then bake the current `pb_data` +
`pb_public` into the deploy binary:

```bash
./nabu/nabu stop
go run ./nabu snapshot
```

`snapshot` packs the trees, rebuilds **`nabu-deploy`** with `-tags deploy` so
go:embed bakes the archive in, then moves the archive to `/tmp` (dated) so the
working tree stays clean. The result:

> **`snapshot` captures the DATABASE, not your `content/` files.** A slide/lesson
> only enters `pb_data` when you run `seed` (step 4). So the rule is: **edit content
> → `seed` → `snapshot`.** If you snapshot without re-seeding, your latest content
> (e.g. a new slide) simply isn't in the blob — the running server looks fine, but
> the deployed copy is stale. `snapshot` checkpoints the SQLite WAL before packing,
> so a clean `stop` + `seed` always produces a complete capture; if it can't fully
> checkpoint (server still running) it warns — heed that and `stop` first.

```
nabu-deploy    # self-contained, carries pb_data + pb_public (git-ignored)
```

> **Whatever is in `pb_data`/`pb_public` at snapshot time is what ships.** Snapshot
> only after you're happy with the seeded/imported content.

### Building for a different CPU (e.g. an ARM server)

`nabu-deploy` must match your **server's** CPU, not your laptop's. The server uses
a pure-Go database driver, so cross-compiling needs no C toolchain — `snapshot`
already forces `CGO_ENABLED=0` and inherits `GOOS`/`GOARCH` from the environment.
Run the **host** `nabu` binary (not `go run`, which would try to run a foreign
arch) with the target set:

```bash
# amd64 (Intel/AMD VM):
GOOS=linux GOARCH=amd64 ./nabu/nabu snapshot
# arm64 (ARM VM):
GOOS=linux GOARCH=arm64 ./nabu/nabu snapshot
```

Check the target with `uname -m` on the server (`x86_64` → amd64, `aarch64` →
arm64).

## 3. Copy it to the server (scp)

Create a home for it on the server, then copy the deploy binary up:

```bash
ssh user@your-server 'mkdir -p /opt/secret-agents'
scp nabu-deploy user@your-server:/opt/secret-agents/
```

If `/opt` needs root, either `sudo mkdir` and `chown` it to your user first, or
scp to your home directory and `sudo mv` it into place.

> **Footnote — `Too many authentication failures`?**
>
> If `scp`/`ssh` dies with `Received disconnect ... Too many authentication
> failures`, the problem is **not** the server — it's that your SSH agent is
> offering too many keys. Agents such as **Bitwarden** load *every* key you have,
> and SSH tries them one by one. The server closes the connection once it hits its
> `MaxAuthTries` limit (default 6), often before your correct key is even reached.
>
> The fix is to tell SSH exactly which key to use. Point `-i` at the **public**
> key file (not the private one — the private key stays in the agent) and add
> `IdentitiesOnly=yes` so SSH offers *only* that key:
>
> ```bash
> # Save the public key you want (here, the Bitwarden key named "MyDemo") to a file
> ssh-add -L | grep 'MyDemo$' > ~/.ssh/MyDemo.pub
>
> # Then force SSH to use only that key
> scp -o IdentitiesOnly=yes -i ~/.ssh/MyDemo.pub \
>   nabu-deploy user@your-server:/opt/secret-agents/
> ```
>
> To make it permanent, add a host entry to `~/.ssh/config` so plain
> `scp`/`ssh`/`rsync` just work:
>
> ```sshconfig
> Host your-server
>     HostName your-server.example.com
>     User user
>     IdentityFile ~/.ssh/MyDemo.pub
>     IdentitiesOnly yes
> ```

## 4. Install on the server (unpack the embedded data)

SSH in and run **`install`**. It extracts the embedded `pb_data` to the path you
pass with `--dir`, and `pb_public` to its sibling directory:

```bash
ssh user@your-server
cd /opt/secret-agents
./nabu-deploy install          # extracts to ./pb_data + ./pb_public (the launch dir)
```

> `pb_data` defaults to `./pb_data` in the directory you run from, so running from
> `/opt/secret-agents` needs no `--dir`. Pass `--dir <path>` to put it elsewhere.

You'll get:

```
/opt/secret-agents/
├── nabu-deploy          # the binary
├── pb_data/              # database + accounts + lessons (unpacked)
└── pb_public/            # the web UI (unpacked)
```

`install` is **non-destructive**: if `pb_data`/`pb_public` already exist and are
non-empty it **leaves them alone** and logs `target exists, skipping; use --force
to overwrite` (so re-running never wipes live data).

> **Updating an existing deploy REQUIRES `--force`.** On a first install the target
> is empty, so plain `install` works. But on every *later* deploy the box already
> has a `pb_data`, so a plain `install` silently keeps the **old** content — this is
> the usual reason a new slide/lesson "doesn't show up" after redeploying. Use
> `./nabu-deploy install --force` to replace it — see
> [Redeploying an update](#6-redeploying-an-update).

## 5. Run it behind Caddy

### Run the server (on loopback)

Bind the app to `127.0.0.1:8090` so it is **not** directly reachable from the
internet — only Caddy will talk to it. For a one-off test:

```bash
cd /opt/secret-agents && ./nabu-deploy serve --http 127.0.0.1:8090
```

For a real deployment, run it as a **systemd service** so it starts on boot and
restarts on failure. `WorkingDirectory` sets the launch dir, so `serve` finds
`./pb_data` without `--dir`. Create `/etc/systemd/system/secret-agents.service`:

```ini
[Unit]
Description=Secret Agents demo server
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/secret-agents
ExecStart=/opt/secret-agents/nabu-deploy serve --http 127.0.0.1:8090
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Then enable and start it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now secret-agents
sudo systemctl status secret-agents        # should be "active (running)"
```

The server handles `SIGTERM` gracefully, so `sudo systemctl stop secret-agents`
shuts it down cleanly (and logs how many sessions it served).

### Put Caddy in front

Point Caddy at the app. Edit `/etc/caddy/Caddyfile` (replace the domain):

```caddyfile
demo.example.com {
    reverse_proxy 127.0.0.1:8090
}
```

Reload Caddy:

```bash
sudo systemctl reload caddy
```

Caddy automatically obtains and renews a Let's Encrypt TLS certificate for the
domain, listens on 443, and proxies everything to the app on 8090 — including
PocketBase's realtime (SSE) stream, which it streams through without extra
config.

Open **`https://demo.example.com`** — the demo is live.

## Sign in and verify

The seeded **demo student/teacher** accounts travel inside the blob, so they
already work (only when `demo_mode` was on at seed time):

| Role    | Email             | Password     |
| ------- | ----------------- | ------------ |
| Student | `alice@nabu.example` | `secret1234` |
| Teacher | `tom@nabu.example`   | `secret1234` |

The **dashboard superuser** (`/_/`) is different — its password is never a baked
literal:

- If you built under Infisical, the demo superuser `admin@nabu.example` exists with
  whatever `NABU_SUPERUSER_PW` held; open **`https://demo.example.com/_/`**.
- If not, no superuser is baked in. On first `serve`, PocketBase prints a one-time
  **"create your first superuser"** link (`/_/#/pbinstall/<token>`) in the logs —
  open it to set up the admin. Alternatively import a roster with a
  `role: superuser` entry (`nabu users import`, see
  [student-management.md](../student-management.md#superusers-via-roster)). There is
  no `nabu superuser` CLI command.

> **Change the demo passwords before sharing a public link.** The student demo
> logins are well-known — fine for a quick funding demo, not for anything sensitive.
> Update them in `/_/`, or re-seed with `demo_mode` off and your own roster before
> snapshotting.

## 6. Redeploying an update

When you change the UI or content, re-snapshot and push it again:

```bash
# local: rebuild lean + reseed/test if needed, then re-snapshot (steps 1–2,
# plus cross-compile with GOOS/GOARCH if the server differs)
./nabu/nabu stop && go run ./nabu snapshot

# local → server: copy the new binary up
scp nabu-deploy user@your-server:/opt/secret-agents/

# server: replace the on-disk data with the new snapshot, then restart
ssh user@your-server
cd /opt/secret-agents
./nabu-deploy install --force          # ./pb_data + ./pb_public (the launch dir)
sudo systemctl restart secret-agents
```

> `--force` replaces **both** `pb_data` and `pb_public` with the embedded
> snapshot — it resets the instance to your baked-in content, discarding any
> sessions students created live. If you need to keep live data, copy `pb_data`
> aside first (`cp -r pb_data pb_data.bak`), or re-snapshot from the server's
> current `pb_data` instead of the demo seed.

## Troubleshooting

- **`this binary has no embedded content`** on `install` — you ran a lean `nabu`,
  not `nabu-deploy`. Produce the deploy binary with `go run ./nabu snapshot` (step
  2) and install *that*.
- **`install` says "target exists, skipping"** — `pb_data`/`pb_public` are
  already there. That's the safe default; add `--force` to overwrite (step 6).
- **`cannot execute binary file` / `Exec format error`** on the server — the
  binary was built for the wrong CPU. Rebuild for the server's arch (see
  [Building for a different CPU](#building-for-a-different-cpu-eg-an-arm-server)).
- **Caddy can't get a certificate** — check the domain's `A` record points at the
  server and ports 80/443 are open; `sudo journalctl -u caddy -f` shows the ACME
  errors.
- **502 Bad Gateway** from Caddy — the app isn't running or isn't on 8090. Check
  `sudo systemctl status secret-agents` and that `--http 127.0.0.1:8090` matches
  the `reverse_proxy` address.
- **Site loads but student login fails** — you snapshotted before seeding, or with
  `demo_mode` off. Seed (step 4), then re-`snapshot` (step 2) with demo_mode on.
- **Can't reach `/_/` — no superuser** — expected if you built without
  `NABU_SUPERUSER_PW`. Use the one-time installer link `serve` prints, or import a
  `role: superuser` roster (see [Sign in](#sign-in-and-verify)).

## Where things live

- `nabu/scripts/build.sh` — builds the web UI + the **lean** `nabu/nabu` (no embed).
- `nabu snapshot` — bakes `pb_data`/`pb_public` into **`nabu-deploy`** (the binary
  you ship) and moves the archive to `/tmp`. Both binaries are git-ignored; rebuild
  them, don't commit them.
- *Getting started* help lesson at `/nabu/help` — run the demo locally.
- [CLAUDE.md](../../CLAUDE.md) — repo rules and pointers to build, deploy and design notes.
