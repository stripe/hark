package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/internal/changelog"
)

func newNewCmd(g *globalFlags) *cobra.Command {
	var (
		draft    changefile.Changefile
		bodyFile string
		user     string
	)

	cmd := &cobra.Command{
		Use:   "new [SLUG]",
		Short: "Create a new changefile",
		Long: "Create a new changefile.\n\n" +
			"Any fields not present have sensible defaults (that may not pass verification).\n\nSLUG is the short phrase that goes in the filename. Leave it out and the file is named FIXME, which `hark validate` rejects until you rename it.",
		Args: cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if draft.Body != "" && bodyFile != "" {
				return errors.New("pass either --body or --body-file, not both")
			}

			newOpts := changelog.NewOptions{BodyPath: bodyFile, User: user}
			if len(args) > 0 {
				newOpts.Slug = args[0]
			}

			_, err := changelog.New(cmd.Context(), g.options(cmd), draft, newOpts)
			return err
		}),
	}

	f := cmd.Flags()
	f.SortFlags = false

	f.StringVar(&draft.Title, "title", "", "the changelog bullet's text (defaults to the pull request's title)")
	f.StringVar(&draft.PRUrl, "pr-url", "", "URL of the pull request (defaults to the PR of the current branch, if available)")
	f.StringVar(&draft.Section, "section", "", "heading to group this change under")
	f.BoolVar(&draft.IsBreaking, "breaking", false, "mark the change as breaking")
	f.BoolVar(&draft.IsStripeAPIChange, "stripe-api-change", false, "mark the change as the result of an API spec bump")
	f.StringArrayVar(&draft.JiraTicketsClosed, "jira-tag", nil, "Jira ticket reference (like DEVSDK-123); repeat for more than one")
	f.StringArrayVar(&draft.GithubIssuesResolved, "github-issue-resolved", nil, "URL of a GitHub issue this change resolves once released; repeat for more than one")
	f.StringVar(&draft.Body, "body", "", "markdown to nest under the changelog bullet. Mutually exclusive with --body-file")
	f.StringVar(&bodyFile, "body-file", "", "file to read the body from.  Mutually exclusive with --body")
	f.StringVar(&user, "user", "", "who to attribute the change to (defaults to $USER)")

	return cmd
}
