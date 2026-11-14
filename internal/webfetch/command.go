package webfetch

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCommand returns `nabu web`, with `fetch` as its one subcommand. It
// touches no database, so nabu.App runs it before PocketBase bootstraps.
func NewCommand() *cobra.Command {
	web := &cobra.Command{
		Use:   "web",
		Short: "Manage the built web app (library-form courses)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("nabu web: name a subcommand (fetch); see nabu web --help")
		},
		SilenceUsage: true,
	}
	var o Options
	fetch := &cobra.Command{
		Use:   "fetch [dir]",
		Short: "Download and verify the Nabu web app for this course",
		Long: "Download nabu-web-<version>.tar.gz from the Nabu release, check its sha256\n" +
			"against the release's checksums.txt, and unpack it into [dir] (default " + DefaultDir + "/).\n" +
			"Serve it with nabu.Options{Public: os.DirFS(\"" + DefaultDir + "\")}.\n\n" +
			"The release is --version, else the version of " + ModulePath + " that ./go.mod\n" +
			"requires, else the version this binary was built from. A folder that already\n" +
			"holds that release is left alone, so the command works offline once fetched.\n\n" +
			"Mode: both. A release binary already embeds the web app and does not need it;\n" +
			"a library-form course (nabu init --library) does.",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				o.Dir = args[0]
			}
			res, err := Fetch(cmd.Context(), o)
			if err != nil {
				return err
			}
			if !res.Fetched {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s already holds the web app %s\n", res.Dir, res.Version)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: web app %s (%d files, from %s), sha256 verified\n",
				res.Dir, res.Version, res.Files, res.Source)
			return err
		},
	}
	fetch.Flags().StringVar(&o.Version, "version", "", "release to fetch, e.g. v0.1.2 (default: from go.mod, else this binary)")
	fetch.Flags().StringVar(&o.BaseURL, "base-url", "", "download from this mirror: <base-url>/<version>/<asset> (default "+DefaultBaseURL+")")
	fetch.Flags().StringVar(&o.GoMod, "go-mod", "", "go.mod to read the version from (default ./go.mod)")
	fetch.Flags().BoolVar(&o.Force, "force", false, "download again even when [dir] already holds the release")
	web.AddCommand(fetch)
	return web
}
