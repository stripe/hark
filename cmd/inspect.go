package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/stripe/hark/internal/changelog"
)

func newInspectCmd(g *globalFlags) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "inspect --format json CHANGEFILE...",
		Short: "Report the effective semver level of changefiles",
		Long: "Report the effective semver level of explicitly named changefiles.\n\n" +
			"The JSON output is an array in argument order. A changefile without semver_level is reported as patch.",
		Args: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: runE(func(cmd *cobra.Command, paths []string) error {
			if format != "json" {
				return fmt.Errorf("--format must be json")
			}
			return changelog.Inspect(cmd.Context(), g.options(cmd), paths)
		}),
	}

	cmd.Flags().StringVar(&format, "format", "", "output format (required: json)")
	return cmd
}
