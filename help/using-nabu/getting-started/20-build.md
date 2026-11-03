---
title: Get the code and build
params:
  readiness: production
  kind: presentation
---
Clone the repository:

```bash
git clone https://gitlab.com/mrmxf/secret-agents.git
cd secret-agents
```

One script builds the web UI into Nabu and compiles the Go binary:

```bash
./nabu/scripts/build.sh
```

This runs `npm install` + `npm run build` (SvelteKit → `pb_public`) and then
`go build`. The first run downloads dependencies, so give it a minute.
