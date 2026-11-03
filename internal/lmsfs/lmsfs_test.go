package lmsfs

import "testing"

// TestParseRef checks the lms:// grammar: only well-formed URIs with a known scope
// and a non-empty path parse.
func TestParseRef(t *testing.T) {
	cases := []struct {
		ref       string
		wantScope Scope
		wantPath  string
		wantOK    bool
	}{
		{"lms://resources/clip.mp4", ScopeResources, "clip.mp4", true},
		{"lms://resources/sub/deep.bin", ScopeResources, "sub/deep.bin", true},
		{"lms://group/s7g1/notes.md", ScopeGroup, "s7g1/notes.md", true},
		{"lms://student/s7g1-alice/draft.txt", ScopeStudent, "s7g1-alice/draft.txt", true},
		{"lms://resources/", "", "", false},   // empty path
		{"lms://resources", "", "", false},    // no path segment
		{"lms://mystery/x", "", "", false},    // unknown scope
		{"https://x/y.mp4", "", "", false},    // wrong scheme
		{"resources/clip.mp4", "", "", false}, // no scheme
	}
	for _, c := range cases {
		scope, p, ok := ParseRef(c.ref)
		if scope != c.wantScope || p != c.wantPath || ok != c.wantOK {
			t.Errorf("ParseRef(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.ref, scope, p, ok, c.wantScope, c.wantPath, c.wantOK)
		}
	}
}

// TestKeyForRef checks the key composition (and that traversal is cleaned away).
func TestKeyForRef(t *testing.T) {
	cases := []struct {
		prefix, rel string
		want        string
	}{
		{"rise/lesson1", "clip.mp4", "rise/lesson1/resources/clip.mp4"},
		{"", "clip.mp4", "resources/clip.mp4"},
		{"p", "../../etc/passwd", "etc/passwd"}, // .. resolved, can't escape
	}
	for _, c := range cases {
		if got := KeyForRef(c.prefix, ScopeResources, c.rel); got != c.want {
			t.Errorf("KeyForRef(%q, resources, %q) = %q, want %q", c.prefix, c.rel, got, c.want)
		}
	}
}

// TestClassify checks that a bucket key is mapped to the right scope + owner/group
// from the segment after the lesson prefix — the logic the reconcile sweep relies
// on to categorise objects it discovers in the bucket.
func TestClassify(t *testing.T) {
	cases := []struct {
		name        string
		prefix, key string
		wantScope   Scope
		wantOwner   string
		wantGroup   string
	}{
		{"resources", "rise/lesson1", "rise/lesson1/resources/clip.mp4", ScopeResources, "", ""},
		{"group", "rise/lesson1", "rise/lesson1/group/s7g1/notes.md", ScopeGroup, "", "s7g1"},
		{"student", "rise/lesson1", "rise/lesson1/student/s7g1-alice/draft.txt", ScopeStudent, "s7g1-alice", ""},
		{"empty prefix", "", "resources/x.png", ScopeResources, "", ""},
		{"trailing slash prefix", "rise/lesson1/", "rise/lesson1/resources/x.png", ScopeResources, "", ""},
		{"nested under resources", "p", "p/resources/sub/deep.bin", ScopeResources, "", ""},
		{"unknown subtree", "p", "p/mystery/x", "", "", ""},
		{"group without tag", "p", "p/group/", "", "", ""},
		{"student without uid", "p", "p/student", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			scope, owner, group := classify(c.prefix, c.key)
			if scope != c.wantScope || owner != c.wantOwner || group != c.wantGroup {
				t.Errorf("classify(%q, %q) = (%q, %q, %q), want (%q, %q, %q)",
					c.prefix, c.key, scope, owner, group, c.wantScope, c.wantOwner, c.wantGroup)
			}
		})
	}
}
