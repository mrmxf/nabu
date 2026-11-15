package admin

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/agentsvc"
	"github.com/mrmxf/nabu/internal/classroom"
	"github.com/mrmxf/nabu/internal/seed"
)

// Deployment modes `nabu doctor --mode` checks for.
const (
	ModeSaaS      = "saas"
	ModeClassroom = "classroom"
	ModeSelfHost  = "selfhost"
)

// DefaultBuildDir is where `clog build` (and the nabu init templates) put the
// static site that `doctor --mode saas` looks for.
const DefaultBuildDir = "_clog_build/public"

// doctorOpts are `nabu doctor`'s flags.
type doctorOpts struct {
	mode     string
	verbose  bool
	deep     bool
	buildDir string
	image    string
}

// NewDoctorCommand returns `nabu doctor`: `admin validate` plus readiness checks
// for one deployment mode. It prints nothing when everything is ready, the
// warnings and failures otherwise (each with a fix), and exits non-zero on any
// failure.
func NewDoctorCommand(app core.App, env Env) *cobra.Command {
	var o doctorOpts
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check this machine is ready for a deployment mode (silent when ready)",
		Long: "Run the admin validate checks (config, pb_data, web app, superuser, content,\n" +
			"secrets, S3), then the readiness checks for --mode:\n\n" +
			"  saas       static build present (" + DefaultBuildDir + "), Cloudflare token and\n" +
			"             account id, media store reachable, agent worker on https with a token\n" +
			"  classroom  web app built in, a LAN address students can reach, and every\n" +
			"             agent job able to run offline (sim_output_fallback) or a worker up\n" +
			"  selfhost   pb_data writeable, and the container image present (or running\n" +
			"             inside a container)\n\n" +
			"Silent when ready. Warnings and failures print with a fix; -v also prints\n" +
			"each passing check. Exit 1 on any failure. Secrets are never printed:\n" +
			"  infisical run --env prod -- nabu doctor --mode saas",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.Context(), app, env, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.mode, "mode", "", "deployment mode to check: saas|classroom|selfhost (default: the common checks only)")
	f.BoolVarP(&o.verbose, "verbose", "v", false, "also print each passing check")
	f.BoolVar(&o.deep, "deep", false, "also write and delete a probe object to prove S3 write access")
	f.StringVar(&o.buildDir, "build-dir", DefaultBuildDir, "static build to check in saas mode")
	f.StringVar(&o.image, "image", "", "container image to look for in selfhost mode, e.g. ghcr.io/mrmxf/nabu:v0.1.3")
	return cmd
}

func runDoctor(ctx context.Context, app core.App, env Env, o doctorOpts) error {
	switch o.mode {
	case "", ModeSaaS, ModeClassroom, ModeSelfHost:
	default:
		return fmt.Errorf("doctor: unknown --mode %q: use saas, classroom or selfhost", o.mode)
	}
	v := &validator{hidePass: !o.verbose}
	tree, err := seed.ContentTree(seed.Paths{}, env.Sources)
	if err != nil {
		return err
	}
	courses := runChecks(ctx, app, v, checkOpts{deep: o.deep, content: tree, web: env.Web, static: o.mode == ModeSaaS})
	n := deriveNeeds(courses)
	switch o.mode {
	case ModeSaaS:
		checkStaticBuild(v, o.buildDir)
		checkCloudflareEnv(v)
		checkMediaStore(v)
		checkSaaSWorker(v, n)
	case ModeClassroom:
		checkWebBuiltIn(v, env.Web)
		addrs, err := classroom.LANAddrs()
		checkLAN(v, addrs, err)
		gaps := classroom.OfflineGaps(courses)
		var health agentsvc.Health
		if len(gaps) > 0 {
			pctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			health = agentsvc.New().Health(pctx)
			cancel()
		}
		checkOffline(v, gaps, health)
	case ModeSelfHost:
		checkImage(v, o.image, exec.LookPath, inContainer, runImageInspect)
	}
	v.summary()
	if v.failed() {
		return fmt.Errorf("doctor: %d check(s) failed", v.fails)
	}
	return nil
}

// checkStaticBuild needs a `nabu build` output: the web app, the content index
// and the _headers CORS rule.
func checkStaticBuild(v *validator, dir string) {
	site := os.DirFS(dir)
	if !hasIndex(site) {
		v.record(sevFail, "static build missing", "dir", dir,
			"fix", "run clog build, or nabu build --strict --out "+dir)
		return
	}
	if _, err := fs.Stat(site, "content/index.json"); err != nil {
		v.record(sevFail, "static build has no content index", "file", filepath.Join(dir, "content", "index.json"),
			"fix", "rebuild with nabu build --strict --out "+dir)
		return
	}
	if _, err := fs.Stat(site, seed.HeadersFile); err != nil {
		v.record(sevWarn, "static build has no _headers", "dir", dir,
			"detail", "other sites cannot read /content/index.json; rebuild with this nabu")
	}
	v.record(sevPass, "static build present", "dir", dir)
}

// checkCloudflareEnv needs the two values wrangler deploys with.
func checkCloudflareEnv(v *validator) {
	for _, name := range []string{"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID"} {
		if envSet(name) {
			v.record(sevPass, "env present", "var", name)
		} else {
			v.record(sevFail, "env missing", "var", name, "needed_by", "Cloudflare deploy",
				"fix", "wrap the command: infisical run --env prod -- nabu doctor --mode saas")
		}
	}
}

// checkMediaStore checks course.media, when set, is a folder this host can read
// (typically a mounted bucket).
func checkMediaStore(v *validator) {
	cfg, err := embedfs.LoadConfig()
	if err != nil {
		return // checkConfig has already failed
	}
	if cfg.Course.Media == "" {
		v.record(sevPass, "no media store configured", "detail", "course.media is empty")
		return
	}
	dir := embedfs.ResolvePath(cfg.Course.Media)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		v.record(sevFail, "media store not reachable", "dir", dir,
			"fix", "mount the media bucket there, or fix course.media in nabu.yaml")
		return
	}
	v.record(sevPass, "media store reachable", "dir", dir)
}

// checkSaaSWorker needs a public worker on https with a dispatch token, when
// the content has agents.
func checkSaaSWorker(v *validator, n needs) {
	if !n.agents {
		v.record(sevPass, "no agents in the content", "detail", "no worker needed")
		return
	}
	dispatch := os.Getenv(agentsvc.EnvWorkerDispatchURL)
	if dispatch == "" {
		if h := os.Getenv(agentsvc.EnvWorkerHealthURL); h != "" {
			dispatch = strings.TrimSuffix(h, "/healthz") + "/v1/jobs/dispatch"
		}
	}
	if dispatch == "" {
		v.record(sevFail, "worker not configured", "vars", agentsvc.EnvWorkerHealthURL+"/"+agentsvc.EnvWorkerDispatchURL,
			"fix", "set the worker URL; see help:reference#agent-deploy")
		return
	}
	u, err := url.Parse(dispatch)
	if err != nil || u.Scheme != "https" {
		v.record(sevFail, "worker not on https", "url", dispatch,
			"fix", "serve the worker over https: a SaaS deploy reaches it across the internet")
		return
	}
	if !envSet(agentsvc.EnvDispatchToken) {
		v.record(sevFail, "env missing", "var", agentsvc.EnvDispatchToken, "needed_by", "worker dispatch",
			"fix", "set the same token on nabu and the worker")
		return
	}
	v.record(sevPass, "worker on https with a token", "host", u.Host)
}

// checkWebBuiltIn needs the web app inside the binary: a classroom laptop may
// have no copy on disk and no internet to fetch one.
func checkWebBuiltIn(v *validator, web fs.FS) {
	if hasIndex(web) {
		v.record(sevPass, "web app built into the binary")
		return
	}
	v.record(sevWarn, "web app not built into this binary", "detail", "serving pb_public/ from disk",
		"fix", "use a release binary, or nabu web fetch before going offline")
}

// checkLAN needs an address students on the room's network can reach.
func checkLAN(v *validator, addrs []string, err error) {
	if err != nil {
		v.record(sevFail, "cannot list network interfaces", "err", err)
		return
	}
	if len(addrs) == 0 {
		v.record(sevFail, "no LAN address", "fix", "join the room's Wi-Fi, or start a hotspot on this laptop")
		return
	}
	v.record(sevPass, "LAN address found", "addrs", strings.Join(addrs, ","))
}

// checkOffline fails each agent job that cannot run with no worker, unless a
// worker answers now.
func checkOffline(v *validator, gaps []classroom.Gap, h agentsvc.Health) {
	if len(gaps) == 0 {
		v.record(sevPass, "every agent job runs offline")
		return
	}
	if h.Alive {
		v.record(sevPass, "agent worker reachable", "jobs_needing_it", len(gaps))
		return
	}
	for _, g := range gaps {
		v.record(sevFail, "agent job needs a worker", "job", g.Where, "agent", g.Agent, "why", g.Why,
			"fix", "add sim_output_fallback: true and a sim_output to the slide, or start a worker")
	}
}

// inContainer reports whether this process runs inside podman or docker.
func inContainer() bool {
	for _, f := range []string{"/run/.containerenv", "/.dockerenv"} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

// runImageInspect reports whether the container runtime rt has image locally.
func runImageInspect(rt, image string) bool {
	return exec.Command(rt, "image", "inspect", image).Run() == nil // #nosec G204 -- rt is podman or docker from LookPath
}

// checkImage needs the self-hosted container image, or to be running in it.
func checkImage(v *validator, image string, lookPath func(string) (string, error), inside func() bool, inspect func(rt, image string) bool) {
	if inside() {
		v.record(sevPass, "running inside a container")
		return
	}
	var rt string
	for _, name := range []string{"podman", "docker"} {
		if _, err := lookPath(name); err == nil {
			rt = name
			break
		}
	}
	if rt == "" {
		v.record(sevFail, "no container runtime", "fix", "install podman (or docker)")
		return
	}
	if image == "" {
		v.record(sevWarn, "container image not checked", "runtime", rt,
			"fix", "name it: nabu doctor --mode selfhost --image <image:tag>")
		return
	}
	if !inspect(rt, image) {
		v.record(sevFail, "container image not present", "image", image, "runtime", rt,
			"fix", rt+" pull "+image)
		return
	}
	v.record(sevPass, "container image present", "image", image, "runtime", rt)
}
