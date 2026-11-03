package students

import (
	"encoding/json"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"gopkg.in/yaml.v3"
)

// Bundle is the full user-data backup: enough to reproduce accounts and state in
// a clean database. Unlike a roster it carries secrets (password hashes) and is
// admin-only. See docs/student-management.md.
type Bundle struct {
	Superusers []BackupSuperuser `json:"superusers" yaml:"superusers"`
	Users      []BackupUser      `json:"users" yaml:"users"`
	Sessions   []BackupSession   `json:"sessions" yaml:"sessions"`
	Activity   []BackupActivity  `json:"activity" yaml:"activity"`
}

type BackupSuperuser struct {
	Email        string `json:"email" yaml:"email"`
	PasswordHash string `json:"password_hash" yaml:"password_hash"`
}

type BackupEnrolment struct {
	Course       string `json:"course" yaml:"course"`
	RoleInCourse string `json:"role_in_course" yaml:"role_in_course"`
	// Deprecated pre-rename keys, accepted when restoring older backups and
	// folded into Course/RoleInCourse by DecodeBundle. Never written.
	LegacyClass       string `json:"class,omitempty" yaml:"class,omitempty"`
	LegacyRoleInClass string `json:"role_in_class,omitempty" yaml:"role_in_class,omitempty"`
}

type BackupUser struct {
	Uid             string            `json:"uid" yaml:"uid"`
	Email           string            `json:"email" yaml:"email"`
	Name            string            `json:"name" yaml:"name"`
	Role            string            `json:"role" yaml:"role"`
	Verified        bool              `json:"verified" yaml:"verified"`
	EmailVisibility bool              `json:"email_visibility" yaml:"email_visibility"`
	PasswordHash    string            `json:"password_hash" yaml:"password_hash"`
	Enrolments      []BackupEnrolment `json:"enrolments,omitempty" yaml:"enrolments,omitempty"`
}

type BackupSession struct {
	UserUID      string         `json:"user_uid" yaml:"user_uid"`
	Course       string         `json:"course" yaml:"course"`
	LegacyClass  string         `json:"class,omitempty" yaml:"class,omitempty"` // deprecated; folded into Course
	Lesson       string         `json:"lesson" yaml:"lesson"`
	CurrentSlide int            `json:"current_slide" yaml:"current_slide"`
	Status       string         `json:"status" yaml:"status"`
	SessionName  string         `json:"session_name" yaml:"session_name"`
	GroupName    string         `json:"group_name" yaml:"group_name"`
	WorkState    map[string]any `json:"work_state,omitempty" yaml:"work_state,omitempty"`
}

type BackupActivity struct {
	UserUID string `json:"user_uid" yaml:"user_uid"`
	Event   string `json:"event" yaml:"event"`
	Created string `json:"created" yaml:"created"`
}

// Backup reads the whole user-data graph into a Bundle.
func Backup(app core.App) (Bundle, error) {
	var b Bundle

	supers, err := app.FindAllRecords(core.CollectionNameSuperusers)
	if err != nil {
		return b, fmt.Errorf("find superusers: %w", err)
	}
	for _, s := range supers {
		b.Superusers = append(b.Superusers, BackupSuperuser{
			Email:        s.Email(),
			PasswordHash: s.GetString("password:hash"),
		})
	}

	uidByID := map[string]string{} // user id -> uid
	slugByCourse := map[string]string{}
	slugByLesson := map[string]string{}

	users, err := app.FindAllRecords("users")
	if err != nil {
		return b, fmt.Errorf("find users: %w", err)
	}
	for _, u := range users {
		uidByID[u.Id] = u.GetString("uid")
		bu := BackupUser{
			Uid:             u.GetString("uid"),
			Email:           u.Email(),
			Name:            u.GetString("name"),
			Role:            u.GetString("role"),
			Verified:        u.GetBool("verified"),
			EmailVisibility: u.GetBool("emailVisibility"),
			PasswordHash:    u.GetString("password:hash"),
		}
		ens, err := app.FindRecordsByFilter("enrolments", "user = {:u}", "", 0, 0,
			map[string]any{"u": u.Id})
		if err != nil {
			return b, fmt.Errorf("find enrolments: %w", err)
		}
		for _, en := range ens {
			slug, err := courseSlugCached(app, en.GetString("course"), slugByCourse)
			if err != nil {
				continue
			}
			bu.Enrolments = append(bu.Enrolments, BackupEnrolment{
				Course:       slug,
				RoleInCourse: en.GetString("role_in_course"),
			})
		}
		b.Users = append(b.Users, bu)
	}

	sessions, err := app.FindAllRecords("sessions")
	if err != nil {
		return b, fmt.Errorf("find sessions: %w", err)
	}
	for _, s := range sessions {
		cslug, _ := courseSlugCached(app, s.GetString("course"), slugByCourse)
		lslug, _ := lessonSlugCached(app, s.GetString("lesson"), slugByLesson)
		var ws map[string]any
		if raw := s.GetString("work_state"); raw != "" && raw != "null" {
			_ = json.Unmarshal([]byte(raw), &ws)
		}
		b.Sessions = append(b.Sessions, BackupSession{
			UserUID:      uidByID[s.GetString("user")],
			Course:       cslug,
			Lesson:       lslug,
			CurrentSlide: s.GetInt("current_slide"),
			Status:       s.GetString("status"),
			SessionName:  s.GetString("session_name"),
			GroupName:    s.GetString("group_name"),
			WorkState:    ws,
		})
	}

	activity, err := app.FindAllRecords("activity")
	if err != nil {
		return b, fmt.Errorf("find activity: %w", err)
	}
	for _, a := range activity {
		b.Activity = append(b.Activity, BackupActivity{
			UserUID: uidByID[a.GetString("user")],
			Event:   a.GetString("event"),
			Created: a.GetString("created"),
		})
	}
	return b, nil
}

// EncodeBundle serialises a Bundle to YAML or JSON (CSV is not supported — the
// bundle is nested).
func EncodeBundle(b Bundle, f Format) ([]byte, error) {
	switch f {
	case FormatJSON:
		return json.MarshalIndent(b, "", "  ")
	case FormatYAML, "":
		return yaml.Marshal(b)
	default:
		return nil, fmt.Errorf("backup does not support format %q (use yaml or json)", f)
	}
}

// DecodeBundle parses a Bundle from YAML or JSON. Backups written before the
// class -> course rename are accepted: their `class` / `role_in_class` keys are
// folded into the canonical fields.
func DecodeBundle(data []byte, f Format) (Bundle, error) {
	var b Bundle
	var err error
	switch f {
	case FormatJSON:
		err = json.Unmarshal(data, &b)
	case FormatYAML, "":
		err = yaml.Unmarshal(data, &b)
	default:
		return b, fmt.Errorf("backup does not support format %q (use yaml or json)", f)
	}
	if err != nil {
		return b, err
	}
	for i := range b.Users {
		for j := range b.Users[i].Enrolments {
			en := &b.Users[i].Enrolments[j]
			if en.Course == "" {
				en.Course = en.LegacyClass
			}
			if en.RoleInCourse == "" {
				en.RoleInCourse = en.LegacyRoleInClass
			}
			en.LegacyClass, en.LegacyRoleInClass = "", ""
		}
	}
	for i := range b.Sessions {
		if b.Sessions[i].Course == "" {
			b.Sessions[i].Course = b.Sessions[i].LegacyClass
		}
		b.Sessions[i].LegacyClass = ""
	}
	return b, nil
}

func courseSlugCached(app core.App, id string, cache map[string]string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("empty course id")
	}
	if s, ok := cache[id]; ok {
		return s, nil
	}
	c, err := app.FindRecordById("courses", id)
	if err != nil {
		return "", err
	}
	cache[id] = c.GetString("slug")
	return cache[id], nil
}

func lessonSlugCached(app core.App, id string, cache map[string]string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("empty lesson id")
	}
	if s, ok := cache[id]; ok {
		return s, nil
	}
	l, err := app.FindRecordById("lessons", id)
	if err != nil {
		return "", err
	}
	cache[id] = l.GetString("slug")
	return cache[id], nil
}
