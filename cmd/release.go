package cmd

import (
	"github.com/spf13/cobra"

	"github.com/stripe/hark/internal/changelog"
	"github.com/stripe/hark/releases"
)

func newReleaseCmd(g *globalFlags) *cobra.Command {
	var release releases.Release

	long := "Create a new release from pending changefiles.\n\nStamps every unreleased changefile with VERSION, records the release in the releases file, and rebuilds the changelog.\n\nPerforms no git operations, just changefile modifications. Is idempotent when run with the same arguments.\n\nThe pinned API version and minimum runtime version carry forward from the previous release unless supplied as flags. If an entry for VERSION already exists, it is used as-is. It's an error to pass a CLI flag that contradicts an existing value.\n\nTo put prose above a release's changes, write it to .hark/" + changelog.IntrosDir + "/" + changelog.IntroName("VERSION")

	cmd := &cobra.Command{
		Use:   "release <VERSION>",
		Short: "Create a new release from pending changefiles",
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			release.Version = args[0]
			return changelog.Release(cmd.Context(), g.options(cmd), release)
		}),
	}

	f := cmd.Flags()
	f.StringVar(&release.PinnedAPIVersion, "pinned-api-version", "", "Stripe API version this release pins to")
	f.StringVar(&release.MinimumRuntimeVersion, "minimum-runtime-version", "", "lowest supported version of the host language (e.g. 3.10)")

	return cmd
}
