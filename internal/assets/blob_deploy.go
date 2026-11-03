//go:build deploy

package assets

import _ "embed"

// Blob is the embedded gzip'd tar produced by `snapshot`, baked in only under the
// `deploy` build tag (`go build -tags deploy`). It contains files under the
// pb_data/ and pb_public/ prefixes; `install` extracts them on the target host.
//
//go:embed embedded.tgz
var Blob []byte
