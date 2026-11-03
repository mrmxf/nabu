package content

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// includeRe matches an `include` widget on its own line. Includes are the one
// widget that is NOT a widget by the time goldmark runs: they splice Markdown
// into the source, so they must be expanded before anything else sees the text.
// Own-line only — an include is a block operation, and allowing one mid-sentence
// would splice a document into the middle of a paragraph.
var includeRe = regexp.MustCompile(`(?m)^[ \t]*{{<\s*LMS\s+([^>]*?\bt\s*=\s*"include"[^>]*?)\s*>}}[ \t]*$`)

// linkDefRe matches a Markdown link reference definition, `[key]: url "title"`,
// so an include can be filtered to named keys with `only=`.
var linkDefRe = regexp.MustCompile(`(?m)^\[([^\]]+)\]:`)

// expandIncludes splices `{{< LMS t="include" from="_references.md" >}}` into the
// source Markdown, returning the expanded text and any issues.
//
// ── Why this runs before everything else ──────────────────────────────────────
//
// The motivating use is a lesson's shared link references. Markdown link
// reference definitions — `[zapier]: https://zapier.com` — only resolve WITHIN A
// SINGLE DOCUMENT: goldmark collects them per parse. So for a slide to write
// `[Zapier][zapier]` and have it become a link, the definitions must be part of
// that slide's source text before conversion.
//
// That is why include is expanded here, in the renderer's first step, rather
// than being an <lms> element hydrated in the browser like every other widget.
// By the time preprocessLMS runs, the included text is indistinguishable from
// text the author typed — which is exactly the point. Move this after goldmark
// and reference-style links silently stop resolving.
//
// Resolution is bundle-relative first, then the lesson dir, so a leaf-bundle
// slide (lesson/<slide>/index.md) and a teacher page (lesson/teacher/x.md) both
// reach the one _references.md their lesson owns without ../ games. A reference
// that escapes the lesson is refused: partials are lesson-scoped.
//
// Includes do NOT nest. Included text is spliced verbatim and never rescanned,
// so a cycle is impossible by construction rather than by depth counting.
//
// A missing file is a WARNING, not a failure: the slide still renders, minus its
// link definitions, and /_/ shows the author what to fix.
func expandIncludes(md string, ctx RenderContext) (string, []lmsIssue) {
	if !strings.Contains(md, `t="include"`) && !strings.Contains(md, `t = "include"`) {
		return md, nil // fast path: nothing to do
	}
	var issues []lmsIssue

	// Protect code regions, so an include written inside a fence stays literal —
	// that is how the help lessons document this widget without triggering it.
	masked, restore := maskCode(md)

	out := includeRe.ReplaceAllStringFunc(masked, func(line string) string {
		args := parseArgs(includeRe.FindStringSubmatch(line)[1])
		from := strings.TrimSpace(args["from"])
		if from == "" {
			issues = append(issues, lmsIssue{StatusErrors, "LMS include: missing required arg from"})
			return ""
		}
		body, err := readPartial(ctx, from)
		if err != nil {
			issues = append(issues, lmsIssue{StatusWarnings,
				fmt.Sprintf("LMS include: %v", err)})
			return ""
		}
		if only := strings.Fields(args["only"]); len(only) > 0 {
			body = filterLinkDefs(body, only)
		}
		// Blank lines around the splice so the included block can never be absorbed
		// into a surrounding paragraph.
		return "\n" + body + "\n"
	})
	return restore(out), issues
}

// readPartial loads an included file, looking in the slide's own bundle first and
// then the lesson dir. `from` must stay inside the lesson: a partial is a
// lesson-scoped authoring convenience, not a way to read arbitrary files at seed
// time, so anything containing a parent traversal or an absolute path is refused
// outright rather than cleaned and hoped about.
func readPartial(ctx RenderContext, from string) (string, error) {
	if filepath.IsAbs(from) || !withinBundle(from) {
		return "", fmt.Errorf("%q must be a relative path inside the lesson", from)
	}
	if ctx.Source.IsZero() {
		return "", fmt.Errorf("file not found: %s (no content tree)", from)
	}
	candidates := []string{path.Join(ctx.BundleDir, from)}
	if ctx.LessonDir != ctx.BundleDir {
		candidates = append(candidates, path.Join(ctx.LessonDir, from))
	}
	for _, p := range candidates {
		b, err := fs.ReadFile(ctx.Source.FS, p)
		if err == nil {
			return strings.TrimRight(string(b), "\n"), nil
		}
	}
	return "", fmt.Errorf("file not found: %s", ctx.Source.Where(candidates[0]))
}

// filterLinkDefs keeps only the named link reference definitions, for a slide
// that wants a short visible list rather than the lesson's whole reference set.
// Non-definition lines are dropped: `only=` is meaningful for a references file,
// and silently keeping prose would make the filter look broken.
func filterLinkDefs(body string, only []string) string {
	want := make(map[string]bool, len(only))
	for _, k := range only {
		want[k] = true
	}
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		m := linkDefRe.FindStringSubmatch(line)
		if m != nil && want[m[1]] {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
