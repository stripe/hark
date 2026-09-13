// Package cmd assembles the hark CLI. Commands here are thin shells: they parse
// arguments and delegate to internal/changelog, which holds the implementations.
//
// The command tree is built by a constructor rather than stored in package-level
// variables, so each test gets an independent CLI with its own filesystem and output
// stream, and flag values cannot leak between them.
package cmd

import (
	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/internal/changelog"
)

type globalFlags struct {
	// for controlling parallelism
	workers int

	// fs is the filesystem CLI commands operate on. Nil means the real one.
	fs afero.Fs
}

// options turns the parsed flags into the Options that internal/changelog takes,
// bound to this command's output stream so output can be redirected in tests.
func (g *globalFlags) options(cmd *cobra.Command) changelog.Options {
	return changelog.Options{
		Fs:          g.fs,
		Out:         cmd.OutOrStdout(),
		ReadOptions: changefile.ReadOptions{Workers: g.workers},
	}
}

// newRootCmd builds the hark command tree against fs. A nil fs means the real
// filesystem; tests pass an in-memory one.
func newRootCmd(version string, fs afero.Fs) *cobra.Command {
	g := &globalFlags{fs: fs}

	root := &cobra.Command{
		Use:     "hark",
		Short:   "Validate and compile changefiles into a changelog",
		Version: version,
	}

	root.PersistentFlags().IntVar(&g.workers, "workers", 0,
		"maximum concurrent changefile readers (default is one per CPU)")

	root.AddCommand(
		newNewCmd(g),
		newReleaseCmd(g),
		newBuildCmd(g),
		newValidateCmd(g),
	)

	return root
}

func Execute(version string) int {
	if err := newRootCmd(version, nil).Execute(); err != nil {
		return 1
	}
	return 0
}

func runE(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		return fn(cmd, args)
	}
}
