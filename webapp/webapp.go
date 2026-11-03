// Package webapp carries Nabu's built web app (the SvelteKit SPA) inside the
// binary when it is built with `-tags webembed`.
//
// The app is a build output, never committed: `npm run build` in web/ writes
// it to webapp/dist, and release CI builds it before `go build -tags
// webembed`. Without the tag (tests, `go get` by a library course) FS is nil
// and Nabu serves pb_public/ from disk instead, or nabu.Options.Public.
package webapp

import "io/fs"

// FS is the embedded web app rooted at its index.html, or nil when this binary
// was built without the webembed tag.
var FS fs.FS
