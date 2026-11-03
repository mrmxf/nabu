package content

import (
	"path"
	"strings"
)

// A leading underscore marks a CONTROL FILE: something that configures or
// supports the content rather than being content itself. Nabu has always relied
// on this informally for _index.md; this makes it a stated, uniformly enforced
// rule, so authors can add supporting files (_course.css, _lesson.css,
// _course.yaml, _references.md, _snippets.md…) beside their slides without those
// files turning into slides or showing up in a student's file list.
//
// The rule, by audience:
//
//	student  never listed, never indexed, never navigable. The file's EFFECTS
//	         are visible (a stylesheet applies, an include splices) but the file
//	         itself is not enumerable.
//	teacher  listed, but shown in a visually distinct style so it reads as
//	         configuration rather than a lesson page. Teachers edit these.
//	admin    no restriction.
//
// Note the deliberate asymmetry in how that is enforced: hiding a control file
// from a student is a boundary, so it happens SERVER-SIDE, before the list is
// serialised. Styling one for a teacher is presentation, so it happens in the
// client. A student must not be able to enumerate control files by reading the
// network response.
//
// Every enforcement point routes through the two helpers below rather than
// open-coding a "_" check, so the rule cannot drift between surfaces.

// IsControlFile reports whether a single path SEGMENT names a control file.
// It takes a bare name (no separators); use IsControlPath for a full key.
func IsControlFile(name string) bool {
	return strings.HasPrefix(name, "_")
}

// IsControlPath reports whether any segment of a slash-separated path is a
// control file or a control directory. Checking every segment (not just the
// basename) means a whole subtree can be marked control by naming its directory
// with a leading underscore — "_drafts/notes.md" is control, the same as
// "_notes.md" — and a student can't reach into one by asking for a deep key.
//
// Keys are cleaned first so "a/./_x", "a//_x" and "a/b/../_x" cannot smuggle a
// control segment past the check.
func IsControlPath(key string) bool {
	key = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(key)), "/")
	if key == "" || key == "." {
		return false
	}
	for _, seg := range strings.Split(key, "/") {
		if IsControlFile(seg) {
			return true
		}
	}
	return false
}
