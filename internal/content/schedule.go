package content

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LessonSchedule is one lesson's teaching schedule: the `allow-unscheduled` gate
// plus the named cohort sessions students may enroll in. It is authored in a
// standalone YAML pre-load (content/lesson-schedules.yaml) and seeded into the
// lesson_schedules / schedule_sessions collections (see internal/seed).
//
// Lesson is the canonical selector — a lesson slug. Seeding WARNs and skips an
// entry whose slug matches no lesson.
type LessonSchedule struct {
	Lesson           string            `yaml:"lesson"`
	AllowUnscheduled bool              `yaml:"allow-unscheduled"`
	Sessions         []ScheduleSession `yaml:"sessions"`
}

// ScheduleSession is one named cohort window. ID is a STABLE anchor: references
// (and the seed upsert) are by id, so Name is a mutable teacher-editable label
// that a rename never breaks. When the author omits ID it defaults to a slug of
// Name at load time.
type ScheduleSession struct {
	ID     string     `yaml:"id"`
	Name   string     `yaml:"name"`
	Enroll DateWindow `yaml:"enroll"`
	Start  DateWindow `yaml:"start"`
}

// DateWindow is a [From, To] range of datetimes, authored as strings so the
// seeder can parse them with the same types.ParseDateTime PocketBase uses.
type DateWindow struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// scheduleDoc is the top-level YAML document shape.
type scheduleDoc struct {
	LessonSchedules []LessonSchedule `yaml:"lesson-schedules"`
}

// nonSlug matches runs of characters that are not lowercase letters or digits,
// collapsed to a single hyphen when deriving a default session id.
var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// LoadSchedules reads the lesson-schedules YAML at path and returns the parsed
// schedules. A missing file is not an error: it returns an empty slice so a
// deployment without a schedule still seeds cleanly. Each session's ID is filled
// from a slug of its Name when the author left it empty.
func LoadSchedules(path string) ([]LessonSchedule, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read schedules %q: %w", path, err)
	}
	var doc scheduleDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse schedules %q: %w", path, err)
	}
	for i := range doc.LessonSchedules {
		for j := range doc.LessonSchedules[i].Sessions {
			s := &doc.LessonSchedules[i].Sessions[j]
			if strings.TrimSpace(s.ID) == "" {
				s.ID = slugID(s.Name)
			}
		}
	}
	return doc.LessonSchedules, nil
}

// slugID derives a stable id from a session name (lowercase, non-alphanumerics
// collapsed to hyphens, trimmed).
func slugID(s string) string {
	s = nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	return strings.Trim(s, "-")
}
