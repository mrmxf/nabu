package content

import (
	"path/filepath"
	"testing"

	"github.com/mrmxf/nabu/internal/source"
)

// TestDataMapperParseValidate covers t_mapper parsing and the seed-time source/root
// validation (index-only mapper; see docs/lesson-choice-values.md).
func TestDataMapperParseValidate(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	mustMkdir(t, dataDir)
	write(t, filepath.Join(dataDir, "lists.yaml"), "short:\n  cats:\n  - {name: Tom}\n")

	cls := filepath.Join(root, "tree", "c", "l")
	mustMkdir(t, cls)
	write(t, filepath.Join(root, "tree", "c", "_index.md"), "---\ntitle: C\n---\n")
	write(t, filepath.Join(cls, "_index.md"), "---\ntitle: L\n---\n")
	slide := func(mapper string) string {
		return "---\ntitle: S\nparams:\n  kind: presentation\n  t_mapper:\n" + mapper + "---\n{{< LMS t=\"tabulate\" from=\"mylist\" >}}\n"
	}

	load := func(mapper string) Slide {
		write(t, filepath.Join(cls, "10-s.md"), slide(mapper))
		courses, err := Load(source.Disk(filepath.Join(root, "tree")), NewGoldmarkRenderer(), Options{Data: source.Disk(dataDir)})
		if err != nil {
			t.Fatal(err)
		}
		return courses[0].Lessons[0].Slides[0]
	}

	// valid: parses + resolves clean.
	s := load("    - {id: mylist, from: topic, source: lists.yaml, root: short}\n")
	if len(s.DataMappers) != 1 || s.DataMappers[0].ID != "mylist" || s.DataMappers[0].Root != "short" {
		t.Fatalf("mapper not parsed: %+v", s.DataMappers)
	}
	if s.Content != StatusOK {
		t.Errorf("valid mapper should be OK, got %v (%s)", s.Content, s.Issue)
	}

	// bad source file → ERROR.
	if s := load("    - {id: mylist, from: topic, source: nope.yaml, root: short}\n"); s.Content != StatusErrors {
		t.Errorf("missing source should ERROR, got %v", s.Content)
	}
	// bad root key → ERROR.
	if s := load("    - {id: mylist, from: topic, source: lists.yaml, root: nope}\n"); s.Content != StatusErrors {
		t.Errorf("missing root should ERROR, got %v", s.Content)
	}
	// missing required field → ERROR.
	if s := load("    - {id: mylist, from: topic, source: lists.yaml}\n"); s.Content != StatusErrors {
		t.Errorf("missing root field should ERROR, got %v", s.Content)
	}
}
