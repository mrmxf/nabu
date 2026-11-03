package content

import (
	"strconv"
	"strings"
	"time"
)

// numbering resolves the course/lesson/slide display-number patterns and the
// readiness -> go_live publish gate. Both are small, deterministic string
// transforms shared across all three content levels.

// levelInfo is the resolved identity of one level (course, lesson or slide) used
// when rendering a numbering pattern.
type levelInfo struct {
	Number int
	Slug   string
	Title  string
}

// numberingCtx carries the identity of each level for pattern resolution. A
// lesson pattern only reads Course/Lesson; a slide pattern reads all three.
type numberingCtx struct {
	Course levelInfo
	Lesson levelInfo
	Slide  levelInfo
}

// resolveNumbering expands a pattern string. A run of the same level letter
// (C=course, L=lesson, S=slide, case-insensitive) selects a level and its length
// selects the format:
//
//	x1 -> number   x2 -> slug   x3 -> title   x4 -> number + title
//
// Any other characters are emitted literally. Example: "CC.L.S" with course
// slug "my-course", lesson #1, slide #3 -> "my-course.1.3".
func resolveNumbering(pattern string, ctx numberingCtx) string {
	runes := []rune(pattern)
	var b strings.Builder
	for i := 0; i < len(runes); {
		c := runes[i]
		if info, ok := levelForRune(c, ctx); ok {
			// consume the run of identical letters (case-insensitive).
			n := 1
			for i+n < len(runes) && toLevelKey(runes[i+n]) == toLevelKey(c) {
				n++
			}
			b.WriteString(formatLevel(info, n))
			i += n
			continue
		}
		b.WriteRune(c)
		i++
	}
	return b.String()
}

// toLevelKey folds a rune to its uppercase level letter, or 0 if not a level.
func toLevelKey(r rune) rune {
	switch r {
	case 'c', 'C':
		return 'C'
	case 'l', 'L':
		return 'L'
	case 's', 'S':
		return 'S'
	}
	return 0
}

// levelForRune returns the levelInfo a run of the given letter refers to.
func levelForRune(r rune, ctx numberingCtx) (levelInfo, bool) {
	switch toLevelKey(r) {
	case 'C':
		return ctx.Course, true
	case 'L':
		return ctx.Lesson, true
	case 'S':
		return ctx.Slide, true
	}
	return levelInfo{}, false
}

// formatLevel renders one level in the format chosen by the run length (capped
// at 4).
func formatLevel(info levelInfo, count int) string {
	if count > 4 {
		count = 4
	}
	switch count {
	case 1:
		return strconv.Itoa(info.Number)
	case 2:
		return info.Slug
	case 3:
		return info.Title
	default: // 4
		return strconv.Itoa(info.Number) + " " + info.Title
	}
}

// readinessLayout matches the "goes live at" timestamp form, e.g.
// 2026-09-01@09:00:00T+0100.
const readinessLayout = "2006-01-02@15:04:05T-0700"

// epoch is the fixed past date used as the go_live gate for `production` items
// so they are always live.
var epoch = time.Unix(0, 0).UTC()

// resolveReadiness normalises an authored readiness value and derives the
// go_live publish gate. An empty value defaults to DRAFT. A non-nil gate in the
// past means the item is live now.
//
// It returns the normalised readiness string to store verbatim plus the gate.
func resolveReadiness(raw string) (readiness string, goLive *time.Time) {
	readiness = strings.TrimSpace(raw)
	if readiness == "" {
		readiness = "DRAFT"
	}
	switch strings.ToLower(readiness) {
	case "draft", "deprecated":
		return readiness, nil
	case "production":
		return readiness, &epoch
	}
	// Otherwise treat it as a scheduled go-live timestamp.
	if t, err := time.Parse(readinessLayout, readiness); err == nil {
		u := t.UTC()
		return readiness, &u
	}
	// Unparseable readiness: keep it verbatim but never publish (safest).
	return readiness, nil
}
