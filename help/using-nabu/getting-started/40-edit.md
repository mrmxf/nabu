---
title: Edit a slide
params:
  readiness: production
  kind: presentation
---
Open `content/my-course/first-lesson/010-welcome.md` in any text editor. The
part between the `---` lines is the **frontmatter**, the slide's settings. Below
it is ordinary Markdown.

Change the text, save, then reload the course:

```bash
nabu stop      # or Ctrl-C in the serve terminal
nabu seed
nabu serve
```

To add a slide, add a file. The number at the front of its name sets the order,
so `020-what-next.md` comes after `010-welcome.md`:

```markdown
---
title: What next
params:
  readiness: production
  kind: presentation
---
A second slide.
```

`nabu seed` stops with an error that names the file and the line if a slide is
wrong. The [authoring guide][authoring] explains every setting, and the
[widgets][widgets] page lists what you can put on a slide.

[authoring]: help:authoring-lessons
[widgets]: help:authoring-lessons#widgets
