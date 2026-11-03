// Package admin groups Nabu's administrative CLI operations under a single
// `admin` parent command. The grouping is deliberate: these actions change
// server-, storage- or account-level state, so keeping them off the top level
// (and behind an extra word) makes them harder to run by accident and easier to
// find when you do want them.
package admin

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"github.com/mrmxf/nabu/embedfs"
	"github.com/mrmxf/nabu/internal/s3svc"
	"github.com/mrmxf/nabu/internal/students"
)

// NewCommand returns the `admin` command tree. Running `admin` on its own prints
// help rather than doing anything — every real action is a named subcommand.
func NewCommand(app core.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Grouped administrative operations (kept off the top level to avoid accidents)",
		Long: "Administrative operations for Nabu operators.\n\n" +
			"These commands change server, storage or account state. They are grouped\n" +
			"under `admin` so they are hard to trigger by accident and easy to discover.\n" +
			"Run a subcommand explicitly, e.g. `nabu admin s3 set-cors --bucket <name>`.\n\n" +
			"Commands that need secrets (e.g. `s3 set-cors`) read them from the\n" +
			"environment; wrap the call with Infisical to inject them, e.g.\n" +
			"  infisical run --env prod -- nabu admin s3 set-cors --bucket MyBucket",
	}
	// `validate` (ops), `s3 set-cors` (set), and `backup`/`restore` (archival) —
	// flat verbs sharing one contract (see newValidateCommand). backup/restore
	// constructors stay in internal/students so they can reuse its helpers.
	cmd.AddCommand(
		newValidateCommand(app),
		newSettingsCommand(app),
		newShowConfigCommand(),
		newS3Command(app),
		students.NewBackupCommand(app),
		students.NewRestoreCommand(app),
	)
	return cmd
}

// newSettingsCommand is the `admin settings` subcategory: PocketBase Settings
// driven from the nabu-config.yaml pocketbase.* scaffold.
func newSettingsCommand(app core.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Apply PocketBase settings from the embedded config",
	}
	cmd.AddCommand(newSettingsApplyCommand(app))
	return cmd
}

// newS3Command is the `admin s3` subcategory: resource-bucket administration.
func newS3Command(app core.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "s3",
		Short: "Resource-bucket (S3) administration",
	}
	cmd.AddCommand(newSetCORSCommand(app))
	return cmd
}

// newSetCORSCommand implements `admin s3 set-cors`: apply the embedded CORS
// policy to a bucket so browsers may upload directly via presigned URLs.
func newSetCORSCommand(app core.App) *cobra.Command {
	var bucket string
	var assumeYes bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set-cors",
		Short: "Apply the embedded CORS policy to a resource bucket",
		Long: "Apply the CORS policy from nabu/embedfs (admin.s3.cors-configfile) to a\n" +
			"bucket, allowing direct browser uploads via presigned PUT URLs.\n\n" +
			"Requires S3 credentials in the environment (as for `serve`). Inject them\n" +
			"with Infisical rather than exporting by hand:\n" +
			"  infisical run --env prod -- nabu admin s3 set-cors --bucket MyBucket\n\n" +
			"Overwrites any existing CORS policy on the bucket, so it prompts for\n" +
			"confirmation unless --yes is given; use --dry-run to preview the policy\n" +
			"without applying it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if bucket == "" {
				return fmt.Errorf("--bucket is required")
			}

			// admin.s3.cors-configfile -> the embedded policy JSON.
			corsJSON, err := embedfs.CORSConfigJSON()
			if err != nil {
				return err
			}

			if dryRun {
				fmt.Printf("CORS policy for bucket %q (dry run — not applied):\n%s\n", bucket, corsJSON)
				return nil
			}

			// Accident suppression: this overwrites the live bucket policy, so make
			// the operator retype the bucket name unless they pass --yes.
			if !assumeYes {
				if !confirmBucket(bucket) {
					fmt.Println("aborted — bucket name did not match")
					return nil
				}
			}

			svc := s3svc.New()
			if !svc.Configured() {
				return fmt.Errorf("s3 not configured: set S3_ENDPOINT and credentials in the environment")
			}
			if err := svc.PutBucketCORS(cmd.Context(), bucket, corsJSON); err != nil {
				return err
			}
			fmt.Printf("CORS policy applied to bucket %q\n", bucket)
			return nil
		},
	}
	cmd.Flags().StringVar(&bucket, "bucket", "", "target resource bucket (required)")
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the policy instead of applying it")
	return cmd
}

// confirmBucket asks the operator to retype the bucket name and returns true only
// on an exact match.
func confirmBucket(bucket string) bool {
	fmt.Printf("About to overwrite the CORS policy on bucket %q.\n", bucket)
	fmt.Print("Retype the bucket name to confirm: ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return false
	}
	return strings.TrimSpace(sc.Text()) == bucket
}
