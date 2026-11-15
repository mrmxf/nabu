---
title: Classroom on a laptop
weight: 18
params:
  readiness: production
  kind: presentation
  audience: admin
  summary: |
    Run a live lesson from a teacher's laptop with nabu classroom: students on
    the same Wi-Fi type their name and join. No cloud and no internet, with the
    security trade-offs that come with it. Linux, macOS and Windows.
---
Mode: classroom. See [deployment modes][modes] for the alternative, a hosted
course.

The teacher's laptop runs Nabu, and students connect from their own devices over
the room's Wi-Fi. It needs no cloud account, no domain name and no internet.

## Security: read this first

`nabu classroom` opens Nabu to **every device on the network**, and lets anyone
who can reach it sign in by typing a name. That is fine in a room on a network
you trust, and risky anywhere else.

- **Trusted Wi-Fi only.** Use the school or venue network, or a hotspot from your
  own phone. **Never** use café, airport or hotel Wi-Fi.
- **Never expose it to the internet.** No router port-forwarding and no tunnels.
- **It is plain http.** Anything typed crosses the Wi-Fi unencrypted. Students
  sign in with a name only, so there are no passwords to steal, but do not ask
  them for anything private.
- **Anyone in the room can type a classmate's name.** Fast login trades that
  risk for speed. It works only while you run `nabu classroom`, only for
  students, and only for a Session that is open.
- **Keep secrets off the laptop.** Do not copy storage keys or worker tokens to a
  machine you carry around.
- **Student data lives in `pb_data/` on the laptop.** Handle it as your school's
  policy says, and delete it when the course is over.
- **Allow the firewall prompt for private networks only.**

## Before the lesson

You need the Nabu release on the laptop (see the [quickstart][install]) and the
course folder. On **Windows**, run Nabu natively in PowerShell, not inside WSL:
WSL sits behind its own network and students cannot reach it.

Check the laptop:

```bash
nabu doctor --mode classroom
```

It prints nothing when the laptop is ready. Otherwise each line says what to
fix. It checks that:

- the web app is built into the binary, so nothing needs downloading;
- the laptop has an address on the room's network;
- every agent job can run with no worker, or a worker answers.

**Agents offline.** With no internet there is usually no agent worker. An agent
job written with `sim_output_fallback: true` then shows the slide's authored
`sim_output` instead of an error. See [agent fundamentals][agents]. To run real
agents, start a worker on the laptop or the LAN and set `WORKER_HEALTH_URL`.

**Sessions.** Students join a **Session**. `content/lesson-schedules.yaml` lists
them; `nabu init` writes a practice Session with no dates, which is always open.
For a real lesson, give each Session the window it is open for:

```yaml
lesson-schedules:
  - lesson: first-lesson
    sessions:
      - name: Tuesday morning
        enroll: { from: "2026-12-01 09:00:00Z", to: "2026-12-01 12:00:00Z" }
```

## Run the lesson

```bash
nabu classroom --export results.yaml
```

In one step it:

1. loads the course into the database (`--no-seed` skips this);
2. switches on name-only sign-in (**fast login**) for this run only;
3. warns about anything that will not work offline;
4. serves on every network interface, on port 8090 (`--port` changes it);
5. prints the address for students and a QR code to scan.

The first time, your laptop asks whether to allow incoming connections:

- **macOS:** choose *Allow*. Later: System Settings, Network, Firewall.
- **Windows:** tick **Private networks** only, then *Allow*.
- **Linux:** most desktops have no firewall on; if yours does, allow port 8090.

Students join the same Wi-Fi, open the address (or scan the code), type their
name and pick the Session. A student who types the same name again in the same
Session gets their own work back.

You can follow along at `http://127.0.0.1:8090` on the laptop itself.

## After the lesson

Press **Ctrl-C**. Nabu switches fast login off again and writes every student,
and where each one got to, into `results.yaml` (or `.json`, or `.csv`). Without
`--export` it prints the command to do it later:

```bash
nabu users export --role student --positions --out results.yaml
```

Then keep `pb_data/` for the next lesson, or delete it as your data policy says.

## When it goes wrong

See [troubleshooting][trouble]. The most common problem is students who cannot
reach the laptop: check they are on the same Wi-Fi, that you allowed the firewall
prompt, and that the network does not keep devices apart. Many guest networks
do; a phone hotspot avoids that.

[modes]: help:reference#deployment-modes
[install]: help:getting-started#install
[agents]: help:reference#agent-fundamentals
[trouble]: help:reference#troubleshooting
