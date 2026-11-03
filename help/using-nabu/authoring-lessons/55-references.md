---
title: Shared references and includes
params:
  readiness: production
  kind: presentation
---
A lesson that sends students to a dozen tools quickly fills up with long URLs
that are painful to read in the source and painful to update when one moves.
Markdown has an answer, and Nabu has the piece that makes it work across slides.

## Reference-style links

Instead of putting the URL inline, name it:

```markdown
Try building the same flow in [Zapier][zapier] and [n8n][n8n].
```

and define the names once:

```markdown
[zapier]: https://zapier.com   "Zapier — no-code automation"
[n8n]:    https://n8n.io       "n8n — self-hosted automation"
```

The slide now reads as prose, and a URL that changes is fixed in one place.

## One file for the whole lesson

Put those definitions in a `_references.md` beside your slides — a
[control file](help:authoring-lessons#naming-ordering), so students never see
it — and pull it into each slide that uses one:

```
{{< LMS t="include" from="_references.md" >}}
```

Put it at the **bottom** of the slide. It renders nothing; it just makes the
names resolvable.

```
content/my-course/module-3/
  _references.md          ← every link the module uses
  100-tools.md            ← writes [Zapier][zapier]
  200-hands-on.md         ← writes [Zapier][zapier] too
```

Why the include is needed at all: Markdown resolves link names **within a single
document**. Two slides are two documents, so each one that uses `[Zapier][zapier]`
has to include the definitions. Nabu splices the file into the slide's source
*before* rendering, so the result is exactly as if you had typed the definitions
at the bottom of that slide.

### Only some of them

To show a short, visible list of just a few links:

```
{{< LMS t="include" from="_references.md" only="zapier n8n" >}}
```

## The rules

- **Own line.** An include is a block, not something you drop mid-sentence.
- **Lesson-scoped.** `from` is looked for in the slide's own folder first, then
  the lesson folder — so a leaf-bundle slide reaches the lesson's file without
  writing `../`. It cannot reach outside the lesson.
- **No nesting.** An included file is spliced as-is and not scanned again, so a
  file that includes itself simply stops.
- **A missing file is a warning, not a failure.** The slide still renders; the
  problem shows up against its `content` status in `/_/`.

## Other uses

Nothing about this is references-only. Any Markdown you repeat — a standing
safety note, a boilerplate hand-in instruction — can live in a `_`-prefixed
partial and be included wherever it is needed:

```
{{< LMS t="include" from="_handin.md" >}}
```
