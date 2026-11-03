package content

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Presentational widgets — structure an author reaches for repeatedly (a
// definition box, a numbered procedure, a pros/cons comparison, a glossary
// entry, a tool card).
//
// Every widget here follows the `callout`/`postit` precedent rather than the
// Svelte-component one: it renders to plain HTML at seed time, styled entirely
// from CSS. Nothing is hydrated, nothing is registered in the client's
// lmsRegistry, and nothing here touches the SvelteKit build. That matters
// because these are the widgets a course uses most; making each one a component
// would put a network of Svelte files between an author and a box with a border.
//
// COLOUR: none of these emit a colour. They emit a `lms-tone-<name>` class, and
// the course stylesheet (_course.css) decides what "warn" looks like. That is the
// whole point of the tone vocabulary — an author writes what they MEAN and the
// course decides how it looks, so re-theming is a palette edit rather than a
// find-and-replace across a hundred slides.
//
// PAIRED CONTAINERS: a widget that wraps content opens an <aside> here and is
// closed by the author's matching `{{< /LMS >}}`, which lmsCloseRe turns into a
// bare `</aside>`. That is why every container in this file is an <aside> — the
// closing tag is not per-widget, so they must all close the same way. Nesting
// works naturally (grid > col, steps > step) because opens and closes pair up in
// document order.

// toneRe/tones guard the `tone` param. An unknown tone still renders (with no
// tone class) rather than emitting a class that silently matches nothing.
var validTones = map[string]bool{
	"define": true, "ok": true, "warn": true, "info": true,
	"attention": true, "step": true, "neutral": true,
}

// toneClass returns the CSS class for a tone, or "" when unset/unknown. The
// catalogue reports an out-of-enum value as a warning, so this only has to be
// safe, not diagnostic.
func toneClass(tone string) string {
	t := nonClassRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(tone)), "-")
	if t == "" || !validTones[t] {
		return ""
	}
	return " lms-tone-" + t
}

// openAside builds `<aside class="lms-<name>[ lms-tone-x]"` plus any extra
// attributes, leaving the caller to append its header markup and `>`.
func openAside(name, tone, extra string) string {
	return `<aside class="lms-` + name + toneClass(tone) + `"` + extra + `>`
}

// titleDiv emits a container's optional heading, in the shape the CSS expects
// (`.lms-<name>-header` wrapping `.lms-<name>-title`) — the same structure
// renderCalloutOpen uses, so one set of heading rules covers every container.
func titleDiv(name, text string) string {
	if text == "" {
		return ""
	}
	return `<div class="lms-` + name + `-header"><span class="lms-` + name + `-title">` +
		html.EscapeString(text) + `</span></div>`
}

// renderDefineOpen opens a `define` container — the workhorse box for a
// definition, key idea or worked example. Tone-coloured, with an optional header.
func renderDefineOpen(args map[string]string) string {
	tone := args["tone"]
	if tone == "" {
		tone = "define"
	}
	return openAside("define", tone, "") + titleDiv("define", args["header"])
}

// renderStepsOpen opens a `steps` container: an ordered procedure whose child
// `step` containers are numbered by a CSS counter, so inserting a step in the
// middle renumbers the rest with no authoring change. `start` seeds the counter
// for a procedure continued across slides.
func renderStepsOpen(args map[string]string) string {
	extra := ""
	if n, err := strconv.Atoi(strings.TrimSpace(args["start"])); err == nil && n != 1 {
		// counter-reset is one less than the first number we want to show
		extra = ` style="counter-reset:lms-step ` + strconv.Itoa(n-1) + `"`
	}
	return openAside("steps", "", extra)
}

// renderStepOpen opens one `step` card inside a `steps` container. The number
// comes from CSS (::before on .lms-step), not from here.
func renderStepOpen(args map[string]string) string {
	tone := args["tone"]
	if tone == "" {
		tone = "step"
	}
	return openAside("step", tone, "") + titleDiv("step", args["title"])
}

// gridLenRe whitelists a CSS length for the grid's min column width, which is
// stamped into an inline custom property. Digits, a decimal point and a unit —
// enough for "18rem"/"240px"/"30%", and nothing that could close the attribute
// or start a second declaration.
var gridLenRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(rem|em|px|ch|%)$`)

// renderGridOpen opens a responsive `grid` of `col` children. `cols` is the
// preferred column count and `min` the point at which columns wrap, so the same
// markup reflows to one column on a phone without an authored breakpoint.
func renderGridOpen(args map[string]string) string {
	var style []string
	if n, err := strconv.Atoi(strings.TrimSpace(args["cols"])); err == nil && n > 0 && n <= 6 {
		style = append(style, "--lms-grid-cols:"+strconv.Itoa(n))
	}
	if m := strings.TrimSpace(args["min"]); gridLenRe.MatchString(m) {
		style = append(style, "--lms-grid-min:"+m)
	}
	extra := ""
	if len(style) > 0 {
		extra = ` style="` + html.EscapeString(strings.Join(style, ";")) + `"`
	}
	return openAside("grid", "", extra)
}

// renderProsConsOpen opens a `proscons` comparison: a two-column grid preset.
// It expects two `col` children — conventionally tone="ok" then
// tone="attention". It is deliberately a preset over `grid` rather than a widget
// that splits its own body: the author labels each side with the col's own
// `title`, and one set of column rules serves both widgets.
func renderProsConsOpen(args map[string]string) string {
	return `<aside class="lms-grid lms-proscons" style="--lms-grid-cols:2">`
}

// renderColOpen opens one titled cell of a `grid` or `proscons`.
func renderColOpen(args map[string]string) string {
	return openAside("col", args["tone"], "") + titleDiv("col", args["title"])
}

// renderVocabOpen opens a `vocab` entry: a glossary term with its part of
// speech, the definition as the wrapped body, and an optional worked example.
// The example is emitted BEFORE the body (CSS orders it after) so the whole
// widget is one opening tag — the closing `{{< /LMS >}}` is a bare </aside> and
// cannot carry trailing markup.
func renderVocabOpen(args map[string]string) string {
	out := openAside("vocab", "define", "")
	out += `<div class="lms-vocab-header"><span class="lms-vocab-term">` +
		html.EscapeString(args["term"]) + `</span>`
	if pos := args["pos"]; pos != "" {
		out += `<span class="lms-vocab-pos">` + html.EscapeString(pos) + `</span>`
	}
	out += `</div>`
	if ex := args["example"]; ex != "" {
		out += `<p class="lms-vocab-example">` + html.EscapeString(ex) + `</p>`
	}
	return out
}

// renderToolCardOpen opens a `toolcard`: one entry in a directory of tools. The
// wrapped body is the description. `url` is rendered as a link only when it is
// http(s) — a tool directory is a list of links, and a `javascript:` href in a
// slide body has no legitimate use.
func renderToolCardOpen(args map[string]string) string {
	out := openAside("toolcard", "", "")
	out += `<div class="lms-toolcard-header">`
	name := html.EscapeString(args["name"])
	if u := safeHTTPURL(args["url"]); u != "" {
		out += `<a class="lms-toolcard-name" href="` + u + `" target="_blank" rel="noopener noreferrer">` + name + `</a>`
	} else {
		out += `<span class="lms-toolcard-name">` + name + `</span>`
	}
	if cost := args["cost"]; cost != "" {
		out += `<span class="lms-toolcard-cost">` + html.EscapeString(cost) + `</span>`
	}
	out += `</div>`
	if focus := args["focus"]; focus != "" {
		out += `<p class="lms-toolcard-focus">` + html.EscapeString(focus) + `</p>`
	}
	return out
}

// renderBanner emits a `banner`: a full-width section divider. Self-closing —
// its text is a param, not wrapped content, because a divider is a label rather
// than a container.
func renderBanner(args map[string]string) string {
	var style []string
	if f := toneVar(args["from"]); f != "" {
		style = append(style, "--lms-banner-from:"+f)
	}
	if t := toneVar(args["to"]); t != "" {
		style = append(style, "--lms-banner-to:"+t)
	}
	extra := ""
	if len(style) > 0 {
		extra = ` style="` + html.EscapeString(strings.Join(style, ";")) + `"`
	}
	return `<div class="lms-banner"` + extra + `><span class="lms-banner-text">` +
		html.EscapeString(args["text"]) + `</span></div>`
}

// toneVar maps a tone name to the CSS custom property the course stylesheet
// defines for it, so a banner's gradient is expressed in the same tone
// vocabulary as everything else. Unknown tones yield "" (the CSS default wins).
func toneVar(tone string) string {
	t := nonClassRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(tone)), "-")
	if t == "" || !validTones[t] {
		return ""
	}
	return "var(--" + t + ")"
}

// renderItemType emits a small inline badge naming what a slide is (assignment,
// discussion, quiz, page). It replaces the banner images an LMS export typically
// carries — the same information as text, at a fraction of the bytes, and
// themeable.
func renderItemType(args map[string]string) string {
	kind := nonClassRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(args["kind"])), "-")
	if kind == "" {
		return ""
	}
	label := map[string]string{
		"assignment": "Assignment",
		"discussion": "Discussion",
		"quiz":       "Quiz",
		"page":       "Page",
	}[kind]
	if label == "" {
		label = args["kind"]
	}
	return `<span class="lms-itemtype lms-itemtype-` + kind + `">` +
		html.EscapeString(label) + `</span>`
}

// ytIDRe matches a bare YouTube video id.
var ytIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// vimeoIDRe matches a bare Vimeo video id.
var vimeoIDRe = regexp.MustCompile(`^[0-9]{6,12}$`)

// ratioRe whitelists an aspect ratio like "16:9".
var ratioRe = regexp.MustCompile(`^[0-9]{1,2}:[0-9]{1,2}$`)

// renderVideo emits a responsive, lazily-loaded video embed.
//
// `src` accepts a watch/share/embed URL or a bare id, and is resolved to an
// embed URL HERE rather than trusted: an <iframe src> from slide text is the one
// place authored content becomes executable third-party code, so only YouTube
// and Vimeo ids are accepted and the URL is rebuilt from the id rather than
// passed through. YouTube uses youtube-nocookie.com so a lesson doesn't set
// tracking cookies on students. An unrecognised src renders nothing (the
// catalogue warning tells the author).
func renderVideo(args map[string]string) string {
	src := embedURL(args["src"])
	if src == "" {
		return ""
	}
	ratio := strings.TrimSpace(args["ratio"])
	if !ratioRe.MatchString(ratio) {
		ratio = "16:9"
	}
	title := args["title"]
	if title == "" {
		title = "Video"
	}
	return `<div class="lms-video" style="--lms-video-ratio:` +
		html.EscapeString(strings.Replace(ratio, ":", "/", 1)) + `">` +
		`<iframe src="` + html.EscapeString(src) + `" title="` + html.EscapeString(title) +
		`" loading="lazy" allowfullscreen ` +
		`referrerpolicy="strict-origin-when-cross-origin" ` +
		`allow="accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture"></iframe></div>`
}

// embedURL resolves an authored video reference to a privacy-preserving embed
// URL, or "" if it isn't a YouTube/Vimeo video we recognise. Only the extracted
// ID is reused; nothing from the authored URL reaches the output.
func embedURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// bare ids
	if ytIDRe.MatchString(s) {
		return "https://www.youtube-nocookie.com/embed/" + s
	}
	if vimeoIDRe.MatchString(s) {
		return "https://player.vimeo.com/video/" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	seg := strings.Split(strings.Trim(u.Path, "/"), "/")
	last := ""
	if len(seg) > 0 {
		last = seg[len(seg)-1]
	}
	switch host {
	case "youtube.com", "m.youtube.com", "youtube-nocookie.com":
		id := u.Query().Get("v") // /watch?v=ID
		if id == "" {
			id = last // /embed/ID, /shorts/ID, /live/ID
		}
		if ytIDRe.MatchString(id) {
			return "https://www.youtube-nocookie.com/embed/" + id
		}
	case "youtu.be":
		if ytIDRe.MatchString(last) {
			return "https://www.youtube-nocookie.com/embed/" + last
		}
	case "vimeo.com", "player.vimeo.com":
		if vimeoIDRe.MatchString(last) {
			return "https://player.vimeo.com/video/" + last
		}
	}
	return ""
}

// safeHTTPURL returns an escaped http(s) URL, or "" for anything else — so a
// `javascript:`/`data:` href authored (or imported) into a slide never reaches
// the rendered page.
func safeHTTPURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return html.EscapeString(u.String())
}
