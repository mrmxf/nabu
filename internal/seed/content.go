package seed

// This file holds the non-lesson demo data seeded for Phase 2: the global
// config rows. The lesson content itself no longer lives in Go — it is authored
// as an on-disk Hugo-style tree under content/ (repo root) and ingested
// by internal/content (see seed.go). Demo student accounts live in seed.go.

// settingSeed is one row in the global settings key/value table.
type settingSeed struct {
	Key   string
	Value string
}

// defaultSettings pre-populates the global config table. Logo values are
// repository paths under web/static; the UI serves them by basename (e.g.
// mi-mrmxf-c.svg -> /mi-mrmxf-c.svg). Templates use {{.YYYY}} {{.MM}} {{.DD}}
// {{.hh}} {{.mm}} {{.ss}} {{.USER}} placeholders. The numbering keys drive the
// course/lesson/slide display numbers (see docs/user/config.md); renderer is
// the ingest default (overridable by `seed --renderer`). app_name is the name
// shown in the navbar and page title, from pocketbase.app.name.
var defaultSettings = []settingSeed{
	{"app_name", "Nabu"},
	{"site_logo", "web/static/mi-mrmxf-c.svg"},
	{"course_logo", "web/static/logo-nabu.svg"},
	{"lesson_logo", "web/static/mi-bot-c.svg"},
	{"auto_session_template", "{{.YYYY}}-{{.MM}}-{{.DD}} @ {{.hh}}:{{.mm}}:{{.ss}}"},
	{"auto_group_template", "{{.USER}}-group"},
	{"course_numbering", "CC"},
	{"lesson_numbering", "CC.L"},
	{"slide_numbering", "CC.L.S"},
	{"renderer", "goldmark"},
	// Outbound "review us" target for the help ShareBar. The repo is on GitLab, so
	// set this to whatever GitHub review/stars URL you want students to land on.
	{"github_review_url", "https://github.com/mrmxf/nabu"},
	// Whether the /signup page requires an email address. Sourced from
	// nabu-config.yaml `signup.require_email` (see settingsWithConfig); "false" =
	// email is optional. Stored as a string ("true"/"false") like every setting.
	{"signup_require_email", "false"},
	// Whether demo accounts are spawned. Sourced from nabu-config.yaml `demo_mode`
	// (see settingsWithConfig). The login page reads this (with /api/demo/status)
	// to warn when demo accounts linger while demo mode is off.
	{"demo_mode", "true"},
	// Whether /login offers the name-only classroom sign-in. Sourced from
	// nabu-config.yaml `classroom_fast_login` (see settingsWithConfig). "true"
	// makes /login show a single name box while a Session is enrollable; the
	// server enforces the same flag (see internal/httpapi/classroom.go). Default
	// off — it is a password-less path and belongs to a supervised lesson only.
	{"classroom_fast_login", "false"},
	// Whether the image agents' "make an image" (Stable Diffusion) surface is exposed.
	// Sourced from nabu-config.yaml `ai.stable-diffusion.enabled` (see
	// settingsWithConfig). "false" hides every image surface in the SPA and the
	// job API refuses image submits. Default off — SD and Ollama contend for VRAM
	// on a single card (see docs/image-server-setup.md §0).
	{"images_enabled", "false"},
}
