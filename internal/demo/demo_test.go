package demo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrmxf/nabu/embedfs"
)

// useConfig points the config at file (or none) for one test.
func useConfig(t *testing.T, file string) {
	t.Helper()
	embedfs.OverrideFile = file
	t.Cleanup(func() { embedfs.OverrideFile = "" })
}

// TestRosterYAMLDefault checks that with no course.demo_roster set the embedded
// course-neutral roster is returned.
func TestRosterYAMLDefault(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "nabu.yaml")
	if err := os.WriteFile(cfg, []byte("demo_mode: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	useConfig(t, cfg)
	got, err := RosterYAML()
	if err != nil {
		t.Fatalf("RosterYAML: %v", err)
	}
	if !bytes.Equal(got, defaultRoster) {
		t.Fatal("expected the embedded default roster")
	}
}

// TestRosterYAMLOverride checks that course.demo_roster is read relative to the
// config file, not the working directory.
func TestRosterYAMLOverride(t *testing.T) {
	dir := t.TempDir()
	want := []byte("users: []\n")
	if err := os.MkdirAll(filepath.Join(dir, "rosters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rosters", "demo.yaml"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "nabu.yaml")
	if err := os.WriteFile(cfg, []byte("course:\n  demo_roster: rosters/demo.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	useConfig(t, cfg)

	got, err := RosterYAML()
	if err != nil {
		t.Fatalf("RosterYAML: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
