// Package source names where Nabu reads course files from: a file tree plus a
// label for logs and messages.
//
// A tree on disk is labelled with its absolute path; a tree baked into the
// binary (nabu.Options) is labelled embed://<name>, e.g. embed://content. The
// label is for people, never a path to open: every read goes through Tree.FS
// with slash-separated paths relative to the tree root, so
//
//	tree.Where("weather/weatherface/010-intro.md")
//
// reads as /srv/course/content/weather/… for a disk tree and as
// embed://content/weather/… for an embedded one. The embed:// form follows the
// lms:// scheme convention used for lesson resources.
package source

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// EmbedScheme prefixes the label of a tree baked into the binary.
const EmbedScheme = "embed://"

// Tree is a readable file tree and the label that says where it came from. The
// zero Tree has no FS and means "not configured".
type Tree struct {
	FS    fs.FS
	Label string
}

// Disk returns the tree rooted at dir on the local filesystem, labelled with its
// absolute path. A missing dir is not an error here; see Exists.
func Disk(dir string) Tree {
	label := dir
	if abs, err := filepath.Abs(dir); err == nil {
		label = abs
	}
	return Tree{FS: os.DirFS(dir), Label: label}
}

// Embed returns fsys as a tree labelled embed://name.
func Embed(fsys fs.FS, name string) Tree {
	return Tree{FS: fsys, Label: EmbedScheme + name}
}

// IsZero reports whether the tree is unset.
func (t Tree) IsZero() bool { return t.FS == nil }

// Exists reports whether the tree's root can be read.
func (t Tree) Exists() bool {
	if t.FS == nil {
		return false
	}
	st, err := fs.Stat(t.FS, ".")
	return err == nil && st.IsDir()
}

// Where names rel (a slash path inside the tree) for a person: the label joined
// with rel. Where(".") is the label itself.
func (t Tree) Where(rel string) string {
	rel = path.Clean(rel)
	if rel == "." || rel == "" {
		return t.Label
	}
	return strings.TrimRight(t.Label, "/") + "/" + rel
}

// Sub returns the subtree at rel, labelled accordingly. A missing subdirectory
// yields a tree whose reads fail, like a missing Disk dir.
func (t Tree) Sub(rel string) Tree {
	if t.FS == nil {
		return Tree{}
	}
	sub, err := fs.Sub(t.FS, rel)
	if err != nil {
		return Tree{}
	}
	return Tree{FS: sub, Label: t.Where(rel)}
}

// CopyTo copies the file rel inside the tree to dst on disk, creating dst's
// parent directories. Errors name the source by Where.
func (t Tree) CopyTo(rel, dst string) error {
	if t.FS == nil {
		return fmt.Errorf("copy %s: no source tree", rel)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("copy %s: %w", t.Where(rel), err)
	}
	in, err := t.FS.Open(rel)
	if err != nil {
		return fmt.Errorf("copy %s: %w", t.Where(rel), err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("copy %s: %w", t.Where(rel), err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %s: %w", t.Where(rel), err)
	}
	return out.Close()
}
