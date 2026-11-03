---
title: Files students never see
params:
  readiness: production
  audience: teacher
  kind: presentation
---
Browsing a lesson's files, you will sometimes see entries in a **dimmed,
monospace italic** — like `_course.css` or `_references.md`. Those are **control
files**, and the styling is telling you two things.

## They configure the lesson; they are not part of it

A file or folder whose name starts with `_` holds supporting material: how the
course looks, links shared across slides, source artwork, drafts. It is never a
slide, and never appears in the student sequence.

## Students cannot see them at all

Not hidden-but-reachable — genuinely absent. They are filtered out on the server
before any list reaches a student's browser, so there is nothing to stumble onto
and nothing to guess at. What students see is the *effect*: the course's styling
applies, shared links resolve.

You see them because you may need to edit them. Admins see them unrestricted.

## Using it deliberately

- **Retire a slide without deleting it.** Rename `40-old-task.md` to
  `_40-old-task.md`. It stops being a slide immediately; the text is still there
  when you want it back.
- **Park work in progress.** A `_drafts/` folder can sit right next to the lesson
  it belongs to. The whole subtree is control material.
- **Keep source material with the lesson.** Original artwork, the spreadsheet a
  table came from — put them in `_source/` rather than somewhere you will lose
  them.

One caution: control files are hidden from students, not secured against them.
Treat them as *out of the way*, not as a place for anything genuinely sensitive.
Answer keys and marking notes belong on a **teacher page** (`teacher/…`, listed
under `teacher_pages` in the lesson's `_index.md`), which is served staff-only.
