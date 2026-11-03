package students

import (
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// NewCommand returns the `users` command tree: roster export/import. Full
// backup/restore live under `admin` (see NewBackupCommand/NewRestoreCommand),
// grouped there with the other admin-only operations. It runs on the server
// (implicitly admin), like `seed`.
func NewCommand(app core.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage users: roster export/import (students, teachers, admins, superusers)",
	}
	cmd.AddCommand(exportCmd(app), importCmd(app))
	return cmd
}

func exportCmd(app core.App) *cobra.Command {
	var out, format, role, course string
	var positions bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a user roster (CSV/YAML/JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := resolveFormat(format, out, FormatCSV)
			recs, err := Export(app, ExportFilter{Role: role, Course: course}, positions)
			if err != nil {
				return err
			}
			data, err := EncodeRoster(recs, f)
			if err != nil {
				return err
			}
			return writeOut(out, data)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output file (default stdout)")
	cmd.Flags().StringVar(&format, "format", "", "csv|yaml|json (default from --out extension, else csv)")
	cmd.Flags().StringVar(&role, "role", "", "filter by role")
	cmd.Flags().StringVar(&course, "course", "", "filter by course slug")
	cmd.Flags().StringVar(&course, "class", "", "deprecated alias for --course")
	_ = cmd.Flags().MarkDeprecated("class", "use --course")
	cmd.Flags().BoolVar(&positions, "positions", false, "include student positions (yaml/json)")
	return cmd
}

func importCmd(app core.App) *cobra.Command {
	var from, format, mode, course string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a user roster with merge|overwrite|delete-orphans",
		Long: "Import a user roster (CSV/YAML/JSON) under the given mode.\n\n" +
			"A `role: superuser` entry is applied to the dashboard _superusers instead\n" +
			"of `users`; its password must come from `password_env` (injected by\n" +
			"Infisical), never a literal — wrap with Infisical when importing one:\n" +
			"  infisical run --env prod -- nabu users import --from roster.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" {
				return fmt.Errorf("--from is required")
			}
			// Apply pending migrations first so importing into a brand-new data dir
			// works in one step (the users/enrolments collections must exist).
			// Idempotent; the HTTP import path skips this since the server, once
			// serving, is already migrated.
			if err := app.RunAppMigrations(); err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}
			data, err := os.ReadFile(from)
			if err != nil {
				return err
			}
			recs, err := DecodeRoster(data, resolveFormat(format, from, FormatCSV))
			if err != nil {
				return err
			}
			sum, err := Apply(app, recs, Mode(mode), course)
			if err != nil {
				return err
			}
			printSummary(sum)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "input file (required)")
	cmd.Flags().StringVar(&format, "format", "", "csv|yaml|json (default from --from extension)")
	cmd.Flags().StringVar(&mode, "mode", string(ModeMerge), "merge|overwrite|delete-orphans")
	cmd.Flags().StringVar(&course, "course", "", "scope course slug for delete-orphans")
	cmd.Flags().StringVar(&course, "class", "", "deprecated alias for --course")
	_ = cmd.Flags().MarkDeprecated("class", "use --course")
	return cmd
}

// NewBackupCommand returns the `backup` command. It lives under `admin` (it emits
// password hashes) but stays in this package so it can use the shared roster
// helpers and the Backup/EncodeBundle logic directly.
func NewBackupCommand(app core.App) *cobra.Command {
	var out, format string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Full user-data backup incl. password hashes (YAML/JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := Backup(app)
			if err != nil {
				return err
			}
			data, err := EncodeBundle(b, resolveFormat(format, out, FormatYAML))
			if err != nil {
				return err
			}
			return writeOut(out, data)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output file (default stdout)")
	cmd.Flags().StringVar(&format, "format", "", "yaml|json (default from --out extension, else yaml)")
	return cmd
}

// NewRestoreCommand returns the `restore` command, mounted under `admin`
// alongside NewBackupCommand (see that function).
func NewRestoreCommand(app core.App) *cobra.Command {
	var from, format string
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore a full user-data backup into the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" {
				return fmt.Errorf("--from is required")
			}
			data, err := os.ReadFile(from)
			if err != nil {
				return err
			}
			b, err := DecodeBundle(data, resolveFormat(format, from, FormatYAML))
			if err != nil {
				return err
			}
			sum, err := Restore(app, b)
			if err != nil {
				return err
			}
			printSummary(sum)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "input file (required)")
	cmd.Flags().StringVar(&format, "format", "", "yaml|json (default from --from extension, else yaml)")
	return cmd
}

// resolveFormat picks an explicit format, else infers from the filename, else
// falls back to def.
func resolveFormat(explicit, filename string, def Format) Format {
	if explicit != "" {
		return Format(explicit)
	}
	if f := FormatFromExt(filename); f != "" {
		return f
	}
	return def
}

func writeOut(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printSummary(s Summary) {
	fmt.Printf("created=%d updated=%d enrolled=%d unenrolled=%d deleted=%d superusers=%d skipped=%d\n",
		s.Created, s.Updated, s.Enrolled, s.Unenrolled, s.Deleted, s.Superusers, s.Skipped)
	for _, e := range s.Errors {
		fmt.Printf("  error: %s\n", e)
	}
}
