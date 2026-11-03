// Command nabu is the Nabu LMS engine for hands-on, agent-driven courses. It
// embeds PocketBase (auth, database, realtime, superuser dashboard)
// as a Go library and serves the SvelteKit single-page app built into
// pb_public.
//
// Run from the repository root:
//
//	go run ./cmd/nabu serve     # start Nabu (dashboard at /_/)
//	go run ./cmd/nabu seed      # populate demo content (see internal/seed)
//	go run ./cmd/nabu stop      # stop a running Nabu (see internal/serverctl)
//	go run ./cmd/nabu snapshot  # capture pb_data + pb_public into the deploy blob
//	go run ./cmd/nabu install   # extract the embedded deploy blob to storage
//
// See CLAUDE.md and docs/phase2.md for the wider design.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mrmxf/util/slogger"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/cmd"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/admin"
	"github.com/mrmxf/nabu/internal/deploy"
	"github.com/mrmxf/nabu/internal/hooks"
	"github.com/mrmxf/nabu/internal/httpapi"
	"github.com/mrmxf/nabu/internal/seed"
	"github.com/mrmxf/nabu/internal/serverctl"
	"github.com/mrmxf/nabu/internal/students"

	// register schema migrations for side effects
	_ "github.com/mrmxf/nabu/migrations"
)

// serverDir is the package source directory (migrations, help), resolved relative
// to the working directory (the repository root in dev).
const serverDir = "."

// defaultDataDir is where pb_data lives when --dir is not given: `pb_data` in the
// current working directory — the folder Nabu was launched from. So a deploy
// binary in /opt/nabu defaults to /opt/nabu/pb_data, and a dev
// run from the repo root uses ./pb_data. pb_public is always its sibling
// (see the OnServe handler), so the built SPA lives at ./pb_public too.
const defaultDataDir = "pb_data"

func main() {
	// slogger installs a pretty console handler as the default slog logger.
	// This is the primary user-facing progress stream (see CLAUDE.md logging).
	slogger.UsePrettyLogger(slog.LevelInfo)

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: defaultDataDir,
	})

	// Automigrate captures schema changes made via the dashboard into Go
	// migration files under migrations/, keeping code and DB in sync. Only when
	// running from source (`go run`): a released binary has no source tree to
	// write migrations into.
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: isGoRun(),
		Dir:         serverDir + "/migrations",
	})

	// Global `--config`/`-c`: a partial YAML file on the local FS that overrides
	// the embedded nabu-config.yaml defaults (deep-merged via kfg). Read by every
	// config load (validate, settings apply, seed, showconfig, …).
	app.RootCmd.PersistentFlags().StringVarP(&embedfs.OverrideFile, "config", "c", "",
		"partial YAML config file that overrides the embedded nabu-config.yaml")
	// Global `--course-dir`/`-C`: a course repo whose nabu.yaml is the override (when
	// --config is not given). Without either, ./nabu.yaml is used if present.
	app.RootCmd.PersistentFlags().StringVarP(&embedfs.CourseDir, "course-dir", "C", "",
		"course directory whose "+embedfs.CourseFile+" overrides the embedded config")

	// `seed` populates demo content; `stop` signals a running server.
	app.RootCmd.AddCommand(seed.NewCommand(app))
	app.RootCmd.AddCommand(serverctl.NewStopCommand(app))

	// `users` — roster export/import and full backup/restore (see internal/students).
	app.RootCmd.AddCommand(students.NewCommand(app))

	// `admin` — grouped administrative operations (e.g. `admin s3 set-cors`),
	// kept off the top level to reduce accidental invocation (see internal/admin).
	app.RootCmd.AddCommand(admin.NewCommand(app))

	// `snapshot` bakes the current pb_data + pb_public into the deploy blob;
	// `install` extracts that blob onto local storage (see internal/deploy).
	app.RootCmd.AddCommand(deploy.NewSnapshotCommand(app))
	app.RootCmd.AddCommand(deploy.NewInstallCommand(app))

	// snapshot/install operate on files and must run BEFORE PocketBase opens or
	// creates the database (see internal/deploy). Handle them here and exit.
	if handled, err := deploy.PreBootstrap(app); handled {
		if err != nil {
			slog.Error("deploy command failed", "err", err)
			os.Exit(1)
		}
		return
	}

	// record login + session_start rows in the activity collection.
	hooks.BindActivity(app)
	hooks.BindSessionActivity(app)
	// stamp the running binary's version/build info into the `nabu` table on serve.
	hooks.BindNabuInfo(app)

	// Serve the built SvelteKit SPA and write the pid file so `stop` can find us.
	// indexFallback=true routes unknown paths back to index.html so client-side
	// routing (e.g. /session/:id) works.
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// tee slog -> PocketBase's _logs so operational logs also show in /_/ Logs.
		bridgeLogsToPocketBase(se.App)
		if err := serverctl.WritePidFile(app.DataDir()); err != nil {
			slog.Warn("could not write pid file", "err", err)
		}
		// pb_public is resolved as a sibling of the data dir so the same code
		// works in dev (./pb_data -> ./pb_public) and in a deployed blob
		// (/opt/nabu/pb_data -> /opt/nabu/pb_public).
		// custom admin/teacher endpoints (roster + backup) — register before the
		// SPA catch-all so /api/* is matched first.
		httpapi.Register(se)
		// fail jobs left queued/running by a previous process so they don't hold
		// their group's quota forever, then start the periodic stuck-job sweeper.
		httpapi.ReconcileOrphanJobs(se.App)
		httpapi.StartJobSweeper(se.App)
		// keep the resource index synced from the live buckets in the background —
		// an initial build plus a periodic sweep. Runs in a goroutine so a slow S3
		// list never blocks boot; browse/usage reads come from the index regardless.
		httpapi.StartResourceSync(se.App)
		publicDir := filepath.Join(filepath.Dir(app.DataDir()), "pb_public")
		se.Router.GET("/{path...}", apis.Static(os.DirFS(publicDir), true))
		return se.Next()
	})

	// On graceful shutdown of the serving process, clean up the pid file and
	// report sessions served. Guarded so other commands (seed, stop, migrate)
	// don't remove a running server's pid file or log a spurious count.
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		if serverctl.IsServing() {
			serverctl.RemovePidFile(app.DataDir())
			slog.Info("server stopping", "sessions_served_this_run", serverctl.SessionsServed())
		}
		return e.Next()
	})

	// Register PocketBase's system commands ourselves — this is what Start() does
	// internally — so we can augment `serve` with a --port shorthand before it
	// runs. Execute() then follows the same bootstrap + graceful-shutdown path as
	// Start() but skips re-registering these commands.
	//
	// We deliberately do NOT register PocketBase's `superuser` command: every
	// account, superusers included, is created through the one roster-import path
	// (`nabu seed` / `nabu users import`, see internal/students) or PocketBase's
	// first-run "create your first superuser" page at /_/. This keeps a single,
	// auditable way in and off the CLI's ad-hoc password entry.
	app.RootCmd.AddCommand(cmd.NewServeCommand(app, true))
	enableServePortFlag(app)

	if err := app.Execute(); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

// enableServePortFlag adds a `--port` shorthand to PocketBase's built-in `serve`
// command: `serve --port 8091` is equivalent to `serve --http 127.0.0.1:8091`.
// It makes running a second instance for A/B comparison a one-flag change — pair
// it with a separate `--dir` so the two instances use different pb_data.
//
// Bootstrap runs in app.Start() before cobra dispatches, so translating the flag
// in serve's PreRunE (not PersistentPreRunE, which would shadow PocketBase's own
// hooks) is safe. Passing both --port and --http is rejected as ambiguous.
func enableServePortFlag(app *pocketbase.PocketBase) {
	serveCmd, _, err := app.RootCmd.Find([]string{"serve"})
	if err != nil || serveCmd == nil || serveCmd.Name() != "serve" {
		slog.Debug("serve command not found; --port flag not added")
		return
	}

	var port int
	serveCmd.Flags().IntVar(&port, "port", 0,
		"shorthand for --http 127.0.0.1:PORT; e.g. a 2nd A/B instance: serve --port 8091 --dir pb_data_b")

	prev := serveCmd.PreRunE
	serveCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(cmd, args); err != nil {
				return err
			}
		}
		if port == 0 {
			return nil // flag unset: leave PocketBase's --http handling alone
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("--port %d out of range (1-65535)", port)
		}
		if cmd.Flags().Changed("http") {
			return errors.New("use either --port or --http, not both")
		}
		return cmd.Flags().Set("http", fmt.Sprintf("127.0.0.1:%d", port))
	}
}

// isGoRun reports whether this process was started by `go run`, whose binaries
// are built into the OS temp dir.
func isGoRun() bool {
	return strings.HasPrefix(os.Args[0], os.TempDir())
}
