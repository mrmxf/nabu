---
title: Where next
params:
  readiness: production
  kind: presentation
---
You have a course, a lesson and a way to teach it. From here:

- **Write lessons:** the [authoring guide][authoring], and the
  [Nabu tutorial][tutorial], which is a complete course to copy from.
- **Run lessons:** [rosters][rosters] for real accounts, and the
  [classroom page][laptop] for live lessons.
- **Put a course online:** [deployment modes][modes] explains the choice between
  a classroom laptop and a hosted course, then [first deployment][first].
- **Add agents:** [agent fundamentals][agents].

**When something goes wrong**

1. Run `nabu doctor` (add `--mode classroom` or `--mode saas`). It names each
   problem and its fix.
2. Read [troubleshooting][trouble] for the server, and
   [troubleshooting agents][trouble-agents] for agent jobs.
3. `nabu <command> --help` explains every command and flag.

[authoring]: help:authoring-lessons
[tutorial]: https://github.com/mrmxf/nabu-tutorial
[rosters]: help:for-teachers#rosters
[laptop]: help:reference#laptop-deploy
[modes]: help:reference#deployment-modes
[first]: help:reference#first-deploy
[agents]: help:reference#agent-fundamentals
[trouble]: help:reference#troubleshooting
[trouble-agents]: help:reference#troubleshooting-agents
