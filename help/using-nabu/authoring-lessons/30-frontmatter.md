---
title: Slide frontmatter
params:
  readiness: production
  kind: presentation
---
Frontmatter is the `---`-fenced block at the top. Hugo's standard fields
(`title`, `slug`, `weight`) stay top-level; everything else goes under `params:`
so it is reachable as `.Params.*`.

```yaml
---
title: The Human Newsroom
params:
  readiness: production           # DRAFT | production | deprecated | a go-live timestamp
  kind: human                     # intro | human | presentation | agent | discussion | reflection
  timers:
    - { label: Task, seconds: 900 }
  t_choice:                       # multiple-choice groups; place each in the body
    - id: human-topic             #   with {{< LMS t="choice" id="human-topic" >}}
      legend: "Pick your topic"   #   (same id across slides shares the pick)
      style: radio                #   radio (default) | dropdown (re-show a pick)
      options:
        - { key: A, label: "First option" }
        - { key: B, label: "Second option" }
  t_quiz:                         # self-check questions; place each in the body
    - id: m1-basics               #   with {{< LMS t="quiz" id="m1-basics" >}}
      scope: student              #   student (default) | group
      reveal: on-answer           #   on-answer (default) | on-request | never
      questions:
        - id: q1
          type: choice            #   choice | multi | truefalse | short
          stem: What is a workflow?
          options:
            - { key: a, label: "Work split into dependent steps", correct: true }
            - { key: b, label: "A pump that isn't broken" }
          rationale: "Steps, with dependencies between them."
  notes_label: "Your group's notes"   # optional: show a notes box
  agent_key: hunter               # for kind=agent
  sim_output: |                   # for kind=agent: the canned result (Markdown)
    **hunter found 5 results…**
---
The slide body, in **Markdown**. Tables, lists, images and blockquotes all work.
```

A course or lesson `_index.md` only needs `title`, optional `weight`, and
`params.summary` / `params.readiness`. A course may also set
`params.guest_access: true` to let signed-out guests view it (this help course
does exactly that).

## Course and lesson extras

These only make sense on an `_index.md`:

```yaml
params:
  logo: my-logo.svg     # navbar logo while this course/lesson is on screen
  css:  _course.css      # stylesheet for this course (or _lesson.css on a lesson)
  teacher_pages:        # lesson only — staff-only companion pages
    briefing: teacher/00-briefing.md
```

`css` names a stylesheet in the same folder. The course's loads first, then the
lesson's, so a lesson can override its course — see *Styling a course*. Both are
control files (the `_` prefix), so students never see them listed.

## Audience (persona)

Tag who a lesson or slide is for with `params.audience`, one of
`student | teacher | admin | all`. It's **hierarchical**: set it on the lesson
`_index.md` as the default, and a slide inherits it unless it sets its own. An
empty audience means "inherit"; a lesson with none defaults to `all`.

```yaml
params:
  audience: teacher     # this lesson is for teachers…
```
```yaml
params:
  audience: all         # …but this one slide is for everyone
```

The help index shows a persona **filter** and each lesson a coloured **badge**
from its audience. An unknown value (a typo like `teachers`) is flagged on the
slide's content status in `/_/`.

## Cross-references between pages (`help:` links)

Link to another help page with a **`help:` link**, not a relative `.md` path
(relative paths 404 in the rendered viewer). It resolves by slug, so it survives
files being moved or reordered:

```markdown
See [troubleshooting](help:reference#troubleshooting-agents) for the fixes.
Read the [frontmatter guide](help:authoring-lessons#frontmatter) first.
```

- `help:<lesson>` — a lesson in the **current course** (opens on its first slide).
- `help:<lesson>#<slide-slug>` — a specific **slide** (the `#` is the slide's slug,
  i.e. its filename without the `NN-` prefix).
- `help:<course>/<lesson>#<slide>` — an explicit course.

Every page's address bar is already a shareable **deep link**
(`…/<lesson>#<slide-slug>`), and the Share bar copies it. A `help:` link that
points at a lesson/slide that doesn't exist is flagged as a WARNING at ingest, so
broken cross-references show up in `/_/` instead of silently failing. Links to
source files or `docs/` (outside the help course) aren't `help:` links — write them
as inline code, e.g. `docs/secrets.md`.
