package students

import (
	"reflect"
	"strings"
	"testing"
)

// TestDecodeRosterLegacyClasses checks that rosters written before the class ->
// course rename still import: the `classes` key (YAML, JSON) and CSV column are
// folded into Courses.
func TestDecodeRosterLegacyClasses(t *testing.T) {
	want := []string{"maths", "art"}
	cases := map[Format]string{
		FormatYAML: "users:\n  - uid: u1\n    email: alice@nabu.example\n    classes: [maths, art]\n",
		FormatJSON: `{"users":[{"uid":"u1","email":"alice@nabu.example","classes":["maths","art"]}]}`,
		FormatCSV:  "uid,email,name,role,classes,password,verified\nu1,alice@nabu.example,Alice,student,maths;art,,\n",
	}
	for f, data := range cases {
		recs, err := DecodeRoster([]byte(data), f)
		if err != nil {
			t.Fatalf("%s: decode: %v", f, err)
		}
		if len(recs) != 1 || !reflect.DeepEqual(recs[0].Courses, want) {
			t.Errorf("%s: courses = %v, want %v", f, recs[0].Courses, want)
		}
		if recs[0].LegacyClasses != nil {
			t.Errorf("%s: legacy classes not cleared: %v", f, recs[0].LegacyClasses)
		}
	}
}

// TestEncodeRosterWritesCourses checks the canonical key is the only one written.
func TestEncodeRosterWritesCourses(t *testing.T) {
	out, err := EncodeRoster([]UserRecord{{Uid: "u1", Email: "alice@nabu.example", Courses: []string{"maths"}}}, FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); !strings.Contains(got, "courses:") || strings.Contains(got, "classes:") {
		t.Errorf("encoded roster = %q, want courses: and no classes:", got)
	}
}

// TestDecodeBundleLegacyClass checks that pre-rename backups restore: `class`
// and `role_in_class` are folded into the canonical fields.
func TestDecodeBundleLegacyClass(t *testing.T) {
	data := "users:\n  - uid: u1\n    enrolments:\n      - class: maths\n        role_in_class: student\n" +
		"sessions:\n  - user_uid: u1\n    class: maths\n    lesson: one\n"
	b, err := DecodeBundle([]byte(data), FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	en := b.Users[0].Enrolments[0]
	if en.Course != "maths" || en.RoleInCourse != "student" || en.LegacyClass != "" {
		t.Errorf("enrolment = %+v", en)
	}
	if s := b.Sessions[0]; s.Course != "maths" || s.LegacyClass != "" {
		t.Errorf("session = %+v", s)
	}
}
