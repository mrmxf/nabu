package nabu

import (
	"regexp"
	"testing"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/demo"
)

// TestVersion checks `--version` is wired to nabu's own build info, never
// PocketBase's "(untracked)".
func TestVersion(t *testing.T) {
	t.Cleanup(func() { embedfs.OverrideYAML, demo.Override = nil, nil })
	v := Version()
	if !regexp.MustCompile(`^\S+ \((dev|prod), \S+, \d{4}-\d{2}-\d{2}\)`).MatchString(v) {
		t.Fatalf("Version() = %q, want \"<tag> (<mode>, <hash>, <date>)\"", v)
	}
	if got := New(Options{}).RootCmd.Version; got != v {
		t.Fatalf("RootCmd.Version = %q, want %q", got, v)
	}
}
