// Package embedfs holds Nabu's build-time configuration: files baked into the
// binary and a small typed loader over github.com/mrmxf/util/kfg (koanf).
//
// The config is deliberately split in two so each part stays auditable on its
// own: nabu-config.yaml carries structured settings (the admin.* tree), and it
// points at standalone data files — e.g. admin.s3.cors-configfile names the S3
// CORS policy JSON, which is kept in aws-CLI shape so it can also be applied with
// `aws s3api put-bucket-cors` if needed.
package embedfs

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"testing/fstest"

	"github.com/mrmxf/util/kfg"
)

// FS is the embedded config filesystem. Add files to the //go:embed list below
// when nabu-config.yaml starts referencing them.
//
//go:embed nabu-config.yaml s3-cors-config.json
var FS embed.FS

// configFile is the root settings document within FS.
const configFile = "nabu-config.yaml"

// OverrideFile, when set (by the root `--config`/`-c` flag), names a partial YAML
// file on the local filesystem that is merged over the embedded defaults — koanf
// deep-merges, so it need only carry the keys it wants to change.
var OverrideFile string

// CourseDir, when set (by the root `--course-dir`/`-C` flag), names a course repo
// whose CourseFile is the override. Ignored when OverrideFile is set.
var CourseDir string

// OverrideYAML, when set (by nabu.Options.ConfigYAML), is a course config baked
// into a library-built binary. It is merged over the embedded defaults and UNDER
// any file override, so an operator's -c/-C/./nabu.yaml still wins at deploy
// time. Relative paths in it resolve against the working directory.
var OverrideYAML []byte

// CourseFile is the conventional name of a course's config, looked for in
// CourseDir or, failing that, the working directory.
const CourseFile = "nabu.yaml"

// EffectiveOverride returns the config file merged over the embedded defaults:
// OverrideFile if set, else CourseDir/nabu.yaml, else ./nabu.yaml when present,
// else "" (embedded defaults only). A CourseDir without a nabu.yaml is not an
// error here; loadRaw reports it.
func EffectiveOverride() string {
	switch {
	case OverrideFile != "":
		return OverrideFile
	case CourseDir != "":
		return filepath.Join(CourseDir, CourseFile)
	}
	if _, err := os.Stat(CourseFile); err == nil {
		return CourseFile
	}
	return ""
}

// Config mirrors nabu-config.yaml. Only the subtrees Nabu reads are modelled.
type Config struct {
	// Logopath is the default navbar logo, served by basename (see nabu-config.yaml).
	Logopath string `koanf:"logopath"`
	// DemoMode controls whether `nabu seed` spawns the demo accounts from the
	// demo roster (see CourseConfig.DemoRoster). Seeded into app_config `demo_mode`.
	DemoMode bool `koanf:"demo_mode"`
	// Course locates the course's files (see CourseConfig).
	Course CourseConfig `koanf:"course"`
	// ClassroomFastLogin turns on the name-only classroom sign-in (see
	// internal/httpapi/classroom.go). Seeded into app_config
	// `classroom_fast_login`; OFF unless explicitly enabled.
	ClassroomFastLogin bool             `koanf:"classroom_fast_login"`
	Signup             SignupConfig     `koanf:"signup"`
	AI                 AIConfig         `koanf:"ai"`
	Admin              AdminConfig      `koanf:"admin"`
	Pocketbase         PocketbaseConfig `koanf:"pocketbase"`
}

// CourseConfig is the course.* subtree — where a course keeps its files. Every
// path is relative to the config file that sets it (see ResolvePath), so a course
// repo's nabu.yaml works wherever nabu is launched from. Command-line flags
// (e.g. `seed --content`) override these.
type CourseConfig struct {
	// Content is the authored content tree (content/<course>/<lesson>/…).
	Content string `koanf:"content"`
	// Assets is the Hugo-style assets tree published into pb_public.
	Assets string `koanf:"assets"`
	// Schedules is the lesson-schedules YAML pre-load (skipped if absent).
	Schedules string `koanf:"schedules"`
	// DemoRoster is the course's demo roster. Empty => the course-neutral roster
	// embedded in internal/demo.
	DemoRoster string `koanf:"demo_roster"`
	// Media is the media store (e.g. an S3 mount) that `serve` publishes at
	// /content/<course>/<lesson>/media/<asset>, from <media>/<course>/<lesson>/media/<asset>.
	// Empty => no media route. `serve --media-dir` overrides it.
	Media string `koanf:"media"`
}

// AIConfig is the ai.* subtree — which AI back-ends this deployment exposes.
// StableDiffusion.Enabled is seeded into the app_config `images_enabled` row (see
// internal/seed) so the SPA can hide every image surface and the job API can
// refuse image submits. Ollama.Enabled is modelled for symmetry but not yet wired.
type AIConfig struct {
	Ollama          AIBackend `koanf:"ollama"`
	StableDiffusion AIBackend `koanf:"stable-diffusion"`
}

// AIBackend is a single ai.<name>.* back-end toggle.
type AIBackend struct {
	Enabled bool `koanf:"enabled"`
}

// SignupConfig is the signup.* subtree — behaviour of the public self-signup
// page. Seeded into app_config so the SPA can read it.
type SignupConfig struct {
	// RequireEmail controls whether a new account must supply an email address.
	// false => email is optional (username + password is enough).
	RequireEmail bool `koanf:"require_email"`
}

// AdminConfig is the admin.* subtree — settings for the grouped `admin` command
// tree (see internal/admin).
type AdminConfig struct {
	S3 S3Config `koanf:"s3"`
}

// S3Config is admin.s3.* — resource-bucket administration settings.
type S3Config struct {
	// CorsConfigFile is the filename (within FS) of the CORS policy JSON applied
	// by `nabu admin s3 set-cors`.
	CorsConfigFile string `koanf:"cors-configfile"`
}

// PocketbaseConfig is the pocketbase.* subtree. Most of it is a documented
// scaffold for a future `nabu admin settings apply`; only the fields below are
// read today — by `nabu admin validate` to decide which secrets are required.
type PocketbaseConfig struct {
	App       AppConfig       `koanf:"app"`
	Superuser SuperuserConfig `koanf:"superuser"`
	SMTP      SMTPConfig      `koanf:"smtp"`
	Backups   BackupsConfig   `koanf:"backups"`
}

// AppConfig is pocketbase.app.* — application identity.
type AppConfig struct {
	Name string `koanf:"name"`
	URL  string `koanf:"url"`
}

// SuperuserConfig is pocketbase.superuser.* — the initial superuser gate. The
// password is never stored; PasswordEnv names the env var that holds it.
type SuperuserConfig struct {
	Email       string `koanf:"email"`
	PasswordEnv string `koanf:"password_env"`
}

// SMTPConfig is pocketbase.smtp.* — mail settings applied by `admin settings
// apply` and read by `admin validate`. The password is never stored; PasswordEnv
// names the env var that holds it.
type SMTPConfig struct {
	Enabled       bool   `koanf:"enabled"`
	Host          string `koanf:"host"`
	Port          int    `koanf:"port"`
	Username      string `koanf:"username"`
	PasswordEnv   string `koanf:"password_env"`
	TLS           bool   `koanf:"tls"`
	SenderName    string `koanf:"sender_name"`
	SenderAddress string `koanf:"sender_address"`
}

// BackupsConfig is pocketbase.backups.* — the auto-backup schedule plus the S3
// target for backups.
type BackupsConfig struct {
	Cron        string          `koanf:"cron"`
	CronMaxKeep int             `koanf:"cron_max_keep"`
	S3          BackupsS3Config `koanf:"s3"`
}

// BackupsS3Config is pocketbase.backups.s3.* — the S3 store for backups. The
// secret key is never stored; PasswordEnv names the env var that holds it.
type BackupsS3Config struct {
	Enabled        bool   `koanf:"enabled"`
	Bucket         string `koanf:"bucket"`
	Region         string `koanf:"region"`
	Endpoint       string `koanf:"endpoint"`
	AccessKey      string `koanf:"access_key"`
	PasswordEnv    string `koanf:"password_env"`
	ForcePathStyle bool   `koanf:"force_path_style"`
}

// LoadConfig reads the embedded nabu-config.yaml (plus any --config override) via
// kfg and returns the typed configuration.
func LoadConfig() (*Config, error) {
	if err := loadRaw(); err != nil {
		return nil, err
	}
	cfg, err := kfg.Unmarshal[Config]("", "koanf")
	if err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", configFile, err)
	}
	return cfg, nil
}

// loadRaw loads the embedded defaults into kfg.Raw and, if OverrideFile is set,
// deep-merges that local partial file on top. kfg's own auto-merge/app/releases
// are disabled so the only inputs are the embedded file and the explicit override.
func loadRaw() error {
	if err := kfg.Konfigure(&kfg.KonfigureOpt{
		AppFs:               FS,
		FilePath:            configFile,
		PreventAutoMerge:    true,
		PreventAutoApp:      true,
		PreventAutoReleases: true,
	}); err != nil {
		return fmt.Errorf("konfigure %s: %w", configFile, err)
	}
	if len(OverrideYAML) > 0 {
		baked := fstest.MapFS{CourseFile: &fstest.MapFile{Data: OverrideYAML}}
		if err := kfg.MergeKonfig(&kfg.KonfigureOpt{AppFs: baked, FilePath: CourseFile}); err != nil {
			return fmt.Errorf("merge baked course config: %w", err)
		}
	}
	if override := EffectiveOverride(); override != "" {
		if _, err := os.Stat(override); err != nil {
			return fmt.Errorf("config override %q: %w", override, err)
		}
		dir, base := filepath.Split(override)
		if dir == "" {
			dir = "."
		}
		if err := kfg.MergeKonfig(&kfg.KonfigureOpt{AppFs: os.DirFS(dir), FilePath: base}); err != nil {
			return fmt.Errorf("merge config override %q: %w", override, err)
		}
	}
	return nil
}

// ResolvePath resolves a path named in the config: absolute paths are returned
// as-is, relative ones are taken relative to the directory of the effective
// override file (see EffectiveOverride), so a course's nabu.yaml can name its
// own files wherever nabu is launched from. With no override, relative paths
// stay relative to the working directory.
func ResolvePath(p string) string {
	override := EffectiveOverride()
	if p == "" || filepath.IsAbs(p) || override == "" {
		return p
	}
	return filepath.Join(filepath.Dir(override), p)
}

// RawMerged returns the effective configuration (embedded defaults + any --config
// override) as a nested map, for `admin showconfig`.
func RawMerged() (map[string]any, error) {
	if err := loadRaw(); err != nil {
		return nil, err
	}
	return kfg.Raw.Raw(), nil
}

// CORSConfigJSON resolves admin.s3.cors-configfile and returns the raw JSON bytes
// of that embedded policy file, ready to hand to the S3 client.
func CORSConfigJSON() ([]byte, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	name := cfg.Admin.S3.CorsConfigFile
	if name == "" {
		return nil, fmt.Errorf("admin.s3.cors-configfile is not set in %s", configFile)
	}
	data, err := FS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read cors policy %q: %w", name, err)
	}
	return data, nil
}
