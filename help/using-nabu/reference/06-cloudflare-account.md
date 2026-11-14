---
title: Cloudflare account, token and secrets (admin)
weight: 21
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    The canonical set of secrets a nabu deployment needs, where each one lives in
    Infisical, and exactly which Cloudflare permissions to tick when you create the
    API token and the R2 key. Ends with a script that proves the token works.
---
This page is about the **SaaS on Cloudflare** deployment mode. A classroom laptop
needs none of it.

You will: _create_ an API token → _create_ an R2 key → _store_ both in Infisical →
_prove_ they work with one command.

Before you start you need:

- a Cloudflare account, with your domain (we use `example.com` below) added as a zone
- the Infisical CLI, logged in (`infisical login`)
- wrangler, the Cloudflare CLI: see [Install the Cloudflare CLI][wrangler]

## The canonical secrets

Every nabu deployment uses these names, exactly. Tools read them from the
environment, and Infisical puts them there; nothing secret is ever in a file.

| Name | Secret? | What it is for | Where it comes from |
|---|---|---|---|
| `CLOUDFLARE_ACCOUNT_ID` | no, but kept with the others | which Cloudflare account to deploy into | dashboard → any zone → right-hand column, "Account ID" |
| `CLOUDFLARE_API_TOKEN` | **yes** | deploying: Pages, Workers, containers, D1, DNS | step 1 below |
| `R2_ACCESS_KEY_ID` | **yes** | reading and writing the buckets: media and the course database backup (Litestream) | step 2 below |
| `R2_SECRET_ACCESS_KEY` | **yes** | the other half of the R2 key | step 2 |

They live in **one Infisical folder per repository**. For the nabu repo:

| Setting | Value |
|---|---|
| Infisical instance | EU, `https://eu.infisical.com` |
| Project | `nabu` (id `ee606f47-f67f-494d-9f42-5bbf49d73eb1`) |
| Folder | `/nabu-core` |
| Environments | `dev` and `prod`. Until prod exists they hold the same values. |

A course repo gets its own folder in the same way, and names it in its `.clog.yaml`
`ci.infisical` block.

Secrets the running server needs (the agent worker token, the superuser password)
are listed in [docs/secrets.md][secrets]. The academy's keys arrive with the academy.

**CI holds no secrets at all.** GitHub Actions logs in to Infisical with a machine
identity (OIDC), so nothing is pasted into GitHub. The identity must be added to the
project, and its allowed subjects must include the repository, e.g. `repo:mrmxf/nabu`.

## 1. Create the API token

Make it an **account** token, not a personal one, so deploys don't stop working
when a person leaves.

1. Dashboard → **Manage Account → Account API Tokens → Create Token → Custom token**.
2. Name it after the repo and mode, e.g. `nabu-core-dev`.
3. Add these permissions. Pick the ones **not** marked "Legacy": the dashboard still
   lists old ones such as "Workers Scripts", "Workers Tail" and "Workers CI".

   **Account permissions**

   | Permission | Level | Why |
   |---|---|---|
   | Workers | Editor | deploy the academy and course Workers and their Durable Objects |
   | Workers Containers | Edit | one container per course |
   | Workers R2 Storage | Edit | create and manage the buckets |
   | D1 | Edit | the academy's roster database |
   | Cloudflare Pages | Edit | the static tier that `nabu build` produces |

   **Zone permissions**, for your zone only (Zone Resources → Include → Specific zone)

   | Permission | Level | Why |
   |---|---|---|
   | DNS | Edit | the course host's record (a Workers custom domain creates it) |
   | Workers Routes | Edit | Worker routes on the zone. If the dashboard marks it "Legacy", leave it out and let the check below decide. |

4. Account Resources: include **your account only**.
5. Set an expiry date, e.g. one year, and put a reminder in your calendar.
6. Create it. Copy the token straight into Infisical as `CLOUDFLARE_API_TOKEN`
   (in `dev`, and in `prod` if you use it). It is shown once.

Changing the permissions of an existing token keeps the same token value, so there
is nothing to re-paste. A **new** token has a new value: replace the one in Infisical.

## 2. Create the R2 key

R2 keys are made separately from API tokens, and speak the S3 protocol.

1. Dashboard → **R2 Object Storage → Manage API tokens → Create Account API token**.
2. Permission: **Object Read & Write**.
3. Specify buckets: only the buckets this deployment uses (media and state).
4. Create it. The page shows three values; keep two:
   - **Access Key ID** (32 characters) → `R2_ACCESS_KEY_ID`
   - **Secret Access Key** (64 characters) → `R2_SECRET_ACCESS_KEY`

   Do not copy the "Token value": that is a third thing, not the secret key.

A key limited to particular buckets cannot list *all* buckets, so the check below
asks about each bucket by name.

## 3. Prove it works

From a clone of the nabu repo:

```bash
infisical run --env dev --path /nabu-core \
  --projectId ee606f47-f67f-494d-9f42-5bbf49d73eb1 --domain https://eu.infisical.com \
  -- scripts/cf-token-check.sh example.com my-media-bucket my-state-bucket
```

It only reads, never changes anything, and never prints a secret. Every line should
say `PASS`:

```text
Cloudflare API token
  PASS  token is valid                 active
  PASS  Workers                        academy and course Workers, Durable Objects
  PASS  Workers Containers             one container per course
  ...
All checks passed.
```

A `FAIL` names the permission that is missing. Add it to the token (step 1), wait a
few seconds, and run the check again.

## When something fails

| You see | It means |
|---|---|
| `token is valid … neither account nor user verify accepts it` | the value in Infisical is wrong or the token was deleted |
| `FAIL Workers Containers … Authentication error` (or any other) | that permission is missing from the token |
| `FAIL zone visible` | the token's Zone Resources do not include your zone |
| `FAIL secret length 42, expected 64` (or any odd length) | the value is not the key. Either the wrong value was pasted, or an Infisical reference is broken: references are case-sensitive, so `${DEV.…}` instead of `${dev.…}` expands to an error message, not the secret |
| `FAIL bucket … access denied` | the R2 key is not allowed that bucket, or the name is wrong |

[wrangler]: help:reference#install-wrangler
[secrets]: https://github.com/mrmxf/nabu/blob/main/docs/secrets.md
