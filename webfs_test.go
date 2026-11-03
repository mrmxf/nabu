package nabu

import (
	"errors"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestLayeredFS(t *testing.T) {
	app := fstest.MapFS{"index.html": {Data: []byte("embedded app")}, "_app/x.js": {Data: []byte("js")}}
	disk := fstest.MapFS{"index.html": {Data: []byte("stale disk app")}, "content/c/l/pic.png": {Data: []byte("png")}}
	l := layeredFS{nil, app, disk}
	read := func(name string) string {
		f, err := l.Open(name)
		if err != nil {
			return "ERR " + err.Error()
		}
		defer func() { _ = f.Close() }()
		b, _ := io.ReadAll(f)
		return string(b)
	}
	if got := read("index.html"); got != "embedded app" {
		t.Errorf("index.html = %q: the embedded app must win over a stale disk copy", got)
	}
	if got := read("content/c/l/pic.png"); got != "png" {
		t.Errorf("seeded content must come from disk, got %q", got)
	}
	if _, err := l.Open("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file: err = %v, want ErrNotExist", err)
	}
	if _, err := (layeredFS{nil}).Open("x"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no layers: err = %v, want ErrNotExist", err)
	}
}
