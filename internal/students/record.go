// Package students implements user roster import/export (CSV/YAML/JSON) with
// merge/overwrite/delete-orphans semantics, plus full user-data backup/restore.
// It is the shared core behind both the `users` CLI command and the custom
// /api/users/* HTTP routes. See docs/student-management.md.
package students

import "github.com/mrmxf/nabu/internal/roster"

// Mode selects how an import reconciles the file against the database.
type Mode string

const (
	ModeMerge         Mode = "merge"          // add new, update listed, union courses; others untouched
	ModeOverwrite     Mode = "overwrite"      // update listed, replace their course set; others untouched
	ModeDeleteOrphans Mode = "delete-orphans" // overwrite + unenrol/delete in-scope students absent from the file
)

// Format is a serialisation format for a roster file.
type Format string

const (
	FormatCSV  Format = "csv"
	FormatYAML Format = "yaml"
	FormatJSON Format = "json"
)

// RoleSuperuser marks a roster entry as a PocketBase dashboard admin. Such
// entries are applied to the _superusers collection (not `users`) and must carry
// a PasswordEnv rather than a literal Password.
const RoleSuperuser = "superuser"

// UserRecord is the canonical roster row/object. `Uid` is the stable match key;
// `Email` is mutable. `Password` / `PasswordEnv` are import-only (never emitted
// on export); `Positions` is export-only (omitted on import).
type UserRecord struct {
	Uid   string `json:"uid" yaml:"uid"`
	Email string `json:"email" yaml:"email"`
	Name  string `json:"name" yaml:"name"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
	// Courses enrols the user into these course slugs. Ignored for superusers.
	Courses []string `json:"courses,omitempty" yaml:"courses,omitempty"`
	// LegacyClasses is the deprecated pre-rename spelling of Courses (`classes:`),
	// still accepted on input so older rosters and exports import unchanged. It is
	// folded into Courses on decode and never written back out.
	LegacyClasses []string `json:"classes,omitempty" yaml:"classes,omitempty"`
	// Password is a literal (import-only). Prefer PasswordEnv so no secret is
	// committed; required (as PasswordEnv) for a superuser.
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	// PasswordEnv names an environment variable (injected by Infisical) that holds
	// the password. Used when Password is empty; it is the only accepted form for a
	// superuser. Not a secret itself, so it is safe to commit.
	PasswordEnv string            `json:"password_env,omitempty" yaml:"password_env,omitempty"`
	Verified    *bool             `json:"verified,omitempty" yaml:"verified,omitempty"`
	Positions   []roster.Position `json:"positions,omitempty" yaml:"positions,omitempty"`
}

// Summary reports what an import/apply did.
type Summary struct {
	Created    int `json:"created" yaml:"created"`
	Updated    int `json:"updated" yaml:"updated"`
	Enrolled   int `json:"enrolled" yaml:"enrolled"`
	Unenrolled int `json:"unenrolled" yaml:"unenrolled"`
	Deleted    int `json:"deleted" yaml:"deleted"`
	// Superusers counts _superusers upserted from `role: superuser` entries.
	Superusers int `json:"superusers,omitempty" yaml:"superusers,omitempty"`
	// Skipped counts entries applied to no effect (e.g. a superuser whose password
	// env var is unset), with a human note collected per entry.
	Skipped int      `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Errors  []string `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// roleInCourse maps a global role to the per-course role stored on an enrolment.
func roleInCourse(role string) string {
	if role == "teacher" {
		return "teacher"
	}
	return "student"
}
