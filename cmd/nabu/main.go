// Command nabu is the Nabu LMS engine: a thin wrapper over package nabu.
//
// Run it from a course repo (or point it at one with -C):
//
//	nabu seed      # load the course and Nabu's help into pb_data
//	nabu serve     # http://127.0.0.1:8090, dashboard at /_/
//	nabu stop      # graceful stop of a running server
//
// A course that wants a self-contained binary imports package nabu instead and
// passes its embedded content in nabu.Options.
package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/mrmxf/nabu"
)

func main() {
	// Automigrate only from source (`go run`): it writes Go migration files when
	// the schema is changed in /_/, and a released binary has no source tree.
	app := nabu.New(nabu.Options{Automigrate: isGoRun()})
	if err := app.Start(); err != nil {
		slog.Error("nabu exited", "err", err)
		os.Exit(1)
	}
}

// isGoRun reports whether this process was started by `go run`, whose binaries
// are built into the OS temp dir.
func isGoRun() bool {
	return strings.HasPrefix(os.Args[0], os.TempDir())
}
