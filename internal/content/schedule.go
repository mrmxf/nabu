package content

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mrmxf/nabu/internal/source"
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

// LoadSchedules reads the lesson-schedules YAML named name inside src and
// returns the parsed schedules. A missing file (or unset src) is not an error: it
// returns an empty slice so a deployment without a schedule still seeds cleanly.
// Each session's ID is filled from a slug of its Name when the author left it
// empty.
func LoadSchedules(src source.Tree, name string) ([]LessonSchedule, error) {
	if src.IsZero() {
		return nil, nil
	}
	data, err := fs.ReadFile(src.FS, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read schedules %s: %w", src.Where(name), err)
	}
	var doc scheduleDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse schedules %s: %w", src.Where(name), err)
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
