---
title: Deployment modes
weight: 15
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    The two ways to run a Nabu course: a classroom laptop that works offline, or a
    hosted course on Cloudflare. Which to pick, and what each needs.
---
Nabu runs a course in one of two ways. Both use the same release, so a course
can start on a laptop and move online later. Every deployment page in this help
says which mode it is about.

| | Classroom | Hosted (SaaS) |
|---|---|---|
| **For** | a teacher running a live lesson in a room | a course online for many groups |
| **Runs on** | the teacher's laptop | Cloudflare: static pages, plus one container per course |
| **Network** | the room's Wi-Fi; no internet needed | the internet |
| **Students sign in** | by typing their name (fast login) | with an account |
| **Agents** | a worker on the laptop or LAN, or authored sample output | a worker on https with a token |
| **Data** | `pb_data/` on the laptop | the course container, copied to object storage |
| **Start with** | `nabu classroom` | `clog build` in CI |
| **Check with** | `nabu doctor --mode classroom` | `nabu doctor --mode saas` |

**Classroom** is the quickest start: one file, one command, no accounts to make.
See the [classroom page][laptop].

**Hosted** keeps a course available between lessons. CI builds the static site
with `clog build`, and Cloudflare serves it. The live parts (sign-in, groups,
agent jobs) run in a container that sleeps when nobody is using it. Setting it
up starts with [your Cloudflare account][cloudflare] and
[first deployment][first].

A third, smaller option is to **self-host** the same container with podman or
docker on your own server. `nabu doctor --mode selfhost` checks it. The
container image is not published yet.

## Which mode am I checking?

`nabu doctor` with no `--mode` runs the checks every mode shares: the settings,
the database folder, the web app, the content and its secrets. Add `--mode` for
the rest:

```bash
nabu doctor --mode classroom   # web app built in, a LAN address, agents that work offline
nabu doctor --mode saas        # static build, Cloudflare token, media store, worker on https
nabu doctor --mode selfhost    # container runtime and image
```

It prints nothing when the machine is ready. Add `-v` to see each passing check.

[laptop]: help:reference#laptop-deploy
[cloudflare]: help:reference#cloudflare-account
[first]: help:reference#first-deploy
