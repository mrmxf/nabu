---
title: Teach a room
params:
  readiness: production
  kind: presentation
---
`nabu classroom` turns your laptop into the room's server. Students need no
accounts: they type their name and they are in. It needs no internet, only the
room's Wi-Fi.

First check the laptop:

```bash
nabu doctor --mode classroom
```

Then start the lesson:

```bash
nabu classroom --export results.yaml
```

It prints the address students open, such as `http://192.168.1.42:8090`, and a
QR code they can scan. They join the same Wi-Fi as you, open the address, type
their name and choose the Session.

When the lesson ends, press **Ctrl-C**. Nabu writes the students, and where each
one got to, into `results.yaml`, and switches name-only sign-in off again.

Before you use it with a real class, read the [classroom page][laptop]: it covers
firewall prompts, choosing a safe network and when Sessions open.

[laptop]: help:reference#laptop-deploy
