package admin

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/serverctl"
)

// newSettingsApplyCommand implements `admin settings apply`: push the
// pocketbase.* block of nabu-config.yaml into PocketBase's own Settings (app
// identity, SMTP, and the backup schedule/target). It is the declarative
// counterpart to `admin validate` — validate checks the config's needs are met;
// apply writes the config into the live settings.
//
// Contract (shared by all admin verbs): verbose by default; -q prints only
// failures; exit 0 on success, 1 on failure; secrets are never printed (SMTP and
// S3-backup passwords come from their password_env vars and show as «set»/«empty»,
// never their value). Changing settings writes the local DB, so it refuses to run
// under a live server unless --force.
func newSettingsApplyCommand(app core.App) *cobra.Command {
	var dryRun, assumeYes, quiet, force bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply pocketbase.* config to PocketBase settings (app, SMTP, backups)",
		Long: "Write the pocketbase.* block of nabu-config.yaml into PocketBase Settings:\n" +
			"application name/URL, SMTP, and the backup schedule + S3 target. The config\n" +
			"is the source of truth for these fields; other settings are left untouched.\n\n" +
			"SMTP and S3-backup passwords are read from their password_env vars — inject\n" +
			"them with Infisical:\n" +
			"  infisical run --env prod -- nabu admin settings apply\n\n" +
			"Use --dry-run to preview the changes without saving; --yes skips the\n" +
			"confirmation prompt. Does NOT create the superuser (use `nabu seed`,\n" +
			"`nabu users import`, or PocketBase's /_/ first-run page).",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Writing settings mutates the local DB; don't race a live server.
			// A dry run reads only, so it's always allowed.
			if !dryRun {
				if running, pid := serverctl.ServerRunning(app.DataDir()); running && !force {
					return fmt.Errorf("a server is running (pid %d); stop it first (`nabu stop`) or pass --force", pid)
				}
			}

			cfg, err := embedfs.LoadConfig()
			if err != nil {
				return err
			}

			next, err := app.Settings().Clone()
			if err != nil {
				return fmt.Errorf("clone settings: %w", err)
			}

			changes, secretErrs := applyConfigToSettings(next, cfg)

			if !quiet {
				for _, c := range changes {
					marker := "same"
					if c.changed() {
						marker = "CHANGE"
					}
					slog.Info("setting", "field", c.name, "from", c.oldVal, "to", c.newVal, "status", marker)
				}
			}
			if len(secretErrs) > 0 {
				for _, e := range secretErrs {
					slog.Error("settings apply", "status", "FAIL", "detail", e)
				}
				os.Exit(1)
			}

			if dryRun {
				if !quiet {
					slog.Info("settings apply", "status", "DRY-RUN", "detail", "nothing saved")
				}
				return nil
			}

			if !assumeYes && !confirmYN("Apply these settings to PocketBase?") {
				fmt.Println("aborted")
				return nil
			}

			if err := app.Save(next); err != nil {
				return fmt.Errorf("save settings: %w", err)
			}
			if !quiet {
				slog.Info("settings applied", "status", "OK")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the changes without saving")
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print only failures")
	cmd.Flags().BoolVar(&force, "force", false, "apply even if a server is running (races the live DB)")
	return cmd
}

// fieldChange is one managed setting's before/after (display) values.
type fieldChange struct {
	name   string
	oldVal string
	newVal string
}

func (f fieldChange) changed() bool { return f.oldVal != f.newVal }

// applyConfigToSettings mutates s to match cfg.Pocketbase and returns the list of
// managed fields (for display) plus any secret-resolution errors (an enabled
// feature whose password_env is unset). Secret values are never placed in the
// returned display strings.
func applyConfigToSettings(s *core.Settings, cfg *embedfs.Config) (changes []fieldChange, secretErrs []string) {
	pb := cfg.Pocketbase

	// setStr records a string field and applies it only when non-empty guard says
	// so (used to avoid wiping PocketBase-required Meta fields to empty).
	setStr := func(name string, field *string, val string, skipEmpty bool) {
		old := *field
		if !(skipEmpty && val == "") {
			*field = val
		}
		changes = append(changes, fieldChange{name, old, *field})
	}
	setInt := func(name string, field *int, val int) {
		old := *field
		*field = val
		changes = append(changes, fieldChange{name, fmt.Sprint(old), fmt.Sprint(*field)})
	}
	setBool := func(name string, field *bool, val bool) {
		old := *field
		*field = val
		changes = append(changes, fieldChange{name, fmt.Sprint(old), fmt.Sprint(*field)})
	}

	// Application identity (Meta) — these are PocketBase-required, so never wipe
	// them to empty; only apply when the config supplies a value.
	setStr("meta.appName", &s.Meta.AppName, pb.App.Name, true)
	setStr("meta.appURL", &s.Meta.AppURL, pb.App.URL, true)
	setStr("meta.senderName", &s.Meta.SenderName, pb.SMTP.SenderName, true)
	setStr("meta.senderAddress", &s.Meta.SenderAddress, pb.SMTP.SenderAddress, true)

	// SMTP.
	setBool("smtp.enabled", &s.SMTP.Enabled, pb.SMTP.Enabled)
	setStr("smtp.host", &s.SMTP.Host, pb.SMTP.Host, false)
	setInt("smtp.port", &s.SMTP.Port, pb.SMTP.Port)
	setStr("smtp.username", &s.SMTP.Username, pb.SMTP.Username, false)
	setBool("smtp.tls", &s.SMTP.TLS, pb.SMTP.TLS)
	smtpPw := envValue(pb.SMTP.PasswordEnv)
	if pb.SMTP.Enabled && smtpPw == "" {
		secretErrs = append(secretErrs, fmt.Sprintf("smtp.enabled but %s is unset", nameOr(pb.SMTP.PasswordEnv, "smtp.password_env")))
	}
	oldPw := s.SMTP.Password
	s.SMTP.Password = smtpPw
	changes = append(changes, fieldChange{"smtp.password", maskSecret(oldPw), maskSecret(smtpPw)})

	// Backups schedule + S3 target.
	setStr("backups.cron", &s.Backups.Cron, pb.Backups.Cron, false)
	setInt("backups.cronMaxKeep", &s.Backups.CronMaxKeep, pb.Backups.CronMaxKeep)
	setBool("backups.s3.enabled", &s.Backups.S3.Enabled, pb.Backups.S3.Enabled)
	setStr("backups.s3.bucket", &s.Backups.S3.Bucket, pb.Backups.S3.Bucket, false)
	setStr("backups.s3.region", &s.Backups.S3.Region, pb.Backups.S3.Region, false)
	setStr("backups.s3.endpoint", &s.Backups.S3.Endpoint, pb.Backups.S3.Endpoint, false)
	setStr("backups.s3.accessKey", &s.Backups.S3.AccessKey, pb.Backups.S3.AccessKey, false)
	setBool("backups.s3.forcePathStyle", &s.Backups.S3.ForcePathStyle, pb.Backups.S3.ForcePathStyle)
	s3Secret := envValue(pb.Backups.S3.PasswordEnv)
	if pb.Backups.S3.Enabled && s3Secret == "" {
		secretErrs = append(secretErrs, fmt.Sprintf("backups.s3.enabled but %s is unset", nameOr(pb.Backups.S3.PasswordEnv, "backups.s3.password_env")))
	}
	oldSecret := s.Backups.S3.Secret
	s.Backups.S3.Secret = s3Secret
	changes = append(changes, fieldChange{"backups.s3.secret", maskSecret(oldSecret), maskSecret(s3Secret)})

	return changes, secretErrs
}

// envValue returns the value of a named env var ("" if the name is empty/unset).
func envValue(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}

// maskSecret reduces a secret to its presence, never its value.
func maskSecret(v string) string {
	if v == "" {
		return "«empty»"
	}
	return "«set»"
}

func nameOr(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}

// confirmYN prompts for a yes/no answer, defaulting to no.
func confirmYN(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(sc.Text()))
	return a == "y" || a == "yes"
}
