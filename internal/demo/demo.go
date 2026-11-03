// Package demo resolves the demo-mode roster so both the seeder and the running
// server share one source of truth for the demo accounts.
//
// The roster is either the course's own file, named by the config key
// `course.demo_roster`, or — when that is unset — the course-neutral default
// embedded here (demo_mode.yaml). It holds only the raw bytes and no decoding
// logic: callers decode it with internal/students. The seed applies these
// records when nabu-config.yaml `demo_mode: true`; the server reads the same list
// at serve time to detect demo accounts still registered while demo_mode is off
// (see internal/httpapi/demo.go), which the login page surfaces as a warning
// banner.
package demo

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/mrmxf/nabu/embedfs"
)

// defaultRoster is the embedded course-neutral demo roster, used when the config
// names no `course.demo_roster` file.
//
//go:embed demo_mode.yaml
var defaultRoster []byte

// RosterYAML returns the raw demo roster bytes, ready to hand to
// students.DecodeRoster with students.FormatYAML: the file named by
// `course.demo_roster` (resolved relative to the course's config file), or the
// embedded default when that key is unset.
func RosterYAML() ([]byte, error) {
	cfg, err := embedfs.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config for course.demo_roster: %w", err)
	}
	if cfg.Course.DemoRoster == "" {
		return defaultRoster, nil
	}
	path := embedfs.ResolvePath(cfg.Course.DemoRoster)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read course.demo_roster %q: %w", path, err)
	}
	return data, nil
}
