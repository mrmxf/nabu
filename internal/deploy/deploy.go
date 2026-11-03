// Package deploy provides the self-extracting deploy blob commands:
//
//	snapshot  capture the current pb_data + pb_public and bake them into a
//	          deploy binary (nabu-deploy). It packs internal/assets/embedded.tgz,
//	          rebuilds with `-tags deploy` so go:embed picks the archive up, then
//	          moves the archive to /tmp so it never lingers in the source tree.
//	install   extract the embedded assets onto the target host's storage. Runs
//	          anywhere (e.g. a container init) with zero external dependencies —
//	          only nabu-deploy carries the assets (a plain `nabu` is lean).
//
// Because go:embed is resolved at compile time, the deploy flow is: build the
// lean dev binary, seed/edit/test, then `snapshot` (pack + tagged rebuild) to
// produce nabu-deploy, then `install` on the target.
//
// Both commands run BEFORE PocketBase bootstraps the app (see PreBootstrap), so
// they operate on the data directory without the framework opening or creating
// the SQLite database first. That is required for correctness: bootstrapping
// would create a fresh schema-only pb_data (masking the empty-volume case that
// install must fill) and could leave recent writes in a -wal sidecar that
// snapshot deliberately excludes.
package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver for checkpointWAL

	"github.com/mrmxf/nabu/internal/assets"
	"github.com/mrmxf/nabu/internal/serverctl"
)

// Deploy-build constants: `snapshot` rebuilds the `deploy`-tagged binary so
// go:embed bakes in the freshly packed archive, and names it deployBin.
const (
	deployBuildTag = "deploy"
	deployBin      = "nabu-deploy"
	deployPkg      = "./cmd/nabu"
)

// publicDir returns the pb_public path as a sibling of the data dir, so the same
// derivation works in dev (nabu/pb_data -> .../pb_public) and in a
// deployed container (--dir /data/pb_data -> /data/pb_public). It matches how
// main.go resolves the directory it serves from.
func publicDir(dataDir string) string {
	return filepath.Join(filepath.Dir(dataDir), "pb_public")
}

// treeTargets maps each archive top-level prefix to the on-disk directory it is
// packed from / extracted to, derived from the app's data dir.
func treeTargets(dataDir string) map[string]string {
	return map[string]string{
		"pb_data":   dataDir,
		"pb_public": publicDir(dataDir),
	}
}

// PreBootstrap runs snapshot/install before PocketBase bootstraps the app and
// reports whether it handled the invocation. When handled is true the caller
// must exit without calling app.Start(). --help/-h and every other command fall
// through to normal cobra handling.
func PreBootstrap(app *pocketbase.PocketBase) (handled bool, err error) {
	cmd, _, ferr := app.RootCmd.Find(os.Args[1:])
	if ferr != nil || cmd == nil {
		return false, nil
	}
	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" {
			return false, nil
		}
	}
	switch cmd.Name() {
	case "snapshot":
		return true, RunSnapshot(app)
	case "install":
		force := false
		for _, a := range os.Args[1:] {
			if a == "--force" {
				force = true
			}
		}
		return true, RunInstall(app, force)
	default:
		return false, nil
	}
}

// isVolatile reports whether a file inside pb_data should be excluded from the
// snapshot: runtime-only or SQLite sidecar files that must not travel with the
// blob and can corrupt a copy taken while the server is running.
func isVolatile(name string) bool {
	base := filepath.Base(name)
	if base == serverctl.PidFileName {
		return true
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

// RunSnapshot captures the live pb_data + pb_public and bakes them into the
// deploy binary (nabu-deploy). It never leaves the archive in the source tree:
//
//  1. move any leftover embedded.tgz aside to /tmp (dated), so a prior snapshot
//     can't get baked in by mistake;
//  2. pack pb_data + pb_public into a fresh embedded.tgz;
//  3. rebuild `nabu-deploy` with `-tags deploy` so go:embed bakes the archive in;
//  4. move embedded.tgz to /tmp (dated) — the source tree is left clean, and the
//     dated archive is kept as a record.
func RunSnapshot(app core.App) error {
	dataDir := app.DataDir()

	// A running Nabu may be mid-write to the SQLite files; warn but proceed.
	if _, err := os.Stat(filepath.Join(dataDir, serverctl.PidFileName)); err == nil {
		slog.Warn("Nabu appears to be running; snapshot may be inconsistent — run `stop` first",
			"pid_file", filepath.Join(dataDir, serverctl.PidFileName))
	}

	// Flush any pending SQLite WAL into data.db BEFORE packing. The snapshot
	// deliberately excludes the -wal sidecar (see isVolatile), so writes still
	// sitting in an uncheckpointed WAL — e.g. the latest `seed` if the DB wasn't
	// cleanly closed, or writes made while serving — would otherwise be silently
	// dropped from the blob. This makes data.db self-contained before it is packed.
	checkpointWAL(dataDir)

	stamp := time.Now().Format("20060102-150405")

	// (1) move any leftover archive out of the way before packing a fresh one.
	if _, err := os.Stat(assets.SourcePath); err == nil {
		if dst, err := moveToTmp(assets.SourcePath, "embedded-prev-"+stamp+".tgz"); err != nil {
			return fmt.Errorf("snapshot: archive existing embed: %w", err)
		} else {
			slog.Info("moved leftover embed aside", "to", dst)
		}
	}

	// (2) pack the live trees into the embed source.
	n, err := pack(treeTargets(dataDir), assets.SourcePath)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	var size int64
	if info, statErr := os.Stat(assets.SourcePath); statErr == nil {
		size = info.Size()
	}
	slog.Info("snapshot packed", "file", assets.SourcePath, "entries", n, "bytes", size)

	// (3) rebuild the deploy-tagged binary so go:embed bakes the archive in.
	if err := buildDeployBinary(); err != nil {
		// Leave the archive in place on failure so the user can retry the build.
		return fmt.Errorf("snapshot: build %s: %w", deployBin, err)
	}
	slog.Info("built deploy binary", "bin", deployBin, "tags", deployBuildTag)

	// (4) move the archive to /tmp so it never lingers in the tree (dated record).
	archived, err := moveToTmp(assets.SourcePath, "embedded-"+stamp+".tgz")
	if err != nil {
		return fmt.Errorf("snapshot: archive embed: %w", err)
	}
	slog.Info("moved embed out of tree", "to", archived)

	fmt.Printf("Snapshot baked %d file(s) into ./%s (archive: %s).\nDeploy it:\n  scp %s user@host:/opt/nabu/\n  ssh user@host 'cd /opt/nabu && ./%s install'   # -> ./pb_data + ./pb_public\n",
		n, deployBin, archived, deployBin, deployBin)
	return nil
}

// buildDeployBinary compiles the deploy-tagged binary (nabu-deploy) from the repo
// root, so go:embed bakes in the embedded.tgz that snapshot just wrote.
//
// It forces CGO_ENABLED=0 (the SQLite driver is pure Go) and inherits GOOS/GOARCH
// from the environment, so cross-compiling for another host is just:
//
//	GOOS=linux GOARCH=arm64 ./bin/nabu snapshot
//
// (run the host `nabu` binary, not `go run`, so only the go build subprocess
// targets the foreign arch).
func buildDeployBinary() error {
	args := []string{"build", "-tags", deployBuildTag}
	// Inject the same git-derived version info as scripts/build.sh, from the shared
	// buildinfo.sh, so nabu-deploy reports the same version as a lean build.
	if ld, err := versionLdflags(); err != nil {
		slog.Warn("could not compute version ldflags; nabu-deploy will report a dev build", "err", err)
	} else if ld != "" {
		args = append(args, "-ldflags", ld)
	}
	args = append(args, "-o", deployBin, deployPkg)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// versionLdflags runs the shared buildinfo.sh (git → buildinfo -X flag) and returns
// its `-ldflags` value, so the deploy build stamps the same version as build.sh.
func versionLdflags() (string, error) {
	out, err := exec.Command("bash", "scripts/buildinfo.sh").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// moveToTmp moves src to /tmp/<name>, falling back to copy+remove when a rename
// would cross filesystems (common on WSL, where the repo and /tmp differ). It
// returns the destination path.
func moveToTmp(src, name string) (string, error) {
	dst := filepath.Join(os.TempDir(), name)
	if err := os.Rename(src, dst); err == nil {
		return dst, nil
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(src); err != nil {
		return "", err
	}
	return dst, nil
}

// RunInstall extracts the embedded deploy blob onto local storage.
func RunInstall(app core.App, force bool) error {
	// A lean (non-deploy) binary carries no blob. Say so plainly instead of
	// failing deep in the gzip reader — install is a nabu-deploy operation.
	if len(assets.Blob) == 0 {
		return fmt.Errorf("install: this binary has no embedded content — use nabu-deploy (produced by `nabu snapshot`), not a plain `nabu`")
	}
	n, err := extract(assets.Blob, treeTargets(app.DataDir()), force)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	slog.Info("install complete", "data_dir", app.DataDir(), "files", n)
	return nil
}

// NewSnapshotCommand registers the `snapshot` command. The actual work runs
// pre-bootstrap via PreBootstrap; this exists for discoverability and --help.
func NewSnapshotCommand(app core.App) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot",
		Short: "Bake pb_data + pb_public into the deploy binary (nabu-deploy)",
		Long: "Capture the current pb_data + pb_public and bake them into a deploy\n" +
			"binary. Packs internal/assets/embedded.tgz, rebuilds nabu-deploy with\n" +
			"`-tags deploy` so go:embed picks the archive up, then moves the archive to\n" +
			"/tmp (dated) so it never lingers in the tree or bloats a plain `nabu`.\n\n" +
			"Run it after you have built, seeded/imported, and tested. Requires the Go\n" +
			"toolchain (it rebuilds). Then `scp nabu-deploy` to the host and `install`.",
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return RunSnapshot(app) },
	}
}

// NewInstallCommand registers the `install` command (see NewSnapshotCommand note).
func NewInstallCommand(app core.App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Extract the embedded pb_data + pb_public onto local storage",
		Long: "Extract the embedded pb_data + pb_public onto local storage.\n\n" +
			"pb_data defaults to ./pb_data (the launch dir) and pb_public to its sibling,\n" +
			"so running from the install dir needs no --dir; pass --dir <path> to place it\n" +
			"elsewhere. Existing trees are left untouched unless --force is given.",
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return RunInstall(app, force) },
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing pb_data / pb_public")
	return cmd
}

// checkpointWAL merges any pending SQLite WAL into data.db so the snapshot — which
// excludes the -wal/-shm sidecars (see isVolatile) — captures the latest committed
// writes. It opens data.db with the pure-Go "sqlite" driver, runs a TRUNCATE
// checkpoint, and closes. Best-effort: a missing db is a no-op; a failure or an
// incomplete checkpoint (e.g. a running server still holds the WAL) is warned, not
// fatal — packing then proceeds with whatever data.db already contains, exactly as
// before, but the operator is told the blob may be stale.
func checkpointWAL(dataDir string) {
	dbPath := filepath.Join(dataDir, "data.db")
	if _, err := os.Stat(dbPath); err != nil {
		return // no database yet
	}
	err := func() error {
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			return err
		}
		defer db.Close()
		if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
			return err
		}
		// wal_checkpoint(TRUNCATE) flushes the WAL into data.db and zeroes it. It
		// returns (busy, log_frames, checkpointed_frames); busy != 0 means it could
		// not fully flush because another connection (a running server) holds it.
		var busy, logFrames, ckpt int
		if err := db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &ckpt); err != nil {
			return err
		}
		if busy != 0 {
			return fmt.Errorf("checkpoint incomplete (busy=%d, %d frames pending) — stop the server first", busy, logFrames)
		}
		return nil
	}()
	if err != nil {
		slog.Warn("snapshot: could not fully checkpoint the SQLite WAL — the blob may be missing recent writes; run `nabu stop`, then snapshot again",
			"err", err)
	}
}

// pack writes a gzip'd tar of each target dir to outPath, storing entries under
// the corresponding prefix. Volatile pb_data files are skipped. It returns the
// number of regular files written. Prefixes are packed in a fixed order for
// deterministic archives.
func pack(targets map[string]string, outPath string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	count := 0
	for _, prefix := range []string{"pb_data", "pb_public"} {
		n, err := packTree(tw, targets[prefix], prefix)
		if err != nil {
			return count, err
		}
		count += n
	}

	if err := tw.Close(); err != nil {
		return count, err
	}
	if err := gw.Close(); err != nil {
		return count, err
	}
	return count, f.Close()
}

// packTree walks root and writes its regular files under prefix/ into tw.
func packTree(tw *tar.Writer, root, prefix string) (int, error) {
	info, err := os.Stat(root)
	if err != nil {
		return 0, fmt.Errorf("source %s: %w", root, err)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("source %s is not a directory", root)
	}

	count := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Exclude PocketBase's realtime notify dir wholesale.
		if d.IsDir() && d.Name() == ".notify" {
			return fs.SkipDir
		}
		if d.IsDir() || isVolatile(p) {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		name := path.Join(prefix, filepath.ToSlash(rel))

		fi, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		if _, err := io.Copy(tw, in); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

// extract writes the files in blob into their target dirs. For each top-level
// tree it skips extraction when the target already exists and is non-empty,
// unless force is set (in which case the existing tree is replaced). It returns
// the number of files written.
func extract(blob []byte, targets map[string]string, force bool) (int, error) {
	gr, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return 0, fmt.Errorf("read blob: %w", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	// decision caches, per top-level prefix, the resolved target dir to write
	// into, or "" to skip. Resolved lazily on first sight of that prefix.
	type plan struct {
		dir  string
		skip bool
	}
	decided := map[string]plan{}
	count := 0
	total := 0

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("read entry: %w", err)
		}
		total++

		clean := path.Clean(hdr.Name)
		prefix, rest, ok := strings.Cut(clean, "/")
		if !ok || strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
			return count, fmt.Errorf("unsafe path in blob: %q", hdr.Name)
		}
		dest, known := targets[prefix]
		if !known {
			return count, fmt.Errorf("unexpected tree in blob: %q", prefix)
		}

		p, ready := decided[prefix]
		if !ready {
			skip, rerr := resolveTree(dest, force)
			if rerr != nil {
				return count, rerr
			}
			p = plan{dir: dest, skip: skip}
			decided[prefix] = p
		}
		if p.skip {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		target := filepath.Join(dest, filepath.FromSlash(rest))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return count, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(hdr.Mode)&0o777)
		if err != nil {
			return count, err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return count, err
		}
		if err := out.Close(); err != nil {
			return count, err
		}
		count++
	}

	if total == 0 {
		return 0, fmt.Errorf("no embedded content; run `snapshot` and rebuild first")
	}
	return count, nil
}

// resolveTree decides whether to skip the given target dir, warning when it
// already exists and is non-empty (unless force replaces it).
func resolveTree(target string, force bool) (skip bool, err error) {
	nonEmpty, err := dirNonEmpty(target)
	if err != nil {
		return false, err
	}
	if !nonEmpty {
		return false, nil
	}
	if !force {
		slog.Warn("target exists, skipping; use --force to overwrite", "path", target)
		return true, nil
	}
	if err := os.RemoveAll(target); err != nil {
		return false, fmt.Errorf("replace %s: %w", target, err)
	}
	return false, nil
}

// dirNonEmpty reports whether dir is an existing directory containing entries.
func dirNonEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}
