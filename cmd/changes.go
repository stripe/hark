package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/stripe/hark/internal/changelog"
)

func newChangesCmd(global *globalFlags) *cobra.Command {
	var version string
	var unreleased bool
	command := &cobra.Command{
		Use:          "changes (--unreleased | --release VERSION)",
		Short:        "List changefile paths for a release or the unreleased queue as JSON",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: runE(func(command *cobra.Command, _ []string) error {
			if unreleased == (version != "") {
				return errors.New("choose exactly one of --unreleased or --release VERSION")
			}
			return changelog.Changes(command.Context(), global.options(command), version)
		}),
	}
	command.Flags().BoolVar(&unreleased, "unreleased", false, "list unstamped changefiles")
	command.Flags().StringVar(&version, "release", "", "list changefiles stamped with this version")
	command.MarkFlagsMutuallyExclusive("unreleased", "release")
	return command
}
