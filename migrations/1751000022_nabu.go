package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init creates the `nabu` collection: a single-row store of the running Nabu
// binary's identity — version, git hash, build date/flavour, OS/arch, and the
// buildinfo "long" string — plus reserved JSON slots for plugins (a future
// plugin+version list) and other runtime info. It is populated at boot from
// buildinfo (see internal/hooks/nabuinfo.go), NOT seeded, so it always reflects
// the binary that is actually serving. The `singleton` field (always "nabu") has
// a unique index so there is exactly one row, upserted on each boot.
//
// Reads are public (version info is low-sensitivity and the SPA shows it in the
// navbar tooltip and the `nabu-info` help widget); create/update/delete rules are
// left nil, so writes are server-only.
func init() {
	m.Register(func(app core.App) error {
		nabu := core.NewBaseCollection("nabu")
		nabu.Fields.Add(
			&core.TextField{Name: "singleton", Required: true}, // always "nabu" — one row
			&core.TextField{Name: "version"},                   // buildinfo Short, e.g. v0.1.6
			&core.TextField{Name: "tag"},
			&core.TextField{Name: "hash"}, // 40-char commit id
			&core.TextField{Name: "built"},
			&core.TextField{Name: "build"}, // "prod" | "dev"
			&core.TextField{Name: "os"},
			&core.TextField{Name: "arch"},
			&core.TextField{Name: "long"},    // buildinfo Long
			&core.JSONField{Name: "plugins"}, // reserved: [{name, version}, ...]
			&core.JSONField{Name: "info"},    // reserved: other runtime info
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		nabu.AddIndex("idx_nabu_singleton", true, "singleton", "")

		public := "" // "" = anyone (no auth); nil (unset) = server-only writes
		nabu.ListRule = &public
		nabu.ViewRule = &public

		if err := app.Save(nabu); err != nil {
			return fmt.Errorf("save nabu collection: %w", err)
		}
		return nil
	}, func(app core.App) error {
		nabu, err := app.FindCollectionByNameOrId("nabu")
		if err != nil {
			return nil // already gone
		}
		return app.Delete(nabu)
	})
}
