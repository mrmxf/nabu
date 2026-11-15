package classroom

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/internal/agentsvc"
	"github.com/mrmxf/nabu/internal/httpapi"
	"github.com/mrmxf/nabu/internal/seed"
	"github.com/mrmxf/nabu/internal/serverctl"
	"github.com/mrmxf/nabu/internal/students"
)

// options are the command's flags.
type options struct {
	port       int
	bind       string
	noSeed     bool
	noFastLogn bool
	export     string
}

// NewCommand returns `nabu classroom`. serve is the registered `serve` command,
// which classroom runs with --http set to every interface.
func NewCommand(app core.App, src seed.Sources, serve func() *cobra.Command) *cobra.Command {
	var o options
	cmd := &cobra.Command{
		Use:   "classroom",
		Short: "Run this laptop as the classroom server (LAN, fast login, QR code)",
		Long: "Run a live lesson from this laptop. In one step it:\n\n" +
			"  1. seeds the course (idempotent; skip with --no-seed)\n" +
			"  2. switches classroom fast login on for this run and restores it on exit\n" +
			"  3. warns about agent jobs that cannot run with no worker reachable\n" +
			"  4. serves on every interface and prints the LAN URLs and a QR code\n" +
			"  5. on exit (Ctrl-C), writes the roster to --export, or prints how to\n\n" +
			"It needs no internet. Students join the same Wi-Fi and open the URL.\n" +
			"Fast login only offers a Session inside its enrolment window.\n\n" +
			"Mode: classroom. Check the laptop first with: nabu doctor --mode classroom",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, app, src, serve(), o)
		},
	}
	f := cmd.Flags()
	f.IntVar(&o.port, "port", 8090, "port to serve on")
	f.StringVar(&o.bind, "bind", "0.0.0.0", "address to listen on (0.0.0.0 = every interface)")
	f.BoolVar(&o.noSeed, "no-seed", false, "serve the database as it is, without seeding the course first")
	f.BoolVar(&o.noFastLogn, "no-fast-login", false, "keep classroom fast login as configured (students sign in with passwords)")
	f.StringVar(&o.export, "export", "", "on exit, write the student roster with positions to this file (.yaml, .json or .csv)")
	return cmd
}

func run(cmd *cobra.Command, app core.App, src seed.Sources, serve *cobra.Command, o options) error {
	if serve == nil {
		return fmt.Errorf("classroom: the serve command is not registered")
	}
	if o.port < 1 || o.port > 65535 {
		return fmt.Errorf("--port %d out of range (1-65535)", o.port)
	}
	if running, pid := serverctl.ServerRunning(app.DataDir()); running {
		return fmt.Errorf("a server is already running (pid %d): stop it with `nabu stop`, then run nabu classroom again", pid)
	}

	if !o.noSeed {
		slog.Info("classroom: seeding the course")
		if err := seed.Run(app, seed.Paths{}, src, "goldmark", false); err != nil {
			return fmt.Errorf("classroom: seed: %w", err)
		}
	}

	restore := func() {}
	if !o.noFastLogn {
		prev, err := httpapi.SetClassroomFastLogin(app, true)
		if err != nil {
			return fmt.Errorf("classroom: %w", err)
		}
		slog.Info("classroom fast login on for this run", "was", prev)
		restore = func() {
			if _, err := httpapi.SetClassroomFastLogin(app, prev); err != nil {
				slog.Warn("could not restore classroom fast login", "err", err,
					"fix", "set app_config classroom_fast_login in /_/, or re-run nabu seed")
				return
			}
			slog.Info("classroom fast login restored", "value", prev)
		}
		warnNoSessions(app)
	}

	warnOfflineGaps(cmd.Context(), src)

	var once sync.Once
	finish := func() { once.Do(func() { restore(); exportRoster(app, o.export, cmd.OutOrStdout()) }) }
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		finish()
		return e.Next()
	})

	if err := printJoin(cmd.OutOrStdout(), o.bind, o.port); err != nil {
		return err
	}
	if err := serve.PersistentFlags().Set("http", o.bind+":"+strconv.Itoa(o.port)); err != nil {
		return fmt.Errorf("classroom: set serve --http: %w", err)
	}
	if serve.PreRunE != nil {
		if err := serve.PreRunE(serve, nil); err != nil {
			return err
		}
	}
	err := serve.RunE(serve, nil)
	finish()
	return err
}

// warnNoSessions warns when fast login is on but no Session is open to join.
func warnNoSessions(app core.App) {
	open, err := httpapi.OpenSessions(app)
	if err != nil {
		slog.Warn("could not read the schedule", "err", err)
		return
	}
	if len(open) == 0 {
		slog.Warn("fast login has no open Session to offer",
			"fix", "add a Session whose enrolment window includes now: the teacher schedule page, or content/lesson-schedules.yaml then nabu seed")
		return
	}
	for _, s := range open {
		slog.Info("open for fast login", "session", s)
	}
}

// warnOfflineGaps warns about each agent job that needs a worker, when none
// answers. With a worker up it says nothing.
func warnOfflineGaps(ctx context.Context, src seed.Sources) {
	courses, label, err := seed.LoadCourses(seed.Paths{}, src)
	if err != nil {
		slog.Warn("could not read the course to check offline fallbacks", "err", err)
		return
	}
	gaps := OfflineGaps(courses)
	if len(gaps) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if h := agentsvc.New().Health(ctx); h.Alive {
		slog.Info("agent worker reachable", "agents", len(h.Agents))
		return
	}
	for _, g := range gaps {
		slog.Warn("agent job will fail with no worker", "job", g.Where, "agent", g.Agent, "why", g.Why, "content", label)
	}
	slog.Warn("no agent worker reachable", "jobs_affected", len(gaps),
		"fix", "add sim_output_fallback: true and a sim_output to those slides, or start a worker and set WORKER_HEALTH_URL")
}

// printJoin prints the URLs students open, and a QR code for the first.
func printJoin(w io.Writer, bind string, port int) error {
	var urls []string
	if bind == "0.0.0.0" || bind == "" {
		addrs, err := LANAddrs()
		if err != nil {
			slog.Warn("could not list network interfaces", "err", err)
		}
		for _, a := range addrs {
			urls = append(urls, fmt.Sprintf("http://%s:%d", a, port))
		}
	} else {
		urls = append(urls, fmt.Sprintf("http://%s:%d", bind, port))
	}
	if len(urls) == 0 {
		slog.Warn("no LAN address found: students cannot reach this laptop",
			"fix", "join the room's Wi-Fi, or start a hotspot, then run nabu classroom again")
		_, err := fmt.Fprintf(w, "\nThis laptop only: http://127.0.0.1:%d\n\n", port)
		return err
	}
	if _, err := fmt.Fprintf(w, "\nStudents: join this Wi-Fi, then open\n"); err != nil {
		return err
	}
	for _, u := range urls {
		if _, err := fmt.Fprintf(w, "  %s\n", u); err != nil {
			return err
		}
	}
	code, err := QR(urls[0])
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "\n%s\nTeacher: http://127.0.0.1:%d   Stop: Ctrl-C\n\n", code, port)
	return err
}

// exportRoster writes the student roster to file, or says how to.
func exportRoster(app core.App, file string, w io.Writer) {
	if file == "" {
		_, _ = fmt.Fprintln(w, "\nTo keep the results: nabu users export --role student --positions --out results.yaml")
		return
	}
	recs, err := students.Export(app, students.ExportFilter{Role: "student"}, true)
	if err != nil {
		slog.Warn("could not export the roster", "err", err, "fix", "run nabu users export --role student --positions --out "+file)
		return
	}
	f := students.FormatFromExt(file)
	if f == "" {
		f = students.FormatYAML
	}
	data, err := students.EncodeRoster(recs, f)
	if err == nil {
		err = os.WriteFile(file, data, 0o600)
	}
	if err != nil {
		slog.Warn("could not write the roster", "file", file, "err", err)
		return
	}
	slog.Info("roster exported", "file", file, "students", len(recs))
}
