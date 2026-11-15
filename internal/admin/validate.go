package admin

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/internal/seed"
)

// newValidateCommand implements `admin validate`: the readiness gate. It parses
// the content tree for things that need secrets (S3 buckets, agents), checks the
// matching env vars and DB paths, confirms a superuser exists or is configured,
// and (with --deep) proves S3 write access. It is the single pre-flight both a
// first-time admin and a redeploying teacher run.
//
// Contract (shared by all admin verbs): verbose by default; -q suppresses
// PASS/WARN but FAIL always prints; exit 0 on success/warnings, 1 on any failure;
// secrets are never printed — only env-var names with set/MISSING.
func newValidateCommand(app core.App, env Env) *cobra.Command {
	var quiet, deep bool
	var contentDir string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check this box is ready to run (env, paths, superuser, content, S3)",
		Long: "Fast-fail readiness gate. Parses the content tree for secret-requiring\n" +
			"resources (S3 buckets, agents), then checks that the matching env vars are\n" +
			"set, pb_data is writeable, a built web app is present, a superuser exists or is\n" +
			"configured, and each referenced bucket is reachable.\n\n" +
			"Exit 0 on success or warnings, 1 on any failure. -q prints only failures\n" +
			"(silent on a clean run). --deep additionally writes and deletes a probe\n" +
			"object to prove S3 write access. Secrets are never printed.\n\n" +
			"Wrap with Infisical so the secrets it checks are present:\n" +
			"  infisical run --env prod -- nabu admin validate",
		// This command reports failures itself and sets its own exit code; keep
		// cobra from printing usage or re-printing the error.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := &validator{quiet: quiet}
			tree, err := seed.ContentTree(seed.Paths{Content: contentDir}, env.Sources)
			if err != nil {
				return err
			}
			runChecks(cmd.Context(), app, v, checkOpts{deep: deep, content: tree, web: env.Web})
			v.summary()
			if v.failed() {
				return fmt.Errorf("validation failed: %d check(s)", v.fails)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print only failures (silent when clean)")
	cmd.Flags().BoolVar(&deep, "deep", false, "also write+delete a probe object to prove S3 write access")
	cmd.Flags().StringVar(&contentDir, "content", "", "authored content tree to validate (default: course.content from the config)")
	return cmd
}
