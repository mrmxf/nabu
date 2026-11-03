package students

import (
	"reflect"
	"testing"
)

func boolp(b bool) *bool { return &b }

// sample roster used across codec round-trip tests.
func sample() []UserRecord {
	return []UserRecord{
		{Uid: "s7g1-alice", Email: "alice@nabu.example", Name: "Alice", Role: "student",
			Courses: []string{"rise-summer"}, Verified: boolp(true)},
		{Uid: "s7g1-bob", Email: "bob@nabu.example", Name: "Bob", Role: "teacher",
			Courses: []string{"rise-summer", "autumn-term"}, Verified: boolp(false)},
	}
}

func TestRosterRoundTrip(t *testing.T) {
	for _, f := range []Format{FormatCSV, FormatYAML, FormatJSON} {
		data, err := EncodeRoster(sample(), f)
		if err != nil {
			t.Fatalf("%s encode: %v", f, err)
		}
		got, err := DecodeRoster(data, f)
		if err != nil {
			t.Fatalf("%s decode: %v", f, err)
		}
		want := sample()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s round-trip mismatch:\n got  %+v\n want %+v", f, got, want)
		}
	}
}

func TestCSVCoursesSemicolonEncoding(t *testing.T) {
	data, err := EncodeRoster(sample(), FormatCSV)
	if err != nil {
		t.Fatal(err)
	}
	// Bob's two courses must be joined with ';' inside a single CSV cell.
	if want := "rise-summer;autumn-term"; !containsBytes(data, want) {
		t.Errorf("CSV missing %q; got:\n%s", want, data)
	}
}

func TestExportDropsPassword(t *testing.T) {
	recs := sample()
	recs[0].Password = "secret1234"
	for _, f := range []Format{FormatCSV, FormatYAML, FormatJSON} {
		data, err := EncodeRoster(recs, f)
		if err != nil {
			t.Fatal(err)
		}
		if containsBytes(data, "secret1234") {
			t.Errorf("%s export leaked password:\n%s", f, data)
		}
	}
}

func TestCSVVerifiedParsing(t *testing.T) {
	csv := "uid,email,name,role,courses,verified\nx,x@d,X,student,,true\ny,y@d,Y,student,,\n"
	recs, err := DecodeRoster([]byte(csv), FormatCSV)
	if err != nil {
		t.Fatal(err)
	}
	if recs[0].Verified == nil || !*recs[0].Verified {
		t.Errorf("row0 verified = %v, want true", recs[0].Verified)
	}
	if recs[1].Verified != nil {
		t.Errorf("row1 verified = %v, want nil (unset)", recs[1].Verified)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Alice.Smith":  "alice-smith",
		"bob+tag":      "bob-tag",
		"  Weird_Name": "weird-name",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func containsBytes(b []byte, s string) bool {
	return len(s) == 0 || indexOf(string(b), s) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
