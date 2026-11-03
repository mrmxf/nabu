---
title: First deployment (admin)
weight: 17
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    Stand up nabu from a local clone with your own content and rosters, wire up
    peasoup S3 for lesson resources, and deploy behind Caddy on a Scaleway ARM
    instance. Then validate with the bring-up runbook.
---
This is an **admin**'s  **first-deploy** walk through.

You will: _clone_ nabu → _seed _content & rosters → _config_ object storage → _config_ the nabu app → _host_ in the cloud.

Assumptions:

- you've created your own course content (probably using AI in another repo)
- your courses are in `/path/to/courses/content`
- your rosters are in `/path/to/rosters/`
- you are comfortable using the command line (`zsh` on a Mac, `bash` in WSL on Windows, `bash `on linux)
- you are using a free [infisical] account to securely store and update your secrets

## 0 · Prerequisites

- Go 1.26+, Node 20+, and git on the build machine
- A clone of the repo [github.com/mrmxf/nabu][public-repo]
- Your **content tree** in Hugo layout: `/path/to/courses/content/<course>/<lesson>/<slide>.md`
  (same shape as nabu's demo repo's `content/`).
- Your **[rosters]** as `/path/to/rosters/*.yaml` (the `users:` envelope — see the
  roster format under `docs/rosters/`).
- `/usr/local/bin` is on your PATH
- **important** this documentation assumes nabu is on your path. It's only for convenience - you can run it from wherever you want.

## 1 · Build

You don't need this step if you have been given a link to download the binary. Ensure you have the prerequisites done before you start

```bash
# build the Svelte SPA → ./pb_public, then the Go binary
./nabu/scripts/build.sh
# put the nabu executable on your path
sudo mv ./nabu/nabu /usr/local/bin/nabu
```

## 2 · Seed your content

The `seed` command parses all your [content] an populates the database. By default, it will look for a folder called `content` in the folder where you run the `nabu` command. To use a different folder, use the `--content` option:

```bash
nabu seed --content /path/to/courses/content
```

Idempotent — you can run it many times and you'll always get the same result. This means it's safe re-run after every content edit. Run under Infisical, it creates the PocketBase **superuser**
(`admin@nabu.example`, password from `NABU_SUPERUSER_PW`) and — while `demo_mode` is
on — the **demo accounts** for the showcase. On a real deployment: turn `demo_mode`
off (and prune any demo students already seeded via `/_/`, or a `--mode delete-orphans`
roster import scoped to a course); the superuser password is your Infisical secret,
so rotate it there if it was ever shared. Without `NABU_SUPERUSER_PW` no superuser
is created — set one up from the `/_/` first-run page.

## 3 · Import your rosters

You can have one or may roster files - up to you how they're organised. The `import` command takes one file, so you may have to loop a folder of files.

The `merge` option upserts by `uid` without touching other users:

```bash
for f in /path/to/rosters/*.yaml; do
  ./nabu/nabu users import --from "$f" --mode merge
done
```

other import options:

- `--mode overwrite` replaces fields
- `--mode delete-orphans --course <slug>` - unenrols anyone missing from the file.

You can set real passwords in the roster (`password:` is import-only) or from the browser as an admin using the path `/_/`.

## 4 · Run it

Test locally to be sure everything has imported correctly

```bash
# start nabu in the folder defaults:
#   courses on: http://127.0.0.1:8090
#     admin on: http://127.0.0.1:8090/_/
#  the folder where you ran nabu contains pb_data & pb_public
# the ampersand runs in the background, so you can close the
# terminal window and it will still run
nabu serve &
# use nabu stop # to cleanly stop the server when you want
```

You can also explicitly specify how you want nabu to run.

```bash
# start nabu in a custom folder 
# use dev mode to see all the logging on the CLI
# note that ONLY pb_data dir is specified.
# pb_public MUST ALWAYS be a sibling of pb_data
nabu serve \
   --dev \
   --http 127.0.0.1:9876 \
   --dir ./pb_data
```

## 5 · custom S3 provider (lesson resources)

Lesson resources (documents, video, backups, user uploads) lives in an S3-compatible bucket. `nabu`
holds **one shared credential set** (using the ENV) and mediates all access to the bucket— see
`docs/secrets.md` for the full model and the teacher flow
([uploading resources](help:reference#uploading-content)).

An s3 provider has a bucket URL of the form
`https://s3.eu-west-1.peasoup.cloud/MyBucket-123456`. nabu only ever needs the
**host** — it adds the bucket (and the lesson prefix) itself when it builds keys
and public read URLs, so **do not** put the bucket name in either variable:

* `S3_ENDPOINT`  is the host, `https://s3.eu-west-1.peasoup.cloud`
- `S3_PUBLIC_BASE` is the host used for **public read URLs** — normally the same
  as `S3_ENDPOINT`, so you can leave it unset. Only set it when reads should go
  through a different host (a CDN or vanity domain). Either way it is the host
  **without** the bucket; nabu appends `/{bucket}/{prefix}/…`. Putting the bucket
  here doubles it in every URL (`…/MyBucket-123456/MyBucket-123456/…`).

The course and lesson configs then add prefixes inside the bucket to ensure that
different sessions of the same course don't stomp on each other's data. Standard
S3 tools (like scooter software's beyond compare) are great for tidying, syncing and backing up buckets.

We recommend you create a free 🔐[infisical] account and manage all the credentials with that tool. It's much easier and safer than copying them from machine to machine

```bash
S3_ENDPOINT=https://s3.eu-west-1.peasoup.cloud   # host url only
S3_REGION=eu-west-1                              # some clients ignores region
S3_ACCESS_KEY=…                                  # 🔒
S3_SECRET_KEY=…                                  # 🔒
S3_PUBLIC_BASE=…                                 # host only, no bucket; unset = same as S3_ENDPOINT
```

Per course: create a bucket and give it a **public-read** policy. For **CORS**
(so browsers may upload directly via presigned links), nabu ships the policy and
applies it for you — run this once per bucket, with secrets injected by Infisical:

```bash
infisical run --env prod -- nabu admin s3 set-cors --bucket MyBucket-123456
# preview first without applying:
infisical run --env prod -- nabu admin s3 set-cors --bucket MyBucket-123456 --dry-run
```

The allowed origins live in `nabu/embedfs/s3-cors-config.json` (pointed to by
`admin.s3.cors-configfile` in `nabu/embedfs/nabu-config.yaml`); edit that list to
add your site origins. Then name the bucket in the lesson's `_index.md`
frontmatter. This is the whole shape a first-time admin needs:

```yaml
# File: /path/to/courses/ClassName/LessonName/_index.md
---
title: "Lesson Title for my generic lesson"
weight: 1
params:
  readiness: production               # production makes the lesson visible to all
  bucket: MyBucket-123456             # the peasoup bucket bucket (the host URL's last path segment)
  prefix: CommonPrefixForThisLesson   # optional key prefix to give lessons different root folders
  summary: |
    Build, orchestrate, and reason about AI agents.
---
```

Teacher uploads then land at `{S3_PUBLIC_BASE}/{bucket}/{prefix}/resources/<file>` —
e.g. `https://s3.eu-west-1.peasoup.cloud/MyBucket-123456/rise-summer/secret-agents/resources/brief.pdf`.

### Running nabu with secrets from Infisical (Linux)

Rather than exporting secrets by hand, wrap the run command with `infisical run` —
it fetches them and injects them as ENV for that one process, so nothing touches
disk:

```bash
# one-time on the host (or use a machine-identity token for headless / systemd):
infisical login           # follow the instructions
infisical init            # choose your project for serving secrets
cd /opt/nabu              # change folder to the scp target

# inject secrets (infisical "production" group) and start nabu:
infisical run --env prod -- \
  ./nabu serve --http 127.0.0.1:8090 --dir ./pb_data
```

## 6 Validate

[agent deploy](help:reference#agent-deploy); for the secrets
model see `docs/secrets.md`.

## 7 Setup and SSH connection

The easiest way to do this is via an ssh config:

```ini
# /home/username/.ssh/config
Host lms
    HostName       my-uid.pub.instances.scw.cloud
    User           root
    IdentityFile   /home/username/.ssh/key-for-instance.pub
    IdentitiesOnly yes
```

This tells the command line how to connect to host `lms`

```sh
#run this command on the LOCAL machine
# make a file called nabu-rc on the cloud host
scp ./nabu/nabu  lms:/opt/nabu/nabu-rc
```

Now you can setyp

```sh
cd /opt/nabu
# stop the service, copy & restart the service
systemctl stop nabu
cp nabu-rc nabu
systemctl start nabu

```

## 7 · Deploy on Scaleway (ARM) behind Caddy

Running in the cloud is designed to be cheap:

- nabu is pure Go and targets a cheap Scaleway **ARM64** dev instance
- **Caddy** gives automatic TLS for secure https browsing
- S3 is cheap storage and can be on any provider

**Build for arm64** (SPA first), or build on the instance:

```bash
(cd nabu/web && npm ci && npm run build)                 # -> ./pb_public
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o nabu/nabu ./nabu
```

Ship `nabu` + `./pb_public` (+ your `/path/to/courses` and `/path/to/rosters`)
to the instance, e.g. `/opt/nabu`.


To run it as a service, with caddy as a front end, with secrets injected from Infisical require a shell script wrapper to handle
logging in to get a token to access secrets.

Assumptions:

- your cloud instance supports systemd to manage services
- you have an EU infisical free account (US accounts use the US domain)
- executable & pb_data & scripts all in `/opt/nab`

```sh

#!/bin/bash
# start-systemd-nabu.sh — bridges systemd (which is fussy) and Infisical's
# two-step auth (login to get a token, then run with that token).
# Client ID/secret stored in /opt/nabu/nabu.env (see nabu.service).
set -euo pipefail

# note that this script uses the EU domain for infisical (GDPR)
INFISICAL_TOKEN=$(infisical login \
  --domain="https://eu.infisical.com"  \
  --method=universal-auth \
  --client-id="$INFISICAL_CLIENT_ID" \
  --client-secret="$INFISICAL_CLIENT_SECRET" \
  --silent --plain)
# now that we have the token - export it so the next command works
export INFISICAL_TOKEN

# exec replaces this script's process with nabu, so systemd tracks
# the real nabu service PID (not the wrapper) for signals/Restart.
# the project ID must be in the ENV file
exec infisical run \
     --domain="https://eu.infisical.com" \
     --projectId="$INFISICAL_PROJECT_ID" \
     --env prod \
     -- /opt/nabu/nabu serve --http 127.0.0.1:8090 --dir /opt/nabu/pb_data
```

For systemd to auto-start and manage nabu during reboot / crashes
and other trauma that occur on the web, we need a systemd unit file.
This one is battle tested and needs the script above to keep the
mental-load of admin to a minimum.

```ini
# /etc/systemd/system/nabu.service
[Unit]
Description=Nabu LMS
After=network.target

[Service]
# a simple service
Type=simple
# run from the nabu directory to resolve pb_public folder
WorkingDirectory=/opt/nabu
# infisical needs Universal Auth client ID/secret to log in non-interactively
# (systemd has no $HOME, so infisical's normal cached-session login won't work)
EnvironmentFile=/opt/nabu/nabu.env
# wrapper exchanges the client id/secret for a token, then execs nabu
ExecStart=/opt/nabu/start-systemd-nabu.sh
# resurrect nabu if something bad happens
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

We then need the magic login ENV so that Infisical can get at your secrets

```sh
# /opt/nabu/nabu.env
# get these values from your infisical dashboard
INFISICAL_CLIENT_ID="org+access-control+machine-identities+machine+id"
INFISICAL_CLIENT_SECRET="machine+add-client-secret-value"
INFISICAL_PROJECT_ID="project-with-your-secrets-id"
```

Caddy terminates TLS and reverse-proxies to nabu:

```caddyfile
# /etc/caddy/Caddyfile
nabu.example.org {
    reverse_proxy 127.0.0.1:8090
}
```

`caddy reload` and Caddy fetches a certificate automatically. The agent **worker**
stays off the public internet — reach it over the tailnet (`WORKER_HEALTH_URL`),
never a public port.

## 7 · Validate & secure

- Run the [agent deploy runbook](help:reference#agent-deploy)
  end to end (nabu → seed → health local → health remote → agents).
- **Security checklist:** superuser password changed; roster passwords set; all
  secrets in Infisical (never git); Caddy TLS live; worker tailnet-only; S3 keys
  only in nabu's ENV. Details in `docs/secrets.md`.

[content]:     ../authoring-lessons/00-intro.md
[infisical]:   https://eu.infisical.com
[rosters]:     ../for-teachers/20-rosters.md
[public-repo]: https://github.com/mrmxf/nabu