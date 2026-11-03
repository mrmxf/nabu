package nabu

import "io/fs"

// layeredFS serves each name from the first layer that has it. Nil layers are
// skipped. The server stacks the web app (embedded, or Options.Public) over
// pb_public/ on disk: app files come from the build that matches this binary,
// while seeded lesson files (/content/…) and the assets pipeline output
// (/data/…) only ever exist on disk.
type layeredFS []fs.FS

// Open implements fs.FS. When no layer has name, the first layer's error is
// returned (fs.ErrNotExist for a plain miss).
func (l layeredFS) Open(name string) (fs.File, error) {
	var firstErr error
	for _, layer := range l {
		if layer == nil {
			continue
		}
		f, err := layer.Open(name)
		if err == nil {
			return f, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return nil, firstErr
}
