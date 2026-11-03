package students

import (
	"fmt"
	"sort"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/roster"
)

// ExportFilter narrows a roster export. TeacherScope, when non-nil, restricts
// output to those course slugs (a teacher may only export their own courses); nil
// means unrestricted (admin).
type ExportFilter struct {
	Role         string
	Course       string
	TeacherScope []string
}

// Export builds canonical roster records for the users matching the filter,
// optionally attaching per-course positions for students.
func Export(app core.App, filter ExportFilter, positions bool) ([]UserRecord, error) {
	courseSlugs, err := resolveScope(app, filter)
	if err != nil {
		return nil, err
	}

	userIDs := map[string]bool{}
	if courseSlugs == nil { // unrestricted: every user
		all, err := app.FindAllRecords("users")
		if err != nil {
			return nil, fmt.Errorf("find users: %w", err)
		}
		for _, u := range all {
			userIDs[u.Id] = true
		}
	} else {
		for _, slug := range courseSlugs {
			c, err := app.FindFirstRecordByData("courses", "slug", slug)
			if err != nil {
				return nil, fmt.Errorf("unknown course %q", slug)
			}
			ens, err := app.FindRecordsByFilter("enrolments", "course = {:c}", "", 0, 0,
				map[string]any{"c": c.Id})
			if err != nil {
				return nil, fmt.Errorf("find enrolments: %w", err)
			}
			for _, en := range ens {
				userIDs[en.GetString("user")] = true
			}
		}
	}

	scopeSet := sliceSet(filter.TeacherScope) // nil => unrestricted
	slugCache := map[string]string{}          // course id -> slug

	var out []UserRecord
	for id := range userIDs {
		u, err := app.FindRecordById("users", id)
		if err != nil {
			continue
		}
		if filter.Role != "" && u.GetString("role") != filter.Role {
			continue
		}
		v := u.GetBool("verified")
		rec := UserRecord{
			Uid:      u.GetString("uid"),
			Email:    u.Email(),
			Name:     u.GetString("name"),
			Role:     u.GetString("role"),
			Verified: &v,
		}
		ens, err := app.FindRecordsByFilter("enrolments", "user = {:u}", "", 0, 0,
			map[string]any{"u": id})
		if err != nil {
			return nil, fmt.Errorf("find enrolments: %w", err)
		}
		for _, en := range ens {
			courseID := en.GetString("course")
			slug, err := courseSlug(app, courseID, slugCache)
			if err != nil {
				continue
			}
			if scopeSet != nil && !scopeSet[slug] {
				continue
			}
			rec.Courses = append(rec.Courses, slug)
			if positions && en.GetString("role_in_course") != "teacher" {
				ps, err := roster.StudentPositions(app, id, courseID)
				if err != nil {
					return nil, err
				}
				rec.Positions = append(rec.Positions, ps...)
			}
		}
		sort.Strings(rec.Courses)
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Uid != out[j].Uid {
			return out[i].Uid < out[j].Uid
		}
		return out[i].Email < out[j].Email
	})
	return out, nil
}

// resolveScope returns the course slugs to export from, or nil for "all users".
// A teacher's requested course must lie within their scope.
func resolveScope(app core.App, filter ExportFilter) ([]string, error) {
	if filter.Course != "" {
		if filter.TeacherScope != nil && !sliceSet(filter.TeacherScope)[filter.Course] {
			return nil, fmt.Errorf("course %q not in scope", filter.Course)
		}
		return []string{filter.Course}, nil
	}
	if filter.TeacherScope != nil {
		return filter.TeacherScope, nil
	}
	return nil, nil
}

func courseSlug(app core.App, id string, cache map[string]string) (string, error) {
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

func sliceSet(ss []string) map[string]bool {
	if ss == nil {
		return nil
	}
	m := map[string]bool{}
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// TeacherCourses returns the course slugs a teacher is enrolled to teach — the
// scope for teacher export/roster requests.
func TeacherCourses(app core.App, userID string) ([]string, error) {
	ens, err := app.FindRecordsByFilter("enrolments", "user = {:u} && role_in_course = 'teacher'", "", 0, 0,
		map[string]any{"u": userID})
	if err != nil {
		return nil, fmt.Errorf("find teacher enrolments: %w", err)
	}
	cache := map[string]string{}
	var out []string
	for _, en := range ens {
		if slug, err := courseSlug(app, en.GetString("course"), cache); err == nil {
			out = append(out, slug)
		}
	}
	sort.Strings(out)
	return out, nil
}
