---
title: The folder tree
params:
  readiness: production
  kind: presentation
---
Author lessons under the content tree (`content/` by default). Nabu's own help
lives under `nabu/help/` and is ingested the same way.

```
content/
  <course-slug>/                 # a course (cohort)
    _index.md                   # course metadata
    _course.css                  # the course's look       (control file)
    _course.yaml                 # notes Nabu doesn't model (control file)
    <lesson-slug>/              # a lesson
      _index.md                 # lesson metadata
      _lesson.css               # overrides for this lesson (control file)
      _references.md            # shared links for this lesson (control file)
      00-welcome.md             # a slide
      10-first-task.md          # a slide
      50-with-image/            # a slide that has images (a "leaf bundle")
        index.md
        diagram.png
      teacher/                  # teacher-only pages (see below)
        00-briefing.md
```

## Control files — the `_` prefix

**A leading underscore means "this configures the lesson; it is not part of
it".** `_index.md` has always worked that way; the rule now covers every
`_`-prefixed file and folder, so you can keep supporting material beside your
slides without it turning into one.

A control file is never a slide, and:

| Who is looking | What they see |
|---|---|
| **Students** | Nothing. Control files are not listed, not indexed, not reachable. Their *effects* show up — the stylesheet applies, the references resolve — but the files themselves do not exist as far as a student is concerned. |
| **Teachers** | The files, shown dimmed and in monospace italic, so it is obvious which entries are configuration rather than lesson material. |
| **Admins** | Everything, unrestricted. |

It works for folders too: name one `_drafts/` or `_original-art/` and the whole
subtree is control material — useful for keeping source artwork or half-written
slides next to the lesson they belong to without Nabu trying to publish them.

## Teacher-only pages

A `teacher/` folder holds companion pages only staff can open — a briefing, a
debrief, a quiz answer key. They are **not** picked up automatically; name them
in the lesson's `_index.md`:

```yaml
params:
  teacher_pages:
    briefing: teacher/00-briefing.md
    answers:  teacher/90-answers.md
```

Each becomes a page at `/teacher/<lesson>/page/<key>`. They render exactly like a
slide but are never part of the student sequence. Belt and braces: set
`params.audience: teacher` in the page's own frontmatter too.
