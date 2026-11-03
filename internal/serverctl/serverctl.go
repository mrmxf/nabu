// Package serverctl adds process control for Nabu: a pid file written on serve,
// a `stop` command that signals it, and an in-process counter of sessions
// served this run.
package serverctl

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// startLayout matches PocketBase's stored `created` datetime format, so the
// pid-file start time can be compared directly against activity rows.
const startLayout = "2006-01-02 15:04:05.000Z"

// DefaultPort is PocketBase's default serve port. `stop` probes 127.0.0.1 on
// this port (override with --port) to tell "nothing is running" apart from
// "something is listening that Nabu didn't start" (e.g. an orphaned process
// whose pid file was lost, or an A/B instance started with `serve --port`).
const DefaultPort = 8090

// portInUse reports whether something is accepting TCP connections on addr. A
// successful dial means a server is listening; connection-refused means the
// port is free. Best-effort: any dial error is treated as "not in use".
func portInUse(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// PidFileName is the basename of Nabu's pid file inside the data dir. It is
// exported so the snapshot logic (internal/deploy) can treat it as volatile.
const PidFileName = "nabu.pid"

func pidFilePath(dataDir string) string {
	return filepath.Join(dataDir, PidFileName)
}

// sessionsServed counts sessions created since this process started.
var sessionsServed atomic.Int64

// serving is true only in the process that wrote the pid file (the `serve`
// command), so other commands (seed, stop, migrate) don't touch it on exit.
var serving atomic.Bool

// IsServing reports whether this process is the running server.
func IsServing() bool { return serving.Load() }

// IncSessionsServed is called from the session-create hook.
func IncSessionsServed() { sessionsServed.Add(1) }

// SessionsServed returns the count of sessions served this run.
func SessionsServed() int64 { return sessionsServed.Load() }

// WritePidFile records this process's pid and start time so `stop` can find it
// and report how many sessions were served.
func WritePidFile(dataDir string) error {
	content := fmt.Sprintf("%d\n%s\n", os.Getpid(), time.Now().UTC().Format(startLayout))
	if err := os.WriteFile(pidFilePath(dataDir), []byte(content), 0o644); err != nil {
		return err
	}
	serving.Store(true)
	return nil
}

// RemovePidFile deletes the pid file (called on graceful shutdown).
func RemovePidFile(dataDir string) { _ = os.Remove(pidFilePath(dataDir)) }

func readPidFile(dataDir string) (pid int, start string, err error) {
	b, err := os.ReadFile(pidFilePath(dataDir))
	if err != nil {
		return 0, "", err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	pid, err = strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, "", fmt.Errorf("invalid pid file: %w", err)
	}
	if len(lines) > 1 {
		start = strings.TrimSpace(lines[1])
	}
	return pid, start, nil
}

// ServerRunning reports whether a Nabu server started with `serve` is currently
// running, by reading the pid file and probing the recorded process with
// signal 0. A missing pid file, or one naming a process that is no longer alive
// (a stale file left by an unclean exit), reports false. The returned pid is
// meaningful only when running is true. It is read-only: it never removes a
// stale pid file, so callers using it as a safety interlock don't mutate state.
func ServerRunning(dataDir string) (running bool, pid int) {
	pid, _, err := readPidFile(dataDir)
	if err != nil {
		return false, 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	if proc.Signal(syscall.Signal(0)) != nil {
		return false, 0 // stale pid file: the process is gone
	}
	return true, pid
}

// NewStopCommand stops a running server (located via the pid file) and reports
// how many new sessions it served this run.
func NewStopCommand(app core.App) *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:          "stop",
		Short:        "Stop the running Nabu server",
		SilenceUsage: true, // a runtime failure shouldn't dump the flag list
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir := app.DataDir()
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			pid, start, err := readPidFile(dataDir)
			if err != nil {
				// No pid file to signal. If the port is nonetheless occupied, a
				// server is running that Nabu can't stop the normal way (its pid
				// file was lost, or another process grabbed the port) — point the
				// operator at the OS tools to find and stop it.
				if portInUse(addr) {
					return portInUseErr(pidFilePath(dataDir), addr)
				}
				return fmt.Errorf("no running server found (%s)", pidFilePath(dataDir))
			}

			proc, err := os.FindProcess(pid)
			if err != nil || proc.Signal(syscall.Signal(0)) != nil {
				RemovePidFile(dataDir)
				return fmt.Errorf("server (pid %d) is not running; removed stale pid file", pid)
			}

			// Report sessions served this run from the activity audit trail.
			if start != "" {
				if n, cErr := app.CountRecords("activity",
					dbx.HashExp{"event": "session_start"},
					dbx.NewExp("created >= {:start}", dbx.Params{"start": start}),
				); cErr == nil {
					fmt.Printf("Server served %d new session(s) since %s\n", n, start)
				}
			}

			if err := proc.Signal(syscall.SIGTERM); err != nil {
				return fmt.Errorf("stop pid %d: %w", pid, err)
			}
			fmt.Printf("Sent stop signal to server (pid %d)\n", pid)

			// Wait for graceful shutdown; the server removes the pid file on exit.
			for i := 0; i < 50; i++ {
				if _, _, err := readPidFile(dataDir); err != nil {
					fmt.Println("Server stopped.")
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", DefaultPort,
		"server port to probe when no pid file is found (match `serve --port`)")
	return cmd
}

// portInUseErr builds the guidance shown when the port is occupied but no pid
// file exists: Nabu can't signal the process itself, so it points the operator
// at the OS tools to identify and stop whatever is holding the port.
func portInUseErr(pidPath, addr string) error {
	port := addr
	if _, p, err := net.SplitHostPort(addr); err == nil {
		port = p
	}
	return fmt.Errorf(
		"no pid file (%s), but %s is already in use — a server is running that Nabu didn't start.\n"+
			"Find and stop it with:\n"+
			"    sudo lsof -nP -iTCP:%s -sTCP:LISTEN   # show the listening pid\n"+
			"    sudo fuser -k %s/tcp                  # stop whatever holds the port",
		pidPath, addr, port, port)
}
