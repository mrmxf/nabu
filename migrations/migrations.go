// Package migrations holds the PocketBase schema migrations for the Secret
// Agents control plane. Each migration registers itself via m.Register in an
// init function and is applied automatically on serve (Automigrate).
//
// Collections are defined here (not via the dashboard) so the schema is
// versioned in Go. See docs/phase2.md for the data model.
package migrations
