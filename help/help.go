// Package help embeds Nabu's own help lessons — a course in the ordinary content
// format (help/using-nabu/<lesson>/<slide>.md) — so a released binary can seed
// them without the source tree. `nabu seed --help-content <dir>` loads a tree
// from disk instead, for editing the help in place.
package help

import "embed"

// FS holds the help course tree, rooted so that FS's top level is the course
// folder(s), exactly like a content directory.
//
//go:embed all:using-nabu
var FS embed.FS
