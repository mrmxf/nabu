package students

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Restore loads a Bundle into the database. It is intended for a clean DB but is
// idempotent by key (users by uid, superusers by email). Record ids are
// regenerated; all cross-references are rebuilt via uid/slug. Password hashes
// are written directly (not re-hashed) so users can log in immediately.
func Restore(app core.App, b Bundle) (Summary, error) {
	var sum Summary

	superCol, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return sum, fmt.Errorf("find superusers collection: %w", err)
	}
	for _, s := range b.Superusers {
		rec, _ := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, s.Email)
		if rec == nil {
			rec = core.NewRecord(superCol)
		}
		rec.SetEmail(s.Email)
		setPasswordHash(rec, s.PasswordHash)
		if err := app.Save(rec); err != nil {
			sum.Errors = append(sum.Errors, fmt.Sprintf("superuser %s: %v", s.Email, err))
		}
	}

	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return sum, fmt.Errorf("find users collection: %w", err)
	}
	enrolCol, err := app.FindCollectionByNameOrId("enrolments")
	if err != nil {
		return sum, fmt.Errorf("find enrolments collection: %w", err)
	}

	idByUID := map[string]string{}
	courseIDCache := map[string]string{}
	lessonIDCache := map[string]string{}

	for _, u := range b.Users {
		rec, _ := app.FindFirstRecordByData("users", "uid", u.Uid)
		created := rec == nil
		if created {
			rec = core.NewRecord(usersCol)
		}
		rec.Set("uid", u.Uid)
		rec.SetEmail(u.Email)
		rec.Set("name", u.Name)
		rec.Set("role", u.Role)
		rec.Set("verified", u.Verified)
		rec.Set("emailVisibility", u.EmailVisibility)
		setPasswordHash(rec, u.PasswordHash)
		if err := app.Save(rec); err != nil {
			sum.Errors = append(sum.Errors, fmt.Sprintf("user %s: %v", u.Uid, err))
			continue
		}
		idByUID[u.Uid] = rec.Id
		if created {
			sum.Created++
		} else {
			sum.Updated++
		}

		for _, en := range u.Enrolments {
			courseID, err := resolveID(app, "courses", en.Course, courseIDCache)
			if err != nil {
				sum.Errors = append(sum.Errors, fmt.Sprintf("user %s: %v", u.Uid, err))
				continue
			}
			// upsert by (user, course) so restore is idempotent.
			er, _ := app.FindFirstRecordByFilter("enrolments", "user = {:u} && course = {:c}",
				map[string]any{"u": rec.Id, "c": courseID})
			if er == nil {
				er = core.NewRecord(enrolCol)
			}
			er.Set("user", rec.Id)
			er.Set("course", courseID)
			er.Set("role_in_course", en.RoleInCourse)
			if err := app.Save(er); err != nil {
				sum.Errors = append(sum.Errors, fmt.Sprintf("enrol %s/%s: %v", u.Uid, en.Course, err))
				continue
			}
			sum.Enrolled++
		}
	}

	sessionsCol, err := app.FindCollectionByNameOrId("sessions")
	if err != nil {
		return sum, fmt.Errorf("find sessions collection: %w", err)
	}
	for _, s := range b.Sessions {
		userID := idByUID[s.UserUID]
		if userID == "" {
			continue
		}
		courseID, err := resolveID(app, "courses", s.Course, courseIDCache)
		if err != nil {
			continue
		}
		lessonID, err := resolveID(app, "lessons", s.Lesson, lessonIDCache)
		if err != nil {
			continue
		}
		rec := core.NewRecord(sessionsCol)
		rec.Set("user", userID)
		rec.Set("course", courseID)
		rec.Set("lesson", lessonID)
		rec.Set("current_slide", s.CurrentSlide)
		rec.Set("status", s.Status)
		rec.Set("session_name", s.SessionName)
		rec.Set("group_name", s.GroupName)
		if s.WorkState != nil {
			rec.Set("work_state", s.WorkState)
		}
		if err := app.Save(rec); err != nil {
			sum.Errors = append(sum.Errors, fmt.Sprintf("session %s: %v", s.UserUID, err))
		}
	}

	activityCol, err := app.FindCollectionByNameOrId("activity")
	if err != nil {
		return sum, fmt.Errorf("find activity collection: %w", err)
	}
	for _, a := range b.Activity {
		userID := idByUID[a.UserUID]
		if userID == "" {
			continue
		}
		rec := core.NewRecord(activityCol)
		rec.Set("user", userID)
		rec.Set("event", a.Event)
		if err := app.Save(rec); err != nil {
			sum.Errors = append(sum.Errors, fmt.Sprintf("activity %s: %v", a.UserUID, err))
		}
	}
	return sum, nil
}

// setPasswordHash writes a pre-computed bcrypt hash directly onto an auth record
// (bypassing SetPassword's re-hash) so a restored account keeps its password.
func setPasswordHash(rec *core.Record, hash string) {
	if hash == "" {
		return
	}
	rec.SetRaw("password", &core.PasswordFieldValue{Hash: hash})
}

func resolveID(app core.App, collection, slug string, cache map[string]string) (string, error) {
	if id, ok := cache[slug]; ok {
		return id, nil
	}
	rec, err := app.FindFirstRecordByData(collection, "slug", slug)
	if err != nil {
		return "", fmt.Errorf("unknown %s %q", collection, slug)
	}
	cache[slug] = rec.Id
	return rec.Id, nil
}
