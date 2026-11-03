package content

import (
	"bytes"
	"fmt"
	"html"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	ghtml "github.com/yuin/goldmark/renderer/html"

	"github.com/mrmxf/nabu/internal/source"
)

// Renderer turns a slide's source Markdown into HTML. Two implementations exist:
// GoldmarkRenderer (pure Go, the default demo path) and HugoRenderer (shells out
// to the hugo binary — wired but not yet implemented).
type Renderer interface {
	// Render converts src Markdown to HTML, returning any per-widget validation
	// issues found (used to stamp the slide's content status).
	Render(src string, ctx RenderContext) (html string, issues []lmsIssue, err error)
}

// RenderContext gives the renderer what it needs to resolve a slide's inline
// assets (page-bundle-relative images).
type RenderContext struct {
	// Source is the tree the slide was read from; BundleDir and LessonDir are
	// slash paths inside it. A zero Source means no files are reachable.
	Source    source.Tree
	BundleDir string // dir the slide's markdown lives in, relative to Source
	// LessonDir is the owning lesson's folder — the same as BundleDir for a flat
	// slide, its parent for a leaf bundle or teacher page. Includes fall back to
	// it so one _references.md serves every slide in the lesson.
	LessonDir  string
	PublicRel  string    // public path for this slide's assets, e.g. content/<course>/<lesson>/<slide>
	Assets     AssetSink // where copied assets go; may be nil (images left as-authored)
	ChoiceIDs  []string  // t_choice ids declared on this slide; validates {{< LMS t="choice" >}} refs
	QuizIDs    []string  // t_quiz ids declared on this slide; validates {{< LMS t="quiz" >}} refs
	CourseSlug string    // the current course slug; resolves `help:` cross-reference links
	LessonSlug string    // the current lesson slug; tells a same-lesson xref from a cross-lesson one
	// HelpRoot marks a slide in Nabu's built-in help tree, which has a static
	// route; authored courses don't. See rewriteXrefLinks.
	HelpRoot bool
	// LmsResolve, if set, converts an lms:// reference (already bound to this slide's
	// lesson bucket/prefix) into a served URL; ok=false leaves it as authored.
	LmsResolve func(ref string) (url string, ok bool)
}

// AssetSink copies a slide's bundle asset (the file rel inside src) into the
// served public tree under publicRel and returns the URL it will be served at.
type AssetSink interface {
	Copy(src source.Tree, rel, publicRel string) (url string, err error)
}

// NewRenderer returns the renderer named by `name` ("goldmark" or "hugo").
func NewRenderer(name string) (Renderer, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "goldmark":
		return NewGoldmarkRenderer(), nil
	case "hugo":
		return &HugoRenderer{}, nil
	default:
		return nil, fmt.Errorf("unknown renderer %q (want goldmark|hugo)", name)
	}
}

// GoldmarkRenderer renders Markdown in-process with goldmark. Content is trusted
// (teacher-authored, not user input), so raw HTML passes through unescaped —
// required for the emitted <lms> elements and authored inline HTML.
type GoldmarkRenderer struct {
	gm goldmark.Markdown
}

// NewGoldmarkRenderer builds a goldmark configured for GFM with standard
// CommonMark line handling: a single newline inside a paragraph is a soft break
// (rendered as a space, so prose wrapped at ~80 cols flows), a blank line starts
// a new paragraph, and an explicit line break needs a trailing `<br>` (or two
// trailing spaces / a `\`). Hard wraps were dropped so authored line-wrapping no
// longer injects a `<br>` at every wrap.
func NewGoldmarkRenderer() *GoldmarkRenderer {
	return &GoldmarkRenderer{
		gm: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithRendererOptions(ghtml.WithUnsafe()),
		),
	}
}

// imgSrcRe matches the src attribute of an <img> tag in rendered HTML.
var imgSrcRe = regexp.MustCompile(`(<img\b[^>]*\bsrc=")([^"]+)(")`)

// Render preprocesses LMS shortcodes, converts to HTML, then rewrites relative
// image sources to their served public URLs (copying the files via Assets).
func (r *GoldmarkRenderer) Render(src string, ctx RenderContext) (string, []lmsIssue, error) {
	// Includes FIRST: they splice Markdown into the source, so everything after
	// this point — shortcode preprocessing, goldmark, the rewrites — sees the
	// included text as if the author had typed it. That ordering is what makes a
	// shared _references.md resolve reference-style links; see expandIncludes.
	src, issues := expandIncludes(src, ctx)
	pre, preIssues, err := preprocessLMS(src, ctx.ChoiceIDs, ctx.QuizIDs)
	issues = append(issues, preIssues...)
	if err != nil {
		return "", issues, err
	}
	var buf bytes.Buffer
	if err := r.gm.Convert([]byte(pre), &buf); err != nil {
		return "", issues, fmt.Errorf("goldmark convert: %w", err)
	}
	out, imgIssues := r.rewriteImages(buf.String(), ctx)
	issues = append(issues, imgIssues...)
	out, linkIssues := r.rewriteLinks(out, ctx)
	issues = append(issues, linkIssues...)
	out = rewriteXrefLinks(out, ctx)
	out, lmsIssues := rewriteLmsRefs(out, ctx)
	issues = append(issues, lmsIssues...)
	return out, issues, nil
}

// lmsRefRe matches an lms:// reference in a rendered src/href attribute.
var lmsRefRe = regexp.MustCompile(`(\b(?:src|href)=")(lms://[^"]+)(")`)

// rewriteLmsRefs resolves lms:// references in a slide's src/href attributes to
// served storage URLs via ctx.LmsResolve. An unresolved reference (unconfigured
// storage, unsupported scope, malformed URI) is left as authored and stamped as a
// WARNING so it surfaces in /_/ rather than silently 404ing in the viewer. Only
// course-resource refs resolve statically today; group/student refs are viewer-
// dependent and deferred, so they warn here.
func rewriteLmsRefs(htmlStr string, ctx RenderContext) (string, []lmsIssue) {
	if ctx.LmsResolve == nil {
		return htmlStr, nil
	}
	var issues []lmsIssue
	out := lmsRefRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		parts := lmsRefRe.FindStringSubmatch(m)
		ref := parts[2]
		url, ok := ctx.LmsResolve(ref)
		if !ok {
			issues = append(issues, lmsIssue{StatusWarnings, fmt.Sprintf("unresolved lms reference: %s", ref)})
			return m
		}
		return parts[1] + url + parts[3]
	})
	return out, issues
}

// helpLinkRe matches an author's stable cross-reference link, written as
// `[text](help:<lesson>)`, `help:<course>/<lesson>`, with an optional `#<slide>`.
var helpLinkRe = regexp.MustCompile(`href="help:([^"]+)"`)

// rewriteXrefLinks turns `help:` cross-reference links into something the player
// can actually follow. Slug-based, so they survive file moves and reordering —
// unlike relative .md paths, which 404 in the rendered viewer. A bare
// `help:<lesson>` targets the current course; `help:<course>/<lesson>` is explicit.
//
// ── Why this function emits two different things ───────────────────────────────
//
// If you are here because a lesson link went somewhere unexpected, this is the
// load-bearing fact: THERE IS EXACTLY ONE STATIC COURSE/LESSON ROUTE IN THE APP,
// and it only serves the help tree —
//
//	web/src/routes/nabu/help/[course]/[lesson]/+page.svelte
//
// An authored course has no equivalent. Its slides are only ever on screen
// inside a running session (routes/session/[id]/run), where which slide you are
// on is player state persisted as sessions.current_slide — not anything
// addressable by URL. So `/BTASB/04-observability/#a4-2` is not a route that
// exists, and adding one would mean inventing a whole session-less viewer.
//
// Hence:
//
//   - HELP tree (ctx.HelpRoot) — a reference becomes a real URL:
//     /nabu/help/<course>/<lesson>#<slide>.
//   - AUTHORED course, SAME lesson — becomes an in-player jump: an anchor
//     carrying data-nabu-slide, which the run page intercepts (see onBodyClick
//     in routes/session/[id]/run/+page.svelte) to move the deck. The href stays
//     a plain fragment so the link still means something if the handler never
//     runs, and so middle-click doesn't navigate away.
//   - AUTHORED course, ANY OTHER lesson or course — there is nothing to point at:
//     you would need a session for that other lesson, and we cannot conjure one.
//     Left exactly as authored so validateXrefs can find it and say why. Do not
//     "fix" this by emitting a URL; it will 404.
//
// If a static authored-course viewer is ever added, this is the ONE place to
// change: give it the same treatment as the HelpRoot branch and drop the
// same-lesson restriction. validateXrefs and its xref_test.go cases are the
// contract to update alongside it.
func rewriteXrefLinks(htmlStr string, ctx RenderContext) string {
	return helpLinkRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		ref := helpLinkRe.FindStringSubmatch(m)[1]
		frag := ""
		if i := strings.IndexByte(ref, '#'); i >= 0 {
			frag, ref = ref[i:], ref[:i]
		}
		cls, lesson := ctx.CourseSlug, ref
		if i := strings.IndexByte(ref, '/'); i >= 0 {
			cls, lesson = ref[:i], ref[i+1:]
		}
		if ctx.HelpRoot {
			return fmt.Sprintf(`href="/nabu/help/%s/%s%s"`, cls, lesson, frag)
		}
		// Authored course: a same-lesson jump is the only expressible target, and
		// only with a #slide — `help:<lesson>` alone names a lesson, and "go to
		// the lesson you are already in" is not a navigation.
		if cls == ctx.CourseSlug && lesson == ctx.LessonSlug && frag != "" {
			slide := strings.TrimPrefix(frag, "#")
			return fmt.Sprintf(`href="#%s" data-nabu-slide="%s"`,
				html.EscapeString(slide), html.EscapeString(slide))
		}
		return m // unresolvable here — validateXrefs explains why, to the author
	})
}

// rewriteImages copies page-bundle-relative images into the public tree and
// points their src at the served URL. Absolute or remote srcs are left alone.
func (r *GoldmarkRenderer) rewriteImages(htmlStr string, ctx RenderContext) (string, []lmsIssue) {
	var issues []lmsIssue
	out := imgSrcRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		parts := imgSrcRe.FindStringSubmatch(m)
		src := parts[2]
		if isRemoteOrAbsolute(src) || !withinBundle(src) || ctx.Assets == nil {
			return m
		}
		rel := path.Join(ctx.BundleDir, src)
		if !fileIn(ctx.Source, rel) {
			issues = append(issues, lmsIssue{StatusWarnings, fmt.Sprintf("image not found: %s", ctx.Source.Where(rel))})
			return m
		}
		url, err := ctx.Assets.Copy(ctx.Source, rel, path.Join(ctx.PublicRel, src))
		if err != nil {
			issues = append(issues, lmsIssue{StatusWarnings, fmt.Sprintf("copy image %s: %v", src, err)})
			return m
		}
		return parts[1] + url + parts[3]
	})
	return out, issues
}

// fileIn reports whether rel names a regular file inside src.
func fileIn(src source.Tree, rel string) bool {
	if src.IsZero() {
		return false
	}
	st, err := fs.Stat(src.FS, rel)
	return err == nil && st.Mode().IsRegular()
}

func isRemoteOrAbsolute(src string) bool {
	return strings.HasPrefix(src, "/") ||
		strings.HasPrefix(src, "http://") ||
		strings.HasPrefix(src, "https://") ||
		strings.HasPrefix(src, "data:")
}

// anchorHrefRe matches the href attribute of an <a> tag in rendered HTML.
var anchorHrefRe = regexp.MustCompile(`(<a\b[^>]*\bhref=")([^"]+)(")`)

// rewriteLinks copies page-bundle-relative asset *links* (a linked PDF, zip,
// audio, an image referenced as a download, …) into the public tree and points
// their href at the served URL — the <a href> analogue of rewriteImages.
// Untouched: remote/absolute links, in-page anchors, `help:` cross-references,
// mail/tel links, and relative `.md` links (content cross-refs — use `help:`).
func (r *GoldmarkRenderer) rewriteLinks(htmlStr string, ctx RenderContext) (string, []lmsIssue) {
	if ctx.Assets == nil {
		return htmlStr, nil
	}
	var issues []lmsIssue
	out := anchorHrefRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		parts := anchorHrefRe.FindStringSubmatch(m)
		href := parts[2]
		if !isBundleRef(href) {
			return m
		}
		ref, frag := splitFragment(href)
		if !withinBundle(ref) || strings.EqualFold(path.Ext(ref), ".md") {
			return m // escapes the bundle (a repo cross-reference) or a .md xref
		}
		url, issue := copyBundleAsset(ctx.Source, ctx.BundleDir, ctx.PublicRel, ref, ctx.Assets)
		if issue != nil {
			issues = append(issues, *issue)
			return m
		}
		return parts[1] + url + frag + parts[3]
	})
	return out, issues
}

// isBundleRef reports whether href is a relative reference into the page bundle
// (not remote/absolute, an in-page anchor, a help: xref, or a mail/tel link).
func isBundleRef(href string) bool {
	if href == "" || isRemoteOrAbsolute(href) || strings.HasPrefix(href, "#") {
		return false
	}
	return !strings.HasPrefix(href, "help:") &&
		!strings.HasPrefix(href, "mailto:") &&
		!strings.HasPrefix(href, "tel:")
}

// withinBundle reports whether a cleaned relative ref stays inside the page
// bundle (does not climb above it with `..`). Refs that escape the bundle — e.g.
// the help docs' repo cross-references like `../../../internal/x.go` — are left
// as authored, never copied. This also prevents a `..` ref from writing outside
// the intended public directory.
func withinBundle(ref string) bool {
	c := path.Clean(ref)
	return c != ".." && !strings.HasPrefix(c, "../")
}

// splitFragment separates a reference from its trailing #fragment / ?query.
func splitFragment(ref string) (pathPart, frag string) {
	if i := strings.IndexAny(ref, "#?"); i >= 0 {
		return ref[:i], ref[i:]
	}
	return ref, ""
}

// copyBundleAsset copies bundleDir/ref (inside src) into the public tree under
// publicRel and returns its served URL. On a missing file or copy error it
// returns a WARNINGS issue and an empty url. Shared by rewriteLinks and the
// frontmatter logo copier.
func copyBundleAsset(src source.Tree, bundleDir, publicRel, ref string, assets AssetSink) (string, *lmsIssue) {
	rel := path.Join(bundleDir, ref)
	if !fileIn(src, rel) {
		return "", &lmsIssue{StatusWarnings, fmt.Sprintf("linked file not found: %s", src.Where(rel))}
	}
	url, err := assets.Copy(src, rel, path.Join(publicRel, ref))
	if err != nil {
		return "", &lmsIssue{StatusWarnings, fmt.Sprintf("copy file %s: %v", ref, err)}
	}
	return url, nil
}

// resolveBundleFile turns a course/lesson frontmatter file reference (`logo`,
// `css`) into a served URL. A bundle-relative file is copied into the public tree
// (like page images) and its public URL returned; a remote/absolute value is used
// as-is; anything else (no sink, or the file isn't in the bundle) is returned
// unchanged — for a logo the frontend can still resolve it by basename, and for a
// stylesheet it is simply an href we don't own. publicRel is the node's public
// dir, e.g. content/<course> or content/<course>/<lesson>.
//
// A missing file is NOT an error here: copyBundleAsset stamps a warning and we
// fall through to the raw value, so a mistyped stylesheet degrades to an unstyled
// course rather than failing the seed.
func resolveBundleFile(bundleDir, publicRel, raw string, opts Options) string {
	if raw == "" || isRemoteOrAbsolute(raw) || !withinBundle(raw) || opts.Assets == nil {
		return raw
	}
	if url, issue := copyBundleAsset(opts.src, bundleDir, publicRel, raw, opts.Assets); issue == nil {
		return url
	}
	return raw
}

// HugoRenderer will render the tree with an external hugo build and import the
// generated HTML. The interface + wiring exist now; the implementation is a
// follow-up, so this returns a clear error rather than a silent no-op.
type HugoRenderer struct{}

// Render is not yet implemented (see type doc).
func (h *HugoRenderer) Render(string, RenderContext) (string, []lmsIssue, error) {
	return "", nil, fmt.Errorf("hugo renderer not yet implemented (use --renderer goldmark)")
}
