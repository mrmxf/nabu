---
title: Styling a course
params:
  readiness: production
  kind: presentation
---
Nabu ships a look. A course can re-theme it, and one lesson can override the
course — without any slide having to know about it.

## Three layers

```
nabu.css        Nabu's own tokens and defaults          (always loaded)
   ↓
_course.css      your course's look                      (whole course)
   ↓
_lesson.css     an exception for one lesson             (that lesson only)
   ↓
<style> block   a genuine one-off, on a single slide    (rare)
```

Each layer loads after the one above, so it wins. Point at them from
frontmatter — the filenames are a convention, not magic:

```yaml
# content/<course>/_index.md
params:
  css: _course.css

# content/<course>/<lesson>/_index.md
params:
  css: _lesson.css
```

Both are [control files](help:authoring-lessons#naming-ordering), so students
never see them in a file listing — only their effect.

## Write tones, not colours

Layout widgets don't take colours. They take a **tone** — `define`, `ok`, `warn`,
`info`, `attention`, `step`, `neutral` — and your stylesheet decides what each
one looks like. Change the palette, and every box in the course changes with it.

A `_course.css` has three tiers. **You normally only edit the first.**

```css
/* content/<course>/_course.css
   Every rule must sit under `.slide .body` — this file is loaded as an ordinary
   stylesheet, so unscoped rules would leak into Nabu's own chrome. */

.slide .body {
  /* TIER 1 · your palette — the only colours in the file */
  --lime:    hsl(80 45% 49%);
  --magenta: hsl(324 45% 42%);
  --ochre:   hsl(46 50% 52%);
  --indigo:  hsl(243 36% 39%);

  /* TIER 2 · what those colours MEAN — what authors type */
  --define:    var(--lime);
  --ok:        var(--lime);
  --attention: var(--magenta);
  --warn:      var(--ochre);
  --step:      var(--ochre);
  --info:      var(--indigo);

  /* TIER 3 · Nabu's own tokens, so built-in chrome matches too */
  --hi1: var(--ok);        --hi2: var(--attention);
  --hi3: var(--warn);      --hi4: var(--info);
}
```

Re-theming a course for a different client is then a six-line edit, and no slide
changes at all.

## Dark mode

Viewers can switch themes. Re-point **tier 1 only** and the rest follows:

```css
:root[data-theme="dark"] .slide .body {
  --lime:    hsl(80 45% 64%);
  --magenta: hsl(324 50% 68%);
  --ochre:   hsl(46 55% 68%);
  --indigo:  hsl(243 50% 68%);
}
```

Handle both `@media (prefers-color-scheme: dark)` (the viewer's system setting)
and `:root[data-theme="dark"]` (the in-app toggle), so the toggle always wins.

## When to reach for a `<style>` block

Almost never. A `<style>` in a slide body is **not scoped** — it applies to every
slide from the moment that slide is shown. It is the right tool for a genuine
one-off (nudging one diagram into place), and the wrong tool for anything you
might want twice. If you are writing the same block on a second slide, it belongs
in `_lesson.css`; on a second lesson, in `_course.css`.
