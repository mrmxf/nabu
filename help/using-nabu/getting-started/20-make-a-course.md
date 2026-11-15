---
title: Make a course
params:
  readiness: production
  kind: presentation
---
A course is a folder of Markdown files. `nabu init` makes one with a first
lesson and a first slide:

```bash
nabu init my-course
cd my-course
```

The parts that matter:

| Path | What it is |
|---|---|
| `nabu.yaml` | the course settings, such as its name |
| `content/my-course/first-lesson/010-welcome.md` | the first slide |
| `content/lesson-schedules.yaml` | the Session students join (a practice one to start) |
| `.clog.yaml`, `.github/workflows/course.yaml` | how CI builds the course: you can ignore them for now |

Now check the folder is ready:

```bash
nabu doctor
```

`nabu doctor` prints **nothing** when everything is ready. Otherwise each line
says what is wrong and how to fix it. A new course shows one warning about the
superuser: that is expected, and the next step deals with it.

The course name must be lowercase letters, digits and hyphens. Run all the
following commands from inside the course folder.
