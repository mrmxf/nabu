---
title: Install the Cloudflare CLI (admin)
weight: 22
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    Install wrangler, Cloudflare's command-line tool, on Linux, macOS or Windows,
    and point it at your deploy token from Infisical rather than a personal login.
---
This page is about the **SaaS on Cloudflare** deployment mode. A classroom laptop
does not need wrangler.

**wrangler** is Cloudflare's command-line tool. nabu's deploy steps use it to upload
the static site, deploy Workers and containers, and manage D1 and R2.

You will: _install_ Node → _install_ wrangler → _connect_ it to your token → _check_.

## 1. Install Node

wrangler is a Node program. nabu pins the Node version in its `.nvmrc` file
(currently `26.13.0`).

- With clog, from inside the repo: `clog Install node` reads `.nvmrc` and installs
  exactly that version.
- Without clog: install [nvm][nvm] (or [nvm-windows][nvmw]), then `nvm install` inside
  the repo.

Check with `node --version`.

## 2. Install wrangler

Pick one:

| Way | Command | When |
|---|---|---|
| clog | `clog Install wrangler` | the usual way; checks it worked afterwards |
| npm | `npm install --global wrangler` | no clog. On Linux with a system Node, add `sudo`. |
| per run | `npx wrangler@<version> …` | CI, or a repo that pins the version |

Check with `wrangler --version`.

## 3. Connect it to your token

wrangler can log in two ways. **Use the token**, so what you do on your laptop is
what CI does.

- **Token (recommended).** When `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` are
  set, wrangler uses them and nothing else. Infisical sets them for one command:

  ```bash
  infisical run --env dev --path /nabu-core \
    --projectId ee606f47-f67f-494d-9f42-5bbf49d73eb1 --domain https://eu.infisical.com \
    -- wrangler whoami
  ```

  The token and its permissions are described in
  [Cloudflare account, token and secrets][account].

- **Browser login (`wrangler login`).** It uses *your own* Cloudflare user and its
  permissions, which are usually far wider than the deploy token's. Fine for poking
  around; never for deploys. `wrangler logout` removes it.

## 4. Check

`wrangler whoami` (run as above) should name your account. With an account token it
may also say it cannot read your user details: that is expected, because an
account token belongs to the account, not to a person.

Then run the token check from the [account page][account], step 3. When it passes,
wrangler can do everything nabu's deploys need.

[account]: help:reference#cloudflare-account
[nvm]: https://github.com/nvm-sh/nvm
[nvmw]: https://github.com/coreybutler/nvm-windows
