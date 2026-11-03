// Package nabu is the Nabu LMS engine as a Go library.
//
// Nabu embeds PocketBase (auth, database, realtime, superuser dashboard) and
// serves a Svelte 5 single-page app. The nabu binary (cmd/nabu) is a thin
// wrapper over this package; a course can instead import it and build its own
// binary with its content baked in:
//
//	//go:embed all:content
//	var content embed.FS
//
//	func main() {
//		course, _ := fs.Sub(content, "content")
//		app := nabu.New(nabu.Options{Content: course})
//		if err := app.Start(); err != nil {
//			os.Exit(1)
//		}
//	}
//
// App embeds *pocketbase.PocketBase, so everything PocketBase offers is
// available before Start: add commands to App.RootCmd, add routes or hooks via
// App.OnServe, and so on. Agents are not part of this process; lessons dispatch
// jobs to a worker over HTTP (see internal/agentsvc and the runtime/ package).
package nabu

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/mrmxf/util/slogger"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/cmd"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/admin"
	"github.com/mrmxf/nabu/internal/demo"
	"github.com/mrmxf/nabu/internal/deploy"
	"github.com/mrmxf/nabu/internal/hooks"
	"github.com/mrmxf/nabu/internal/httpapi"
	"github.com/mrmxf/nabu/internal/seed"
	"github.com/mrmxf/nabu/internal/serverctl"
	"github.com/mrmxf/nabu/internal/students"

	// register schema migrations for side effects
	_ "github.com/mrmxf/nabu/migrations"
)

// DefaultDataDir is where pb_data lives when neither Options.DataDir nor --dir
// is given: `pb_data` in the directory Nabu is launched from. pb_public is
// always its sibling, so a deployment in /opt/nabu uses /opt/nabu/pb_data and
// /opt/nabu/pb_public.
const DefaultDataDir = "pb_data"

// Options configures a Nabu App. The zero value is the plain nabu binary: the
// course is read from disk via nabu.yaml (or -c / -C / flags) and the SPA from
// the pb_public folder beside pb_data.
//
// For each embedded tree the precedence is: an explicit command-line flag
// (e.g. `seed --content dir`), then the field here, then the course config.
type Options struct {
	// ConfigYAML is a course config (the contents of a nabu.yaml) baked into the
	// binary. It is merged over Nabu's defaults and under any config file given
	// at run time (-c, -C, ./nabu.yaml), so an operator can still override it.
	ConfigYAML []byte

	// Content is the course content tree, rooted at the folder that holds the
	// course folders (content/<course>/<lesson>/<slide>.md => <course>/…). A
	// lesson-schedules.yaml at its root is used as the schedules pre-load.
	Content fs.FS
	// Assets is the Hugo-style assets tree published into pb_public at seed.
	Assets fs.FS
	// Help replaces Nabu's built-in help course. Nil keeps the built-in help.
	Help fs.FS
	// DemoRoster is the demo roster (YAML) used when demo_mode is on and the
	// config names no course.demo_roster file.
	DemoRoster []byte

	// Public is the built single-page app to serve. Nil serves the pb_public
	// folder beside the data dir, as the plain binary does. The SPA is a build
	// output (web/, `npm run build`), not part of the Go module.
	Public fs.FS

	// DataDir overrides DefaultDataDir. The --dir flag still wins.
	DataDir string
	// Automigrate writes Go migration files into MigrationsDir when the schema is
	// changed in the /_/ dashboard. Only for developing Nabu itself; leave it
	// false in a course binary.
	Automigrate bool
	// MigrationsDir is where Automigrate writes. Default "migrations".
	MigrationsDir string
	// KeepLogger leaves the default slog logger alone. By default New installs
	// Nabu's pretty console logger, which is Nabu's user-facing progress stream.
	KeepLogger bool
}

// App is a configured Nabu server: PocketBase plus Nabu's commands, hooks,
// migrations and routes. Configure it further before calling Start.
type App struct {
	*pocketbase.PocketBase
	opts Options
}

// New builds a Nabu App. It registers Nabu's commands (seed, stop, users,
// admin, snapshot, install, migrate), global flags (-c/--config,
// -C/--course-dir), hooks and routes, but does not start anything.
//
// New sets package-level state shared by Nabu's internals (the baked config and
// demo roster), so build one App per process.
func New(opts Options) *App {
	if !opts.KeepLogger {
		slogger.UsePrettyLogger(slog.LevelInfo)
	}
	if opts.DataDir == "" {
		opts.DataDir = DefaultDataDir
	}
	if opts.MigrationsDir == "" {
		opts.MigrationsDir = "migrations"
	}
	embedfs.OverrideYAML = opts.ConfigYAML
	demo.Override = opts.DemoRoster

	app := &App{
		PocketBase: pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: opts.DataDir}),
		opts:       opts,
	}
	pb := app.PocketBase

	migratecmd.MustRegister(pb, pb.RootCmd, migratecmd.Config{
		Automigrate: opts.Automigrate,
		Dir:         opts.MigrationsDir,
	})

	// Global `--config`/`-c`: a partial YAML file that overrides the embedded
	// defaults (deep-merged via kfg). Read by every config load.
	pb.RootCmd.PersistentFlags().StringVarP(&embedfs.OverrideFile, "config", "c", "",
		"partial YAML config file that overrides the embedded nabu-config.yaml")
	// Global `--course-dir`/`-C`: a course repo whose nabu.yaml is the override
	// (when --config is not given). Without either, ./nabu.yaml is used if present.
	pb.RootCmd.PersistentFlags().StringVarP(&embedfs.CourseDir, "course-dir", "C", "",
		"course directory whose "+embedfs.CourseFile+" overrides the embedded config")

	pb.RootCmd.AddCommand(seed.NewCommand(pb, seed.Sources{
		Content: opts.Content,
		Assets:  opts.Assets,
		Help:    opts.Help,
	}))
	pb.RootCmd.AddCommand(serverctl.NewStopCommand(pb))
	pb.RootCmd.AddCommand(students.NewCommand(pb))
	pb.RootCmd.AddCommand(admin.NewCommand(pb))
	pb.RootCmd.AddCommand(deploy.NewSnapshotCommand(pb))
	pb.RootCmd.AddCommand(deploy.NewInstallCommand(pb))

	// record login + session_start rows in the activity collection, and stamp
	// the running binary's version into the `nabu` table on serve.
	hooks.BindActivity(pb)
	hooks.BindSessionActivity(pb)
	hooks.BindNabuInfo(pb)

	pb.OnServe().BindFunc(app.onServe)

	// On graceful shutdown of the serving process, clean up the pid file and
	// report sessions served. Guarded so other commands (seed, stop, migrate)
	// don't remove a running server's pid file or log a spurious count.
	pb.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		if serverctl.IsServing() {
			serverctl.RemovePidFile(pb.DataDir())
			slog.Info("server stopping", "sessions_served_this_run", serverctl.SessionsServed())
		}
		return e.Next()
	})
	return app
}

// onServe wires Nabu's routes and background work when `serve` starts, then
// serves the SPA as the catch-all "/{path...}". A caller's own OnServe handler
// (bound after New) runs when this one calls se.Next(); the router matches the
// most specific pattern first, so the caller's routes win over the catch-all.
func (a *App) onServe(se *core.ServeEvent) error {
	// tee slog -> PocketBase's _logs so operational logs also show in /_/ Logs.
	bridgeLogsToPocketBase(se.App)
	if err := serverctl.WritePidFile(a.DataDir()); err != nil {
		slog.Warn("could not write pid file", "err", err)
	}
	httpapi.Register(se)
	// fail jobs left queued/running by a previous process so they don't hold
	// their group's quota forever, then start the periodic stuck-job sweeper.
	httpapi.ReconcileOrphanJobs(se.App)
	httpapi.StartJobSweeper(se.App)
	// keep the resource index synced from the live buckets in the background.
	httpapi.StartResourceSync(se.App)

	public := a.opts.Public
	if public == nil {
		public = os.DirFS(filepath.Join(filepath.Dir(a.DataDir()), "pb_public"))
	}
	// indexFallback=true routes unknown paths to index.html so client-side
	// routes (e.g. /session/:id) work.
	se.Router.GET("/{path...}", apis.Static(public, true))
	return se.Next()
}

// Start runs the command line: `serve`, `seed`, `stop` and the rest. It returns
// when the command finishes (for `serve`, on graceful shutdown).
func (a *App) Start() error {
	pb := a.PocketBase
	// snapshot/install operate on files and must run BEFORE PocketBase opens or
	// creates the database (see internal/deploy).
	if handled, err := deploy.PreBootstrap(pb); handled {
		if err != nil {
			return fmt.Errorf("deploy command: %w", err)
		}
		return nil
	}

	// Register PocketBase's system commands ourselves — this is what
	// pocketbase.Start does internally — so `serve` can gain a --port shorthand.
	// We deliberately do NOT register PocketBase's `superuser` command: every
	// account is created through the one roster-import path (`nabu seed` /
	// `nabu users import`) or PocketBase's first-run page at /_/.
	pb.RootCmd.AddCommand(cmd.NewServeCommand(pb, true))
	enableServePortFlag(pb)
	return pb.Execute()
}

// enableServePortFlag adds a `--port` shorthand to PocketBase's built-in `serve`
// command: `serve --port 8091` is equivalent to `serve --http 127.0.0.1:8091`.
// It makes running a second instance for A/B comparison a one-flag change — pair
// it with a separate `--dir` so the two instances use different pb_data.
//
// Bootstrap runs before cobra dispatches, so translating the flag in serve's
// PreRunE (not PersistentPreRunE, which would shadow PocketBase's own hooks) is
// safe. Passing both --port and --http is rejected as ambiguous.
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
