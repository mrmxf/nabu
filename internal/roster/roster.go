// Package roster is the canonical source for a course's people and each
// student's position within it. It is reused by the users export (--positions),
// the /api/courses/{slug}/roster endpoint, and the admin page, so "where a
// student is" has exactly one implementation.
package roster

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Position is where a student is within one lesson of a course.
type Position struct {
	Lesson       string `json:"lesson" yaml:"lesson"`
	CurrentSlide int    `json:"current_slide" yaml:"current_slide"` // 1-based, for display
	TotalSlides  int    `json:"total_slides" yaml:"total_slides"`
	Percent      int    `json:"percent" yaml:"percent"`
	Status       string `json:"status" yaml:"status"` // active | complete | not-started
}

// Member is one person in a course roster.
type Member struct {
	Uid       string     `json:"uid" yaml:"uid"`
	Email     string     `json:"email" yaml:"email"`
	Name      string     `json:"name" yaml:"name"`
	Role      string     `json:"role" yaml:"role"`
	Positions []Position `json:"positions,omitempty" yaml:"positions,omitempty"`
}

// Roster is a course's teachers and students, students carrying positions.
type Roster struct {
	Course   string   `json:"course" yaml:"course"`
	Teachers []Member `json:"teachers" yaml:"teachers"`
	Students []Member `json:"students" yaml:"students"`
}

// CourseRoster returns the roster for the course with the given slug.
func CourseRoster(app core.App, courseSlug string) (Roster, error) {
	course, err := app.FindFirstRecordByData("courses", "slug", courseSlug)
	if err != nil {
		return Roster{}, fmt.Errorf("find course %q: %w", courseSlug, err)
	}
	enrolments, err := app.FindRecordsByFilter("enrolments", "course = {:course}", "", 0, 0,
		map[string]any{"course": course.Id})
	if err != nil {
		return Roster{}, fmt.Errorf("find enrolments: %w", err)
	}

	out := Roster{Course: courseSlug}
	for _, en := range enrolments {
		user, err := app.FindRecordById("users", en.GetString("user"))
		if err != nil {
			continue // user vanished; skip
		}
		m := Member{
			Uid:   user.GetString("uid"),
			Email: user.Email(),
			Name:  user.GetString("name"),
			Role:  user.GetString("role"),
		}
		if en.GetString("role_in_course") == "teacher" {
			out.Teachers = append(out.Teachers, m)
			continue
		}
		positions, err := studentPositions(app, user.Id, course.Id)
		if err != nil {
			return Roster{}, err
		}
		m.Positions = positions
		out.Students = append(out.Students, m)
	}
	return out, nil
}

// StudentPositions returns one Position per session the student has in the given
// course (by record id). Exported for reuse by roster export.
func StudentPositions(app core.App, userID, courseID string) ([]Position, error) {
	return studentPositions(app, userID, courseID)
}

// studentPositions returns one Position per session the student has in the course.
func studentPositions(app core.App, userID, courseID string) ([]Position, error) {
	sessions, err := app.FindRecordsByFilter("sessions", "user = {:u} && course = {:c}", "lesson", 0, 0,
		map[string]any{"u": userID, "c": courseID})
	if err != nil {
		return nil, fmt.Errorf("find sessions: %w", err)
	}
	var positions []Position
	for _, s := range sessions {
		lesson, err := app.FindRecordById("lessons", s.GetString("lesson"))
		if err != nil {
			continue
		}
		total, err := liveSlideCount(app, lesson.Id)
		if err != nil {
			return nil, err
		}
		cur := s.GetInt("current_slide")
		status := s.GetString("status")
		if status == "" {
			status = "not-started"
		}
		percent := 0
		if total > 0 {
			percent = ((cur + 1) * 100) / total
			if percent > 100 {
				percent = 100
			}
		}
		positions = append(positions, Position{
			Lesson:       lesson.GetString("slug"),
			CurrentSlide: cur + 1,
			TotalSlides:  total,
			Percent:      percent,
			Status:       status,
		})
	}
	return positions, nil
}

// liveSlideCount counts the slides a student can actually see in a lesson —
// published (go_live in the past) and not broken — matching the collection's
// list rule so positions align with the player.
func liveSlideCount(app core.App, lessonID string) (int, error) {
	slides, err := app.FindRecordsByFilter("slides",
		"lesson = {:l} && go_live != '' && go_live <= {:now} && content != 'ERRORS' && content != 'CORRUPT'",
		"", 0, 0,
		map[string]any{"l": lessonID, "now": time.Now().UTC().Format("2006-01-02 15:04:05.000Z")})
	if err != nil {
		return 0, fmt.Errorf("count live slides: %w", err)
	}
	return len(slides), nil
}
