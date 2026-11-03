---
title: Admin cold start — validate, seed, roster, backup/restore
weight: 19
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    The first-time admin sequence to take a freshly built box to a running
    instance with accounts and content: validate the box is ready, seed, install
    the roster, then back up and restore onto the target. Pairs with the
    agent deploy runbook, which proves the running system.
---
This is the **admin data-lifecycle** runbook: the exact order to bring a box from
a clean build to a running instance with accounts and content. It pairs with
[agent deploy](help:reference#agent-deploy) — that runbook
proves the *running* system (health, agents); this one gets you *to* a running
system. Every step that needs secrets is wrapped with Infisical so nothing touches
disk.

All admin commands share one contract: **verbose by default**, `-q` prints only
failures, **exit 0 on success/warnings and 1 on failure**, and **secrets are never
printed** (env vars show as set/MISSING, never their values).

## 1 · Build the binary

Assumed starting point: a clean clone builds successfully.

```bash
./nabu/scripts/build.sh                    # SPA -> pb_public, then the lean ./nabu/nabu
```

This box seeds its own content below (step 4), so there is nothing to `install`.
`install` is only for the shipped-blob path — extracting a pre-baked `nabu-deploy`
(from `nabu snapshot`) onto a target host (see [docs/user/deploy.md](../../../docs/user/deploy.md)).

## 2 · Validate the box is ready

### 2a · Give the host some swap

Small cloud instances routinely ship with **no swap at all**. On a 2 GB box running
nabu, Caddy and PocketBase that is a real hazard: with nothing to reclaim to, a
memory spike doesn't slow down first — the kernel's OOM killer simply picks a
process and kills it, with no warning and nothing in nabu's own logs to explain the
disappearance.

Check what you have:

```bash
free -h                                    # "Swap: 0B 0B 0B" => none configured
swapon --show                              # prints nothing at all => none configured
```

If there is none, a 2 GB swapfile is enough. It is a shock absorber, not extra RAM
— the point is to convert a hard kill into a slowdown you can see coming:

```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile                   # readable by root only; it holds process memory
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab   # survives reboot
sudo sysctl -w vm.swappiness=10            # prefer RAM; swap is the emergency reserve
echo 'vm.swappiness=10' | sudo tee /etc/sysctl.d/99-nabu-swap.conf
```

**Prove it:**

```bash
free -h                                    # Swap total now non-zero
swapon --show                              # /swapfile listed
```

Two things to know before you do this:

- **The swapfile competes with `pb_data` for disk.** It lives on the same
  filesystem, so 2 GB of swap is 2 GB that lesson data and backups can no longer
  use. Check `df -h /` first, and size the swapfile down on a small volume.
- **Swap being *used* is normal; swap being *paged in* continuously is not.** A
  steady `si` column in `vmstat 1` means the box is thrashing and is effectively
  down — that is the signal to add RAM, not more swap.

### 2b · Validate nabu's own configuration

```bash
infisical run --env prod -- ./nabu/nabu admin validate
```

`validate` parses the content tree for things that need secrets (S3 buckets,
agents), then checks the matching env vars, that `pb_data` is writeable and
`pb_public` is built, and that a superuser exists or is configured. Fix every
**FAIL** before continuing. If it reports buckets, apply their CORS policy once
each (so browser uploads work):

```bash
infisical run --env prod -- ./nabu/nabu admin s3 set-cors --bucket <bucket-it-named>
```

Optionally prove S3 write access end-to-end:

```bash
infisical run --env prod -- ./nabu/nabu admin validate --deep
```

## 3 · Apply PocketBase settings from config

Push the `pocketbase.*` block of `nabu-config.yaml` (app name/URL, SMTP, and the
backup schedule + S3 target) into PocketBase's own settings:

```bash
infisical run --env prod -- ./nabu/nabu admin settings apply --dry-run   # preview first
infisical run --env prod -- ./nabu/nabu admin settings apply             # then apply (prompts)
```

> ⚠️ **The shipped `pocketbase:` block is a placeholder scaffold.** `settings
> apply` is *declarative* — it makes settings match the config exactly, so
> applying the unedited scaffold would overwrite real SMTP/backup settings with
> placeholders (and disable them). **Always `--dry-run` first**, and edit
> `nabu/embedfs/nabu-config.yaml` to your real values before applying in prod.
> SMTP and S3-backup passwords come from their `password_env` vars (never the
> file). This step does **not** create the superuser — that's step 4.

## 4 · Seed the superuser and content

Wrap `seed` with Infisical so the superuser password (`NABU_SUPERUSER_PW`) is
present — it is never a committed literal:

```bash
infisical run --env prod -- ./nabu/nabu seed   # idempotent: content, demo accounts, + superuser (admin@nabu.example)
```

The demo accounts (students/teacher/app-admin) only seed while `demo_mode` is on
(`nabu-config.yaml`); turn it off for a real cohort. If `NABU_SUPERUSER_PW` is
unset, no superuser is created — set one up from the `/_/` first-run page. There is
no `nabu superuser` command; the only other way to add one is a `role: superuser`
roster entry in step 5.

## 5 · Install the roster

```bash
./nabu/nabu users import --from roster.yaml --mode merge
```

## 6 · Back up here, restore on the target

Capture accounts + state (this bundle carries password hashes — keep it safe):

```bash
./nabu/nabu admin backup --out backup.yaml
```

Move `backup.yaml` to the target instance, then restore it there:

```bash
./nabu/nabu admin restore --from backup.yaml
```

## Done → prove it runs

The instance now has its accounts and content. Serve it and walk the runtime
gates in [agent deploy](help:reference#agent-deploy):

```bash
infisical run --env prod -- ./nabu/nabu serve --dir /data/pb_data
```
