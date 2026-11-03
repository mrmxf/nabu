---
title: Naming and ordering
params:
  readiness: production
  kind: presentation
---
- The `NN-` number prefix on a filename sets the order (`10-`, `20-`, …). Leave
  gaps so you can insert slides later.
- The **slug** is the filename with the prefix and `.md` removed
  (`10-first-task.md` → `first-task`). Override it with `slug:` in frontmatter.
- Two slides can share a title but must have different slugs (different
  filenames).

## The `_` prefix means "control file"

**A leading underscore marks a file or folder as configuration rather than
content.** `_index.md` has always behaved this way; the rule is general.

Anything starting with `_` is **never a slide, lesson or course**. That is what
lets you keep supporting material beside your slides:

| File | What it does |
|---|---|
| `_index.md` | metadata for the course or lesson it sits in |
| `_course.css` / `_lesson.css` | how the course / this lesson looks |
| `_references.md` | shared links, pulled in with the `include` widget |
| `_course.yaml` | your own notes — Nabu ignores files it doesn't know |
| `_drafts/`, `_original-art/` | a whole subtree of working material |

Who sees them:

| Audience | Behaviour |
|---|---|
| **Student** | Never listed, never indexed, never reachable. The *effects* are visible (styling applies, includes resolve); the files are not. |
| **Teacher** | Listed, but dimmed and in monospace italic so configuration never reads as lesson material. |
| **Admin** | No restriction. |

Two practical consequences:

- **Renaming a slide to `_old-thing.md` retires it** without deleting it. It
  stops being a slide immediately, and students can't reach it.
- **A stray folder can't become a lesson by accident.** Drop a folder of source
  images into a course and Nabu would otherwise try to publish it as a lesson;
  name it `_original-art/` and it is simply ignored.
