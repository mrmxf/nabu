package nabu_test

import (
	"testing/fstest"

	"github.com/mrmxf/nabu"
)

// A course binary bakes in its content, config and demo roster. In a real
// course the fs.FS comes from //go:embed (see the package doc).
func Example() {
	content := fstest.MapFS{
		"my-course/_index.md":                   {Data: []byte("---\ntitle: My course\nparams:\n  readiness: production\n---\n")},
		"my-course/first-lesson/_index.md":      {Data: []byte("---\ntitle: First lesson\nparams:\n  readiness: production\n---\n")},
		"my-course/first-lesson/010-welcome.md": {Data: []byte("---\ntitle: Welcome\nparams:\n  kind: intro\n  readiness: production\n---\nHello.\n")},
	}
	app := nabu.New(nabu.Options{
		Content:    content,
		ConfigYAML: []byte("pocketbase:\n  app:\n    name: My Course\n"),
	})
	// add your own commands, hooks or routes here, e.g. app.OnServe().BindFunc(...)
	_ = app.Start // then: if err := app.Start(); err != nil { os.Exit(1) }
}
