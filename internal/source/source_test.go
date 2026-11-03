package source

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbedWhere(t *testing.T) {
	tr := Embed(fstest.MapFS{"c/l/010-a.md": {Data: []byte("x")}}, "content")
	if got := tr.Where("c/l/010-a.md"); got != "embed://content/c/l/010-a.md" {
		t.Errorf("Where = %q", got)
	}
	if got := tr.Where("."); got != "embed://content" {
		t.Errorf("Where(.) = %q", got)
	}
	if !tr.Exists() {
		t.Error("embedded tree should exist")
	}
	if got := tr.Sub("c").Where("l"); got != "embed://content/c/l" {
		t.Errorf("Sub.Where = %q", got)
	}
}

func TestDiskLabelIsAbsolute(t *testing.T) {
	dir := t.TempDir()
	tr := Disk(dir)
	if !filepath.IsAbs(tr.Label) || !tr.Exists() {
		t.Fatalf("disk tree = %+v", tr)
	}
	if got := tr.Where("x/y.md"); !strings.HasSuffix(got, "/x/y.md") || !strings.HasPrefix(got, tr.Label) {
		t.Errorf("Where = %q", got)
	}
	if Disk(filepath.Join(dir, "missing")).Exists() {
		t.Error("missing dir should not exist")
	}
	if (Tree{}).Exists() || !(Tree{}).IsZero() {
		t.Error("zero tree should be unset")
	}
}
