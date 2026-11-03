package admin

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mrmxf/nabu/embedfs"
)

// newShowConfigCommand implements `admin showconfig`: print the effective
// configuration — the embedded nabu-config.yaml deep-merged with any --config
// override — to stdout as YAML. Useful for confirming what an override file
// actually changes. The config carries no secrets (only password_env names), so
// it is safe to print.
func newShowConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "showconfig",
		Short: "Print the effective config (embedded + --config override) to stdout",
		Long: "Print the effective Nabu configuration as YAML: the embedded\n" +
			"nabu-config.yaml deep-merged with any file passed via the global\n" +
			"--config/-c flag. Handy for checking what an override actually changes:\n" +
			"  nabu --config ./local.yaml admin showconfig",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := embedfs.RawMerged()
			if err != nil {
				return err
			}
			out, err := yaml.Marshal(raw)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			fmt.Print(string(out))
			return nil
		},
	}
}
