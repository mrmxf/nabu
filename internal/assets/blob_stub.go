//go:build !deploy

package assets

// Blob is empty in normal (non-deploy) builds: no embedded.tgz is referenced, so
// the tree builds from a clean checkout with no snapshot present and the dev
// binary carries no archive. Only `go build -tags deploy` bakes the snapshot in
// (see blob_deploy.go). `install` on such a binary reports "no embedded content".
var Blob []byte
