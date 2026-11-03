package admin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/content"
	"github.com/mrmxf/nabu/internal/s3svc"
	"github.com/mrmxf/nabu/internal/source"
)

// severity is the outcome of one validation check. Ordered so the worst seen
// drives the exit code: PASS < WARN < FAIL.
type severity int

const (
	sevPass severity = iota
	sevWarn
	sevFail
)

// validator accumulates check outcomes and logs each per the shared admin
// conventions: verbose by default; -q suppresses PASS/WARN but FAIL always
// prints; secrets are never logged (only env-var names + set/MISSING).
type validator struct {
	quiet bool
	worst severity
}

// record logs one outcome and tracks the worst severity seen. kv are extra
// structured fields — callers must pass only non-secret values (var NAMES, paths,
// counts), never secret values.
func (v *validator) record(sev severity, check string, kv ...any) {
	if sev > v.worst {
		v.worst = sev
	}
	switch sev {
	case sevFail:
		slog.Error(check, append([]any{"status", "FAIL"}, kv...)...)
	case sevWarn:
		if !v.quiet {
			slog.Warn(check, append([]any{"status", "WARN"}, kv...)...)
		}
	default:
		if !v.quiet {
			slog.Info(check, append([]any{"status", "PASS"}, kv...)...)
		}
	}
}

// failed reports whether any check failed (⇒ exit 1).
func (v *validator) failed() bool { return v.worst >= sevFail }

// summary prints the final one-line verdict (always printed on failure).
func (v *validator) summary() {
	switch {
	case v.worst >= sevFail:
		slog.Error("validation FAILED", "hint", "fix the FAIL lines above, then re-run")
	case v.worst == sevWarn:
		if !v.quiet {
			slog.Warn("validation passed with warnings")
		}
	default:
		if !v.quiet {
			slog.Info("validation passed")
		}
	}
}

// checkOpts configures a validation run.
type checkOpts struct {
	deep       bool
	contentDir string
}

// runChecks executes every readiness check in fast-fail order. It does not stop
// on the first failure — it runs them all so one pass surfaces every fixable
// problem — but any FAIL makes failed() true.
func runChecks(ctx context.Context, app core.App, v *validator, opt checkOpts) {
	cfg := checkConfig(v)                      // 1 · embedded config parses
	checkDBPaths(v, app)                       // 2 · pb_data writeable, pb_public present
	checkSuperuser(v, app, cfg)                // 3 · a superuser exists or is configured
	courses := checkContent(v, opt.contentDir) // 4 · content parses + declares needs
	n := deriveNeeds(courses)
	checkEnv(v, cfg, n)          // 5 · env vars present for the discovered needs
	checkS3(ctx, v, n, opt.deep) // 6 · buckets reachable (and writeable, if --deep)
}

// needs is what the content tree implies the deployment must provide.
type needs struct {
	buckets []string // distinct, non-empty lesson buckets ⇒ S3 required
	agents  bool     // any agent slide/job ⇒ a worker is expected (optional)
}

// deriveNeeds walks loaded courses and collects the secret-requiring signals.
// Pure (no I/O) so it is unit-testable.
func deriveNeeds(courses []content.Course) needs {
	seen := map[string]bool{}
	var n needs
	for _, c := range courses {
		for _, l := range c.Lessons {
			if l.Bucket != "" && !seen[l.Bucket] {
				seen[l.Bucket] = true
				n.buckets = append(n.buckets, l.Bucket)
			}
			for _, s := range l.Slides {
				if s.Kind == "agent" || len(s.AgentJobs) > 0 {
					n.agents = true
				}
			}
		}
	}
	sort.Strings(n.buckets)
	return n
}

// envSet reports whether an env var is present and non-empty. Never returns the
// value — validation only reveals presence.
func envSet(name string) bool {
	return name != "" && os.Getenv(name) != ""
}

// --- individual checks -------------------------------------------------------

func checkConfig(v *validator) *embedfs.Config {
	cfg, err := embedfs.LoadConfig()
	if err != nil {
		v.record(sevFail, "config load", "err", err)
		return nil
	}
	v.record(sevPass, "config load", "app", cfg.Pocketbase.App.Name)
	return cfg
}

func checkDBPaths(v *validator, app core.App) {
	dataDir := app.DataDir()
	if err := probeWriteable(dataDir); err != nil {
		v.record(sevFail, "pb_data writeable", "dir", dataDir, "err", err)
	} else {
		v.record(sevPass, "pb_data writeable", "dir", dataDir)
	}
	pub := filepath.Join(filepath.Dir(dataDir), "pb_public")
	if fi, err := os.Stat(pub); err != nil || !fi.IsDir() {
		v.record(sevFail, "pb_public present", "dir", pub, "hint", "build the SPA: nabu/scripts/build.sh")
	} else {
		v.record(sevPass, "pb_public present", "dir", pub)
	}
}

// probeWriteable creates and removes a temp file to prove dir is writeable.
func probeWriteable(dir string) error {
	f, err := os.CreateTemp(dir, ".nabu-validate-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func checkSuperuser(v *validator, app core.App, cfg *embedfs.Config) {
	if supers, err := app.FindAllRecords(core.CollectionNameSuperusers); err == nil && len(supers) > 0 {
		v.record(sevPass, "superuser exists", "count", len(supers))
		return
	}
	su := superuserCfg(cfg)
	if su.Email != "" && su.PasswordEnv != "" {
		if envSet(su.PasswordEnv) {
			v.record(sevPass, "superuser configured (none yet, can be created)",
				"email", su.Email, "password_env", su.PasswordEnv)
			return
		}
		v.record(sevFail, "superuser configured but password env unset",
			"email", su.Email, "password_env", su.PasswordEnv)
		return
	}
	v.record(sevFail, "no superuser and none configured",
		"hint", "run `nabu seed` (demo), `nabu users import` a roster with a `role: superuser` entry, or open /_/ to create the first superuser")
}

func checkContent(v *validator, dir string) []content.Course {
	if _, err := os.Stat(dir); err != nil {
		v.record(sevWarn, "content tree", "dir", dir, "detail", "not found; skipping content checks")
		return nil
	}
	r, err := content.NewRenderer("goldmark")
	if err != nil {
		v.record(sevFail, "content renderer", "err", err)
		return nil
	}
	courses, err := content.Load(source.Disk(dir), r, content.Options{})
	if err != nil {
		v.record(sevFail, "content load", "dir", dir, "err", err)
		return nil
	}
	probs := content.Problems(courses)
	for _, p := range probs {
		if p.Status == content.StatusWarnings {
			v.record(sevWarn, "content warning", "path", p.Path, "issue", p.Issue)
		} else { // ERRORS or CORRUPT
			v.record(sevFail, "content error", "path", p.Path, "content_status", string(p.Status), "issue", p.Issue)
		}
	}
	if len(probs) == 0 {
		v.record(sevPass, "content parses clean", "dir", dir, "courses", len(courses))
	}
	return courses
}

func checkEnv(v *validator, cfg *embedfs.Config, n needs) {
	// S3 resource store — required when any lesson declares a bucket.
	if len(n.buckets) > 0 {
		for _, name := range []string{"S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
			if envSet(name) {
				v.record(sevPass, "env present", "var", name)
			} else {
				v.record(sevFail, "env missing", "var", name, "needed_by", "lesson S3 bucket")
			}
		}
		for _, name := range []string{"S3_REGION", "S3_PUBLIC_BASE"} {
			if !envSet(name) {
				v.record(sevPass, "env optional (unset, default used)", "var", name)
			} else {
				v.record(sevPass, "env present", "var", name)
			}
		}
	} else {
		v.record(sevPass, "no S3 buckets referenced by content", "detail", "S3 env not required")
	}

	// SMTP + backup secrets — required only when those features are enabled.
	if cfg != nil {
		if cfg.Pocketbase.SMTP.Enabled {
			requireEnv(v, cfg.Pocketbase.SMTP.PasswordEnv, "SMTP enabled")
		}
		if cfg.Pocketbase.Backups.S3.Enabled {
			requireEnv(v, cfg.Pocketbase.Backups.S3.PasswordEnv, "S3 backups enabled")
		}
	}

	// Worker — only a warning: agents degrade to an offline sample without it.
	if n.agents && !envSet("WORKER_HEALTH_URL") && !envSet("WORKER_DISPATCH_URL") {
		v.record(sevWarn, "worker not configured", "vars", "WORKER_HEALTH_URL/WORKER_DISPATCH_URL",
			"detail", "content uses agents; they will show offline until a worker is set")
	}
}

// requireEnv fails when a named env var (from config) is missing.
func requireEnv(v *validator, name, why string) {
	if name == "" {
		v.record(sevFail, "password_env not named in config", "needed_by", why)
		return
	}
	if envSet(name) {
		v.record(sevPass, "env present", "var", name, "needed_by", why)
	} else {
		v.record(sevFail, "env missing", "var", name, "needed_by", why)
	}
}

func checkS3(ctx context.Context, v *validator, n needs, deep bool) {
	if len(n.buckets) == 0 {
		return // nothing to reach; checkEnv already noted S3 isn't required
	}
	svc := s3svc.New()
	if !svc.Configured() {
		v.record(sevFail, "s3 reachability skipped", "detail", "s3 not configured (see env FAILs above)")
		return
	}
	for _, b := range n.buckets {
		if err := svc.HeadBucket(ctx, b); err != nil {
			v.record(sevFail, "s3 bucket reachable", "bucket", b, "err", err)
			continue
		}
		v.record(sevPass, "s3 bucket reachable", "bucket", b)
		if deep {
			key := fmt.Sprintf("nabu-validate/probe-%d", time.Now().UnixNano())
			if err := svc.PutProbe(ctx, b, key); err != nil {
				v.record(sevFail, "s3 write probe", "bucket", b, "err", err)
				continue
			}
			if err := svc.Delete(ctx, b, key); err != nil {
				v.record(sevWarn, "s3 probe cleanup", "bucket", b, "key", key, "err", err)
			} else {
				v.record(sevPass, "s3 write probe", "bucket", b)
			}
		}
	}
}

// superuserCfg safely extracts the superuser config (cfg may be nil on a config
// load failure).
func superuserCfg(cfg *embedfs.Config) embedfs.SuperuserConfig {
	if cfg == nil {
		return embedfs.SuperuserConfig{}
	}
	return cfg.Pocketbase.Superuser
}
