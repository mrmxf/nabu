//go:build webembed

package webapp

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// A webembed build with no webapp/dist fails to compile (go:embed matches
// nothing), so by here dist exists and Sub cannot fail.
func init() {
	if sub, err := fs.Sub(dist, "dist"); err == nil {
		FS = sub
	}
}
