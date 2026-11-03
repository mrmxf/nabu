// Package assets carries the deploy blob: a gzip'd tar of pb_data and pb_public
// baked into the binary via go:embed — but ONLY under the `deploy` build tag.
//
// Normal builds (`go build ./...`, `scripts/build.sh`) exclude the embed:
// Blob is empty (see blob_stub.go), no embedded.tgz file is needed, and the dev
// binary stays lean — this is what keeps `nabu` from carrying a multi-MB archive.
// The deploy build (`go build -tags deploy`, driven by `nabu snapshot`) embeds
// embedded.tgz into Blob (see blob_deploy.go), and `install` extracts it on the
// target host. Because go:embed is resolved at compile time, the deploy flow is:
// snapshot (writes embedded.tgz, then builds nabu-deploy with -tags deploy).
package assets

// SourcePath is where `snapshot` writes the archive before the tagged rebuild
// bakes it in, relative to the repository root (the working directory the server
// commands run from). The file is transient: `snapshot` moves it to /tmp after
// baking, and it is gitignored, so it never lingers in the tree.
const SourcePath = "internal/assets/embedded.tgz"
