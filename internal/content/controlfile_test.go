package content

import "testing"

func TestIsControlFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"_index.md", true},
		{"_references.md", true},
		{"_course.css", true},
		{"_", true},
		{"010-welcome.md", false},
		{"index.md", false},
		{"", false},
		{"a_b.md", false}, // underscore must LEAD, not merely appear
	} {
		if got := IsControlFile(tc.name); got != tc.want {
			t.Errorf("IsControlFile(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsControlPath(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want bool
	}{
		// any segment counts, not just the basename — so a whole subtree can be
		// marked control by naming its directory.
		{"lesson/_references.md", true},
		{"_drafts/notes.md", true},
		{"a/_b/c/d.png", true},
		{"_x", true},

		{"lesson/010-welcome.md", false},
		{"resources/course.mp4", false},
		{"", false},
		{"/", false},

		// traversal and sloppy separators must not smuggle a control segment past
		// the check — the key is cleaned before it is split.
		{"a/./_x", true},
		{"a//_x", true},
		{"a/b/../_x", true},
		{"./_x", true},

		// ...and cleaning must not invent one either.
		{"a/b/../c.md", false},
	} {
		if got := IsControlPath(tc.key); got != tc.want {
			t.Errorf("IsControlPath(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}
