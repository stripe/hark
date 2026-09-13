package cmd

import (
	"github.com/spf13/cobra"

	"github.com/stripe/hark/internal/changelog"
)

func newValidateCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate every changefile for correctness",
		Long: "Validate every changefile for correctness.\n\n" +
			"Reports all of the problems it finds, grouped by file.",
		Args: cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			return changelog.Validate(cmd.Context(), g.options(cmd))
		}),
	}
}
