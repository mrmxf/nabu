package students

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Apply reconciles roster records against the database under the given mode.
// scopeCourse (a course slug, optional) narrows delete-orphans; when empty the
// scope is the union of courses referenced by the file. It never panics — per
// row problems are collected into Summary.Errors and processing continues.
func Apply(app core.App, recs []UserRecord, mode Mode, scopeCourse string) (Summary, error) {
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return Summary{}, fmt.Errorf("find users collection: %w", err)
	}
	enrolCol, err := app.FindCollectionByNameOrId("enrolments")
	if err != nil {
		return Summary{}, fmt.Errorf("find enrolments collection: %w", err)
	}

	var sum Summary
	courseCache := map[string]string{} // slug -> id ("" = missing)
	processed := map[string]bool{}     // user ids touched by this import
	scopeSlugs := map[string]bool{}
	replace := mode == ModeOverwrite || mode == ModeDeleteOrphans

	for _, rec := range recs {
		// Superusers (dashboard admins) live in a different collection and take no
		// enrolment; route them through the same import so a roster is the single
		// path for every principal.
		if strings.EqualFold(rec.Role, RoleSuperuser) {
			applySuperuser(app, rec, &sum)
			continue
		}

		user, created, err := upsertUser(app, usersCol, rec)
		if err != nil {
			sum.Errors = append(sum.Errors, fmt.Sprintf("%s: %v", recKey(rec), err))
			continue
		}
		if created {
			sum.Created++
		} else {
			sum.Updated++
		}
		processed[user.Id] = true

		// resolve target course ids
		targetIDs := map[string]bool{}
		for _, slug := range rec.Courses {
			scopeSlugs[slug] = true
			id, err := courseID(app, slug, courseCache)
			if err != nil {
				sum.Errors = append(sum.Errors, fmt.Sprintf("%s: %v", recKey(rec), err))
				continue
			}
			targetIDs[id] = true
		}

		if err := reconcileEnrolments(app, enrolCol, user, targetIDs, replace, &sum); err != nil {
			sum.Errors = append(sum.Errors, fmt.Sprintf("%s: %v", recKey(rec), err))
		}
	}

	if mode == ModeDeleteOrphans {
		if scopeCourse != "" {
			scopeSlugs = map[string]bool{scopeCourse: true}
		}
		if err := deleteOrphans(app, scopeSlugs, courseCache, processed, &sum); err != nil {
			return sum, err
		}
	}
	return sum, nil
}

// upsertUser finds a user by uid (fallback email) and creates/updates it,
// generating a uid when neither the row nor an existing record has one.
func upsertUser(app core.App, col *core.Collection, rec UserRecord) (*core.Record, bool, error) {
	var user *core.Record
	if rec.Uid != "" {
		user, _ = app.FindFirstRecordByData("users", "uid", rec.Uid)
	}
	if user == nil && rec.Email != "" {
		user, _ = app.FindAuthRecordByEmail("users", rec.Email)
	}
	created := user == nil
	if created {
		if rec.Email == "" {
			return nil, false, fmt.Errorf("new user needs an email")
		}
		user = core.NewRecord(col)
	}

	uid := rec.Uid
	if uid == "" {
		if existing := user.GetString("uid"); existing != "" {
			uid = existing
		} else {
			uid = Slugify(localPart(rec.Email))
		}
	}
	user.Set("uid", uid)
	if rec.Email != "" {
		user.SetEmail(rec.Email)
	}
	if rec.Name != "" {
		user.Set("name", rec.Name)
	}
	if rec.Role != "" {
		user.Set("role", rec.Role)
	} else if created {
		user.Set("role", "student")
	}
	switch {
	case rec.Verified != nil:
		user.Set("verified", *rec.Verified)
	case created:
		user.Set("verified", true)
	}
	if pw := resolvePassword(rec); pw != "" {
		user.SetPassword(pw)
	}
	if err := app.Save(user); err != nil {
		return nil, false, fmt.Errorf("save user: %w", err)
	}
	return user, created, nil
}

// resolvePassword returns the password for a roster entry: the literal Password
// if set, otherwise the value of the PasswordEnv environment variable (injected
// by Infisical). Empty means "leave the existing password unchanged".
func resolvePassword(rec UserRecord) string {
	if rec.Password != "" {
		return rec.Password
	}
	if rec.PasswordEnv != "" {
		return os.Getenv(rec.PasswordEnv)
	}
	return ""
}

// applySuperuser upserts a `role: superuser` entry into the _superusers
// collection. The password comes only from PasswordEnv (never a literal); when
// that env var is unset a new superuser is skipped and an existing one is left
// untouched, so a box is never locked out of /_/. Per-entry problems are recorded
// on the summary rather than aborting the whole import.
func applySuperuser(app core.App, rec UserRecord, sum *Summary) {
	if rec.Email == "" {
		sum.Errors = append(sum.Errors, fmt.Sprintf("%s: superuser needs an email", recKey(rec)))
		return
	}
	if rec.PasswordEnv == "" {
		sum.Errors = append(sum.Errors, fmt.Sprintf("%s: superuser needs password_env (no literal passwords)", rec.Email))
		return
	}
	existing, _ := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, rec.Email)
	password := os.Getenv(rec.PasswordEnv)
	if password == "" {
		sum.Skipped++
		if existing != nil {
			slog.Warn("superuser password env unset; leaving existing account unchanged",
				"email", rec.Email, "password_env", rec.PasswordEnv)
		} else {
			slog.Warn("superuser password env unset; skipping (set it via Infisical, then reimport)",
				"email", rec.Email, "password_env", rec.PasswordEnv)
		}
		return
	}
	rec2 := existing
	if rec2 == nil {
		col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if err != nil {
			sum.Errors = append(sum.Errors, fmt.Sprintf("%s: find superusers collection: %v", rec.Email, err))
			return
		}
		rec2 = core.NewRecord(col)
		rec2.SetEmail(rec.Email)
	}
	rec2.SetPassword(password)
	if err := app.Save(rec2); err != nil {
		sum.Errors = append(sum.Errors, fmt.Sprintf("%s: save superuser: %v", rec.Email, err))
		return
	}
	sum.Superusers++
	slog.Info("imported superuser", "email", rec.Email, "password_env", rec.PasswordEnv)
}

// reconcileEnrolments ensures the user is enrolled in every target course. When
// replace is set, enrolments outside the target set are removed.
func reconcileEnrolments(app core.App, col *core.Collection, user *core.Record, targetIDs map[string]bool, replace bool, sum *Summary) error {
	existing, err := app.FindRecordsByFilter("enrolments", "user = {:u}", "", 0, 0,
		map[string]any{"u": user.Id})
	if err != nil {
		return fmt.Errorf("find enrolments: %w", err)
	}
	have := map[string]*core.Record{}
	for _, en := range existing {
		have[en.GetString("course")] = en
	}
	roleIn := roleInCourse(user.GetString("role"))

	for courseID := range targetIDs {
		if have[courseID] == nil {
			en := core.NewRecord(col)
			en.Set("user", user.Id)
			en.Set("course", courseID)
			en.Set("role_in_course", roleIn)
			if err := app.Save(en); err != nil {
				return fmt.Errorf("enrol: %w", err)
			}
			sum.Enrolled++
		}
	}
	if replace {
		for courseID, en := range have {
			if !targetIDs[courseID] {
				if err := app.Delete(en); err != nil {
					return fmt.Errorf("unenrol: %w", err)
				}
				sum.Unenrolled++
			}
		}
	}
	return nil
}

// deleteOrphans unenrols in-scope students absent from the import and deletes a
// student account once it has no remaining enrolments. Teachers/admins are
// never touched (superusers live in a different collection entirely).
func deleteOrphans(app core.App, scopeSlugs map[string]bool, courseCache map[string]string, processed map[string]bool, sum *Summary) error {
	for slug := range scopeSlugs {
		id, err := courseID(app, slug, courseCache)
		if err != nil {
			continue
		}
		enrolments, err := app.FindRecordsByFilter("enrolments", "course = {:c}", "", 0, 0,
			map[string]any{"c": id})
		if err != nil {
			return fmt.Errorf("find course enrolments: %w", err)
		}
		for _, en := range enrolments {
			userID := en.GetString("user")
			if processed[userID] {
				continue
			}
			user, err := app.FindRecordById("users", userID)
			if err != nil {
				continue
			}
			if user.GetString("role") != "student" {
				continue // never orphan-delete teachers/admins
			}
			if err := app.Delete(en); err != nil {
				return fmt.Errorf("unenrol orphan: %w", err)
			}
			sum.Unenrolled++
			remaining, err := app.FindRecordsByFilter("enrolments", "user = {:u}", "", 1, 0,
				map[string]any{"u": userID})
			if err != nil {
				return fmt.Errorf("check remaining enrolments: %w", err)
			}
			if len(remaining) == 0 {
				if err := app.Delete(user); err != nil {
					return fmt.Errorf("delete orphan student: %w", err)
				}
				sum.Deleted++
			}
		}
	}
	return nil
}

// courseID resolves a course slug to its record id, caching results. A missing
// course is an error the caller records per-row.
func courseID(app core.App, slug string, cache map[string]string) (string, error) {
	if id, ok := cache[slug]; ok {
		if id == "" {
			return "", fmt.Errorf("unknown course %q", slug)
		}
		return id, nil
	}
	rec, err := app.FindFirstRecordByData("courses", "slug", slug)
	if err != nil {
		cache[slug] = ""
		return "", fmt.Errorf("unknown course %q", slug)
	}
	cache[slug] = rec.Id
	return rec.Id, nil
}

func recKey(rec UserRecord) string {
	if rec.Uid != "" {
		return rec.Uid
	}
	return rec.Email
}

func localPart(email string) string {
	if i := strings.IndexByte(email, '@'); i >= 0 {
		return email[:i]
	}
	return email
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify lowercases a string and collapses every run of non-[a-z0-9] into a
// single dash, trimming leading/trailing dashes. It is the one place a
// human-typed name becomes a uid-safe handle — used for roster imports (the
// email local-part fallback) and for the classroom fast-login handle and
// session suffix (see internal/httpapi/classroom.go), so both produce the
// same uid for the same input.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
