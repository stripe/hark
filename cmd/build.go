package cmd

import (
	"github.com/spf13/cobra"

	"github.com/stripe/hark/internal/changelog"
)

func newBuildCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Compile all changefiles into a " + changelog.Filename + ".",
		Long: "Compile all changefiles into " + changelog.Filename + ".\n\n" +
			"Running this is idempotent and only modifies the changelog file itself.",
		Args: cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			return changelog.Build(cmd.Context(), g.options(cmd))
		}),
	}
}
