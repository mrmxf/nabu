package seed

import (
	"testing"

	"github.com/mrmxf/nabu/internal/content"
)

// TestReportContentProblems: seed fails on real breakage (ERRORS/CORRUPT) always,
// on warnings only under --strict, and passes when clean.
func TestReportContentProblems(t *testing.T) {
	corrupt := []content.Problem{{Path: "c/l", Status: content.StatusCorrupt, Issue: "frontmatter: bad"}}
	warns := []content.Problem{{Path: "c/l/s", Status: content.StatusWarnings, Issue: "broken xref"}}

	if err := reportContentProblems(corrupt, false); err == nil {
		t.Error("corrupt content must fail the seed")
	}
	if err := reportContentProblems(warns, false); err != nil {
		t.Errorf("warnings should not fail by default: %v", err)
	}
	if err := reportContentProblems(warns, true); err == nil {
		t.Error("warnings must fail under --strict")
	}
	if err := reportContentProblems(nil, true); err != nil {
		t.Errorf("clean content must pass: %v", err)
	}
}
