package content

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// This file implements the goldmark-path side of the LMS widget contract. A
// slide body may contain inline widgets written as Hugo-style shortcodes:
//
//	{{< LMS t="timer" start="onshow" duration="15:00" >}}
//
// which preprocess into an inert, renderer-agnostic element carrying an
// HTML-escaped JSON payload that the client hydrates into a Svelte component:
//
//	<lms param='{"duration":"15:00","start":"onshow","t":"timer"}'></lms>
//
// The arg schemas are NOT declared here — they live in nabu-widget-catalogue.yaml
// (embedded below), the single source of truth shared with Hugo and the client.
// This code only validates payloads against that catalogue and stamps per-slide
// issues.

//go:embed nabu-widget-catalogue.yaml
var lmsCatalogYAML []byte

// propSchema is the (JSON-Schema-shaped) schema of one widget param.
type propSchema struct {
	Type        string   `yaml:"type"`
	Enum        []string `yaml:"enum"`
	Pattern     string   `yaml:"pattern"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Default     any      `yaml:"default"`
}

// widgetSchema is the JSON Schema for one widget type's params.
type widgetSchema struct {
	Title       string                `yaml:"title"`
	Description string                `yaml:"description"`
	Type        string                `yaml:"type"`
	Properties  map[string]propSchema `yaml:"properties"`
	Required    []string              `yaml:"required"`
	// DefaultValueKey names the field a referenceable widget (e.g. choice) exposes
	// to a `print` when nothing overrides it. Validated per-group at seed.
	DefaultValueKey string `yaml:"default_value_key"`
}

// catalogChoiceDefaultKey returns the catalogue's default value field for choices
// ("slug" if unset/unavailable), used as the fallback when a t_choice group omits
// its own default_value_key.
func catalogChoiceDefaultKey() string {
	if c, err := loadCatalog(); err == nil {
		if w, ok := c.Widgets["choice"]; ok && w.DefaultValueKey != "" {
			return w.DefaultValueKey
		}
	}
	return "slug"
}

// lmsCatalog is the whole catalogue: widget type -> its params schema.
type lmsCatalog struct {
	Version int                     `yaml:"version"`
	Widgets map[string]widgetSchema `yaml:"widgets"`
}

var (
	catalogOnce sync.Once
	catalog     *lmsCatalog
	catalogErr  error
)

// loadCatalog parses the embedded nabu-widget-catalogue.yaml once. The catalogue
// is compiled-in, so a parse error here is a programming error, surfaced to
// callers.
func loadCatalog() (*lmsCatalog, error) {
	catalogOnce.Do(func() {
		var c lmsCatalog
		if err := yaml.Unmarshal(lmsCatalogYAML, &c); err != nil {
			catalogErr = fmt.Errorf("parse nabu-widget-catalogue.yaml: %w", err)
			return
		}
		catalog = &c
	})
	return catalog, catalogErr
}

// widgetTypes returns the sorted-free set of known widget `t` keys. Used by the
// registry-parity test.
func widgetTypes() ([]string, error) {
	c, err := loadCatalog()
	if err != nil {
		return nil, err
	}
	ts := make([]string, 0, len(c.Widgets))
	for t := range c.Widgets {
		ts = append(ts, t)
	}
	return ts, nil
}

// lmsIssue is a validation problem found while preprocessing a slide's widgets.
type lmsIssue struct {
	Level ContentStatus // StatusWarnings or StatusErrors
	Msg   string
}

// lmsLineRe matches a shortcode that occupies a whole line. Such a shortcode is
// emitted as a block element (goldmark HTML block, not paragraph-wrapped) — the
// right shape for the block widgets (choice, agent, timer, tabulate, browser).
var lmsLineRe = regexp.MustCompile(`(?m)^[ \t]*{{<\s*LMS\s+(.*?)\s*>}}[ \t]*$`)

// lmsInlineRe matches a shortcode anywhere, including embedded in a line of text
// (e.g. an inline `print` inside a sentence or heading). Applied after the
// own-line pass so it only sees shortcodes that pass detection there. The
// renderer runs with WithUnsafe, so the emitted inline <lms> element survives
// goldmark and is hydrated client-side just like a block one.
var lmsInlineRe = regexp.MustCompile(`{{<\s*LMS\s+(.*?)\s*>}}`)

// lmsCloseRe matches a paired-shortcode CLOSING tag `{{< /LMS >}}` on its own
// line. It closes a container widget (currently `callout`), emitted as the
// matching </aside>. It carries no args, so unlike an opening `{{< LMS t=… >}}`
// it is not validated against the catalogue.
var lmsCloseRe = regexp.MustCompile(`(?m)^[ \t]*{{<\s*/\s*LMS\s*>}}[ \t]*$`)

// argRe extracts key="value" pairs from a shortcode's argument string.
var argRe = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)

// fenceRe and codeSpanRe find code regions whose contents must be left literal:
// a `{{< LMS … >}}` written inside a code fence or inline code span is
// documentation (e.g. the help lessons that explain the widget syntax), not a
// widget to hydrate. They are masked before conversion and restored after.
var (
	fenceRe    = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")
	codeSpanRe = regexp.MustCompile("``[^`]*``|`[^`\n]*`")
)

// maskCode replaces every fenced code block and inline code span with an inert
// NUL-delimited token (a byte sequence that cannot occur in source Markdown),
// returning the masked text and a restore func that puts the originals back.
// Fences are masked before spans so backticks inside a fence don't confuse the
// span matcher.
func maskCode(s string) (string, func(string) string) {
	var stash []string
	store := func(m string) string {
		tok := fmt.Sprintf("\x00LMSCODE%d\x00", len(stash))
		stash = append(stash, m)
		return tok
	}
	s = fenceRe.ReplaceAllStringFunc(s, store)
	s = codeSpanRe.ReplaceAllStringFunc(s, store)
	return s, func(out string) string {
		for i, orig := range stash {
			out = strings.Replace(out, fmt.Sprintf("\x00LMSCODE%d\x00", i), orig, 1)
		}
		return out
	}
}

// preprocessLMS replaces every `{{< LMS … >}}` shortcode with its `<lms>` element
// and returns any validation issues found against the catalogue. Both block
// (own-line) and inline (mid-text) placements are supported. Rendering is never
// blocked: an unknown widget or bad arg still emits an element (so the author
// sees their intent) but is reported so the slide's content status reflects it.
func preprocessLMS(md string, choiceIDs, quizIDs []string) (string, []lmsIssue, error) {
	cat, err := loadCatalog()
	if err != nil {
		return md, nil, err
	}
	var issues []lmsIssue
	// convert one shortcode's argument string to its <lms> element, collecting
	// validation issues along the way.
	convert := func(argStr string) string {
		args := parseArgs(argStr)
		issues = append(issues, validateWidget(cat, args)...)
		issues = append(issues, validateChoiceRef(args, choiceIDs)...)
		issues = append(issues, validateGroupRef(args, "quiz", "t_quiz", quizIDs)...)
		return renderLMS(args)
	}
	// Protect code so shortcode EXAMPLES in fences / inline code stay literal
	// (the help lessons document the widget syntax in code).
	masked, restore := maskCode(md)
	// Pass 1 — own-line shortcodes become block elements.
	out := lmsLineRe.ReplaceAllStringFunc(masked, func(line string) string {
		return convert(lmsLineRe.FindStringSubmatch(line)[1])
	})
	// Pass 2 — any remaining shortcodes are inline; convert them in place. The
	// block pass already consumed the own-line ones, so this never double-converts.
	out = lmsInlineRe.ReplaceAllStringFunc(out, func(match string) string {
		return convert(lmsInlineRe.FindStringSubmatch(match)[1])
	})
	// Pass 3 — paired-shortcode closings: {{< /LMS >}} closes a container widget
	// (callout) with its matching </aside>.
	out = lmsCloseRe.ReplaceAllString(out, "</aside>")
	return restore(out), issues, nil
}

// validateChoiceRef checks that a `choice` widget references an id declared in
// the slide's t_choice frontmatter. A dangling id renders nothing (the client
// leaves the element inert), so it is a warning, not a hard error. A wholly
// missing id is already reported by validateWidget against the schema.
func validateChoiceRef(args map[string]string, choiceIDs []string) []lmsIssue {
	if args["t"] != "choice" {
		return nil
	}
	id := args["id"]
	if id == "" {
		return nil
	}
	if !contains(choiceIDs, id) {
		return []lmsIssue{{StatusWarnings,
			fmt.Sprintf("LMS choice references unknown id %q (no matching t_choice group)", id)}}
	}
	return nil
}

// validateGroupRef checks that a widget placing a frontmatter-declared group
// (currently `quiz`) references an id the slide actually declares. Same shape as
// validateChoiceRef: a dangling id renders an inert element rather than
// breaking the slide, so it is a warning. Factored out because every widget that
// follows the "bulky config in frontmatter, id in the body" pattern needs this
// exact check — pass the widget's `t`, the frontmatter key to name in the
// message, and the declared ids.
func validateGroupRef(args map[string]string, widget, fmKey string, ids []string) []lmsIssue {
	if args["t"] != widget {
		return nil
	}
	id := args["id"]
	if id == "" {
		return nil // a wholly missing id is already reported against the schema
	}
	if !contains(ids, id) {
		return []lmsIssue{{StatusWarnings,
			fmt.Sprintf("LMS %s references unknown id %q (no matching %s group)", widget, id, fmKey)}}
	}
	return nil
}

// parseArgs turns `t="timer" duration="15:00"` into an ordered-independent map.
func parseArgs(s string) map[string]string {
	args := map[string]string{}
	for _, m := range argRe.FindAllStringSubmatch(s, -1) {
		args[m[1]] = m[2]
	}
	return args
}

// renderLMS emits the inert <lms> element with an HTML-escaped JSON payload.
// json.Marshal sorts map keys, so output is deterministic.
//
// The switch below is the exception list: PRESENTATIONAL widgets render to plain
// HTML right here and are styled from CSS, rather than becoming an <lms> element
// the client hydrates into a Svelte component. If you add a widget that is only
// structure and styling, add it here — it needs no component, no registry entry,
// and no rebuild of the SPA. See contentwidgets.go for the reasoning and the
// tone vocabulary these share.
//
// `include` never reaches this function: it is expanded into the source Markdown
// before preprocessing (see expandIncludes).
func renderLMS(args map[string]string) string {
	switch args["t"] {
	// paired containers — closed by the author's {{< /LMS >}} => </aside>
	case "callout":
		return renderCalloutOpen(args)
	case "postit":
		return renderPostitOpen(args)
	case "define":
		return renderDefineOpen(args)
	case "steps":
		return renderStepsOpen(args)
	case "step":
		return renderStepOpen(args)
	case "grid":
		return renderGridOpen(args)
	case "proscons":
		return renderProsConsOpen(args)
	case "col":
		return renderColOpen(args)
	case "vocab":
		return renderVocabOpen(args)
	case "toolcard":
		return renderToolCardOpen(args)
	// self-closing presentational widgets
	case "banner":
		return renderBanner(args)
	case "itemtype":
		return renderItemType(args)
	case "video":
		return renderVideo(args)
	}
	payload, _ := json.Marshal(args)
	return `<lms param='` + html.EscapeString(string(payload)) + `'></lms>`
}

// nonClassRe strips anything that isn't a safe CSS class-suffix character, so an
// unexpected `style` value can never break the emitted class attribute.
var nonClassRe = regexp.MustCompile(`[^a-z0-9-]+`)

// renderCalloutOpen emits the OPENING of a `callout` container: a styled <aside>
// that wraps the markdown following it, up to the matching {{< /LMS >}} (see
// lmsCloseRe). `style` selects the look (default "notepad"); an optional `header`
// becomes a bold title. The header text is wrapped in an inner <span> so the
// faux Mac title-bar styles (prompt, cli-output) can give it a solid background
// that cuts through the pinstripes. The author surrounds the block with blank
// lines, so goldmark renders the inner markdown between the raw <aside> tags (a
// CommonMark HTML block). Styling lives globally in Slide.svelte (.lms-callout*).
func renderCalloutOpen(args map[string]string) string {
	style := args["style"]
	if style == "" {
		style = "notepad"
	}
	style = nonClassRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(style)), "-")
	out := `<aside class="lms-callout lms-callout-` + style + `">`
	if h := args["header"]; h != "" {
		out += `<div class="lms-callout-header"><span class="lms-callout-title">` +
			html.EscapeString(h) + `</span></div>`
	}
	return out
}

// postitColorRe whitelists the characters allowed in a post-it's `color` (which
// is stamped verbatim into an inline `style="--postit-bg:…"`). It permits the
// tokens of a CSS <color> — names, hex, and functional forms like
// `oklch(0.9 0.13 95)` — while stripping anything (quotes, `;`, `{}`) that could
// break out of the attribute or inject a second declaration.
var postitColorRe = regexp.MustCompile(`[^a-zA-Z0-9#().,%/ -]+`)

// renderPostitOpen emits the OPENING of a `postit` container: a draggable,
// minimisable sticky note whose `header` becomes a banner (also the drag handle)
// above the wrapped markdown, closed by the matching {{< /LMS >}} → </aside>.
// `color` sets the note background via the `--postit-bg` custom property; the
// banner and text colours are derived from it in CSS (see .lms-postit* in
// Slide.svelte). `subtype` ("" | list | code) is a presentation modifier class.
// Drag + minimise behaviour is wired client-side by hydrateLms.
func renderPostitOpen(args map[string]string) string {
	subtype := nonClassRe.ReplaceAllString(
		strings.ToLower(strings.TrimSpace(args["subtype"])), "-")
	course := "lms-postit"
	if subtype != "" {
		course += " lms-postit-" + subtype
	}
	out := `<aside class="` + course + `"`
	if color := strings.TrimSpace(args["color"]); color != "" {
		// html.EscapeString guards the attribute; postitColorRe guards the CSS.
		out += ` style="--postit-bg:` + html.EscapeString(postitColorRe.ReplaceAllString(color, "")) + `"`
	}
	// The banner leads with a drag-affordance glyph (the banner is the drag handle);
	// aria-hidden so it's decorative to screen readers.
	out += `><div class="lms-postit-banner"><i class="fa-solid fa-up-down-left-right lms-postit-grip" aria-hidden="true"></i><span class="lms-postit-title">`
	out += html.EscapeString(args["header"])
	// Starts collapsed (aria-expanded="false"); hydrateLms wraps the body, toggles
	// `is-open`, and keeps aria-expanded in sync.
	out += `</span><button type="button" class="lms-postit-min" aria-label="Expand or collapse note" aria-expanded="false"></button></div>`
	return out
}

// validateWidget checks one widget's args against the catalogue. An unknown `t`
// is a warning (renders, but the client may not hydrate it); a missing required
// arg is an error; an out-of-enum value is a warning.
func validateWidget(cat *lmsCatalog, args map[string]string) []lmsIssue {
	t := args["t"]
	if t == "" {
		return []lmsIssue{{StatusErrors, "LMS widget missing required `t`"}}
	}
	schema, ok := cat.Widgets[t]
	if !ok {
		return []lmsIssue{{StatusWarnings, fmt.Sprintf("unknown LMS widget %q", t)}}
	}
	var issues []lmsIssue
	for _, req := range schema.Required {
		if _, present := args[req]; !present {
			issues = append(issues, lmsIssue{StatusErrors,
				fmt.Sprintf("LMS %q missing required arg %q", t, req)})
		}
	}
	for k, v := range args {
		if k == "t" {
			continue
		}
		prop, known := schema.Properties[k]
		if !known {
			issues = append(issues, lmsIssue{StatusWarnings,
				fmt.Sprintf("LMS %q: unknown arg %q", t, k)})
			continue
		}
		if len(prop.Enum) > 0 && !contains(prop.Enum, v) {
			issues = append(issues, lmsIssue{StatusWarnings,
				fmt.Sprintf("LMS %q: %q=%q not one of %s", t, k, v, strings.Join(prop.Enum, "|"))})
		}
	}
	return issues
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
