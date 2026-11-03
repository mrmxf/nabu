package hooks

import (
	"log/slog"

	"github.com/mrmxf/util/buildinfo"
	"github.com/pocketbase/pocketbase/core"
)

// BindNabuInfo upserts the single `nabu` row at serve start with the running
// binary's identity from buildinfo — version, git hash, build date/flavour and
// OS/arch, injected via -ldflags at build time (see nabu/scripts/buildinfo.sh).
// Populating on boot (rather than at seed) means the row always reflects the
// binary that is actually serving: a `nabu-deploy` overwrites whatever value a
// snapshot baked in with its own. Best-effort — a missing collection (pre-migration)
// or a save error is warned, never fatal.
func BindNabuInfo(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := upsertNabuInfo(e.App); err != nil {
			slog.Warn("could not record nabu build info", "err", err)
		}
		return e.Next()
	})
}

func upsertNabuInfo(app core.App) error {
	col, err := app.FindCollectionByNameOrId("nabu")
	if err != nil {
		return nil // collection not present yet (pre-migration) — skip quietly
	}
	rec, _ := app.FindFirstRecordByData("nabu", "singleton", "nabu")
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("singleton", "nabu")
	}

	bi := buildinfo.Info()
	build := "dev"
	if buildinfo.IsProductionBuild {
		build = "prod"
	}
	rec.Set("version", bi.Short)
	rec.Set("tag", bi.Tag)
	rec.Set("hash", bi.CommitId)
	rec.Set("built", bi.Date)
	rec.Set("build", build)
	rec.Set("os", bi.OS)
	rec.Set("arch", bi.ARCH)
	rec.Set("long", bi.Long)
	// `plugins` and `info` are reserved for a future feature — left untouched.

	if err := app.Save(rec); err != nil {
		return err
	}
	slog.Info("nabu build info", "version", bi.Short, "build", build, "hash", bi.CommitId)
	return nil
}
