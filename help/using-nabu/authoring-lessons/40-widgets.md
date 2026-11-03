---
title: Inline widgets (LMS shortcodes)
params:
  readiness: production
  kind: presentation

  t_choice:
    - id: demo_topic                  # referenced by the shortcode; also the saved-answer key
      legend: "Pick your topic"  # optional heading above the options
      style: radio               # optional: radio (default) | dropdown
      scope: group
      default_value_key: "slug"
      options:
        - { key: A, label: "First option",  slug: "1st" }
        - { key: B, label: "Second option", slug: "2nd" }
---
Drop a widget into the body with a shortcode on its own line:

```
{{< LMS t="timer" duration="15:00" start="onshow" >}}
```

Available widget types (`t`) and their arguments are defined in
`nabu/internal/content/nabu-widget-catalogue.yaml`:

- `timer` — `duration` (mm:ss / hh:mm:ss), `start` (`onshow`|`onclick`), `label`
- `figure` — `src`, `alt`, `caption`
- `note` — `tone` (`info`|`tip`|`warning`), `text`
- `choice` — `id` (see below)
- `quiz` — `id` (see below)
- `include` — `from`, `only` (see *Shared references*)
- `nabu-info` — `verbose` (`true`|`false`, default false) — the running Nabu version

Plus the **layout widgets** below: `callout`, `postit`, `define`, `steps`,
`step`, `grid`, `col`, `proscons`, `vocab`, `toolcard`, `banner`, `itemtype`,
`video`.

Unknown types or missing required args don't break the build; they are recorded
against the slide's `content` status so you can spot them in `/_/`.

## Layout widgets

These wrap or decorate content. They are plain HTML and CSS — nothing runs in the
browser — so they are cheap to use and safe to nest.

**Paired widgets wrap the content after them**, up to a matching
`{{< /LMS >}}`. Always leave a **blank line** around the wrapped content,
otherwise it renders as literal text instead of Markdown:

```
{{< LMS t="define" tone="warn" header="Watch out" >}}

A **brittle** workflow is one that only works when nothing goes wrong.

{{< /LMS >}}
```

{{< LMS t="define" tone="warn" header="Watch out" >}}

A **brittle** workflow is one that only works when nothing goes wrong.

{{< /LMS >}}

### Tones, not colours

Layout widgets take a `tone` — what the box **means** — and the course stylesheet
decides what that looks like:

`define` · `ok` · `warn` · `info` · `attention` · `step` · `neutral`

Write the meaning, never the colour. Re-theming a whole course is then one edit
to its `_course.css` palette instead of a hunt through every slide. See
*Styling a course*.

### The boxes

`define` — the workhorse. A definition, key idea or worked example.
Params: `tone`, `header`.

`callout` — same idea, with more styles: `notepad`, `prompt` and `cli-output`
(for showing AI prompts and terminal output), plus the semantic
`intro`, `note`, `tip`, `tips`, `warning`, `discuss`.
Params: `style`, `header`.

`postit` — a draggable, minimisable sticky note. Params: `color`, `subtype`
(`list`|`code`), `header`.

`vocab` — one glossary term. Params: `term`, `pos` (part of speech), `example`;
the wrapped body is the definition.

```
{{< LMS t="vocab" term="Node" pos="Noun" example="A transcoding node converts video before passing it on." >}}

A discrete unit of work inside a workflow.

{{< /LMS >}}
```

{{< LMS t="vocab" term="Node" pos="Noun" example="A transcoding node converts video before passing it on." >}}

A discrete unit of work inside a workflow.

{{< /LMS >}}

`toolcard` — one entry in a directory of tools. Params: `name`, `url`, `cost`,
`focus`. Only `http`/`https` links are made clickable.

### Procedures

`steps` wraps a sequence of `step` cards and numbers them **in CSS** — so
inserting a step in the middle renumbers the rest and you change nothing. Use
`start` to continue a procedure across slides.

```
{{< LMS t="steps" >}}

{{< LMS t="step" title="Open the editor" >}}

Sign in first.

{{< /LMS >}}

{{< LMS t="step" title="Make a flow" >}}

Drag a trigger onto the canvas.

{{< /LMS >}}

{{< /LMS >}}
```

{{< LMS t="steps" >}}

{{< LMS t="step" title="Open the editor" >}}

Sign in first.

{{< /LMS >}}

{{< LMS t="step" title="Make a flow" >}}

Drag a trigger onto the canvas.

{{< /LMS >}}

{{< /LMS >}}

### Columns

`grid` lays `col` children side by side and **stacks them on a narrow screen by
itself** — you never write a breakpoint. Params: `cols` (1–6), `min` (the width
at which a column wraps, e.g. `18rem`).

`proscons` is a two-column preset; put two `col`s inside it.

```
{{< LMS t="proscons" >}}

{{< LMS t="col" title="Pros" tone="ok" >}}

- Fast to build

{{< /LMS >}}

{{< LMS t="col" title="Cons" tone="attention" >}}

- Breaks silently

{{< /LMS >}}

{{< /LMS >}}
```

{{< LMS t="proscons" >}}

{{< LMS t="col" title="Pros" tone="ok" >}}

- Fast to build

{{< /LMS >}}

{{< LMS t="col" title="Cons" tone="attention" >}}

- Breaks silently

{{< /LMS >}}

{{< /LMS >}}

### Odds and ends

`banner` — a section divider across a long deck. Params: `text`, `from`, `to`
(tones for the gradient). Self-closing.

`itemtype` — a small badge naming what a slide is: `assignment`, `discussion`,
`quiz`, `page`.

`video` — a responsive YouTube/Vimeo embed. `src` takes a watch, share or embed
URL, or a bare id. Nabu **rebuilds** the embed URL from the video id and serves
YouTube from `youtube-nocookie.com`, so a lesson can't embed arbitrary third-party
frames or set tracking cookies on students. Anything it doesn't recognise renders
nothing.

```
{{< LMS t="video" src="https://youtu.be/E0hrcDO3Noc" title="Workflow tools" >}}
```

## Self-check quizzes

A `quiz` widget places a set of questions at that point in the body; the
questions live in frontmatter under `t_quiz` and are referenced by `id` — the
same split as `choice`.

```yaml
---
params:
  t_quiz:
    - id: m1-basics
      legend: "Check your understanding"
      scope: student            # student (default) | group
      reveal: on-answer         # on-answer (default) | on-request | never
      questions:
        - id: q1
          type: choice          # choice | multi | truefalse | short
          stem: What is a workflow?
          options:
            - { key: a, label: "Work split into dependent steps", correct: true }
            - { key: b, label: "A plumbing pump that isn't broken" }
          rationale: >
            A workflow breaks work into steps with dependencies between them.
---
{{< LMS t="quiz" id="m1-basics" >}}
```

- `truefalse` writes itself — give the question `answer: "true"` or `"false"` and
  Nabu supplies both options.
- `short` is free text. It is never auto-marked; on reveal the student sees your
  `answer` alongside what they wrote.
- `rationale` is the point of the exercise. A student who guessed still learns
  something.

**Quizzes are formative.** Nothing is stored, scored or reported: a student
answers, finds out, and moves on. Put answer keys and teaching notes on a
`teacher/` page rather than relying on `reveal: never`.

Authoring mistakes are caught at seed. Anything that makes a quiz
*unanswerable* — no questions, a choice with no options, a duplicate id — is an
error; anything merely questionable, like forgetting to mark an option correct,
is a warning, so a half-written draft never blocks you.

## Multiple choice

A `choice` widget places a set of options at that exact point in the body, but the
options themselves live in frontmatter under `t_choice` and are referenced by
`id`. Keeping the (bulky) option lists in frontmatter keeps multi-file diffs
small; the shortcode controls only where the selector appears.

```yaml
---
params:
  t_choice:
    - id: demo_topic                  # referenced by the shortcode; also the saved-answer key
      legend: "Pick your topic"  # optional heading above the options
      style: radio               # optional: radio (default) | dropdown
      default_value_key: "slug"
      options:
        - { key: A, label: "First option",  slug: "1st" }
        - { key: B, label: "Second option", slug: "2nd" }
---
Pick one below:

{{< LMS t="choice" id="demo_topic" >}}
```

The `id` is also the **storage key**: two slides that use the same `id` share the
student's pick (this replaces the old `choice_key`). A `choice` whose `id` has no
matching `t_choice` group renders nothing and is flagged on the slide's `content`
status.

`style` controls how the options render: `radio` (the default) is the button
selector; `dropdown` is a native `<select>`. Because the pick is shared by `id`,
a later slide can re-declare the same group with `style: dropdown` to **re-show**
what the student picked earlier as a compact dropdown. An unrecognized `style`
falls back to `radio` and is flagged on the slide's `content` status.

## Live widgets

These do something at run time, so they need a session behind them. In the
read-only help viewer they render inert.

`print` — shows a stored value inline in a sentence. `from` names either a
`t_choice` id (printing what the student or group picked) or a built-in:
`username`, `email`, `group`, `session`. `show` (`label`|`slug`|`key`) overrides
which field of a choice is printed.

```
Good luck, {{< LMS t="print" from="username" >}} — your group is
{{< LMS t="print" from="group" >}}.
```

`tabulate` — renders, as a table, the rows a `t_mapper` derived from the
student's choice. `from` is the mapper id. Updates live; renders nothing until a
value exists. See *Choice values* in the reference lessons.

`uploader` — a hand-in control. `scope` (`student`|`group`) decides whose folder
the file lands in; `accept` limits file types; `label` sets the button text.

`browser` — a tabbed file browser over the lesson's resource store. `tabs` is a
space-separated list of `resources`, `group`, `student`; `variant` is `full` (the
managed browser) or `list` (a compact read-only list); `size` (`true`|`false`)
shows or hides the size column.

`agent` — dispatches one of the slide's `t_agent` jobs and streams its progress.
`id` matches the job. `agentdef` shows one field of an agent's profile
(`field`, default `system`) — useful for showing students the prompt an agent is
running. `serverstatus` shows worker health and queue depth; `show` defaults to
`"health,agents,group"`.

## Nabu version info

Show the running Nabu instance's build identity (read live from the `nabu` table).
Terse by default; `verbose="true"` shows the full details:

```
Running {{< LMS t="nabu-info" >}}.
```

Running {{< LMS t="nabu-info" >}}.

```
{{< LMS t="nabu-info" verbose="true" >}}
```

{{< LMS t="nabu-info" verbose="true" >}}
