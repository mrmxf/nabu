package httpapi

import "testing"

// TestKeyUnder is the security boundary for the delete/multipart/confirm routes
// that accept a key from the client: a key must clean to something strictly under
// base+"/", so no traversal or sibling-subtree access can escape the caller's own
// area (e.g. one student reaching another student's files).
func TestKeyUnder(t *testing.T) {
	cases := []struct {
		base, clientKey string
		wantKey         string
		wantOK          bool
	}{
		{"p/student/s7g1-alice", "p/student/s7g1-alice/draft.txt", "p/student/s7g1-alice/draft.txt", true},
		{"p/student/s7g1-alice", "p/student/s7g1-alice/sub/x.bin", "p/student/s7g1-alice/sub/x.bin", true},
		{"p/group/s7g1", "p/group/s7g1/notes.md", "p/group/s7g1/notes.md", true},
		// escape attempts:
		{"p/student/s7g1-alice", "p/student/s7g1-bob/secret.txt", "", false},      // another student
		{"p/student/s7g1-alice", "p/resources/course.mp4", "", false},             // staff area
		{"p/student/s7g1-alice", "p/student/s7g1-alice/../s7g1-bob/x", "", false}, // traversal
		{"p/student/s7g1-alice", "p/student/s7g1-alice", "", false},               // the base itself, not under it
		{"p/student/s7g1-alice", "", "", false},                                   // empty
	}
	for _, c := range cases {
		key, ok := keyUnder(c.base, c.clientKey)
		if key != c.wantKey || ok != c.wantOK {
			t.Errorf("keyUnder(%q, %q) = (%q, %v), want (%q, %v)",
				c.base, c.clientKey, key, ok, c.wantKey, c.wantOK)
		}
	}
}
