package changelog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/stripe/hark/changefile"
)

// the title written when the author gave nothing to use as one. Deliberately fails [Validate] so all of our changes are well-named.
const placeholderTitle = changefile.FixmeSlug + ": describe this change"

// Options for seeding new changefiles
type NewOptions struct {
	// the short phrase in the filename, used for uniqueness and searchability. The default fails in [Validate] so it's replaced with something good.
	Slug string
	// used by the auto PRs because they write the diff to disk before use
	BodyPath string
	// who to attribute the change to. Defaults to the current user. Automation should pass it explicitly, since $USER isn't useful in CI
	User string
	PRs  PullRequestFinder
	// Function to determine the current user. Defaults to returning $USER, but can be overridden for tests.
	LookupUser func() string
}

func (n NewOptions) withDefaults() NewOptions {
	if n.PRs == nil {
		n.PRs = ghFinder{}
	}
	if n.LookupUser == nil {
		n.LookupUser = func() string { return os.Getenv("USER") }
	}
	return n
}

// creates a changefile from draft, returning the path it was written to.
// It tries to pre-populate as much as it can. Anything it provided a validation-failing default for it will warn the user about.
//
// The default changefile purposefully doesn't pass validation and is meant as a starting point.
func New(ctx context.Context, opts Options, draft changefile.Changefile, newOpts NewOptions) (string, error) {
	opts = opts.withDefaults()
	newOpts = newOpts.withDefaults()

	// Only call out to `gh` if it could fill in information we don't already have
	if draft.Title == "" || draft.PRUrl == "" {
		pr, lookupErr := newOpts.PRs.CurrentPR(ctx)
		if lookupErr != nil {
			// Not fatal: the changefile can be written before the PR exists.
			if _, err := fmt.Fprintf(opts.Out, "could not find a pull request (%v); continuing without one\n", lookupErr); err != nil {
				return "", err
			}
		}
		if pr != nil {
			seedFromPR(&draft, pr)
		}
	}

	var defaulted struct{ slug, title, user bool }

	// Set the user from explicit arg or current user-getter function (falling back to an acceptable default)
	user := newOpts.User
	if user == "" {
		user = newOpts.LookupUser()
	}
	if user == "" {
		user, defaulted.user = changefile.UnknownUser, true
	}

	if newOpts.BodyPath != "" && draft.Body == "" {
		data, err := afero.ReadFile(opts.Fs, newOpts.BodyPath)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", newOpts.BodyPath, err)
		}
		draft.Body = strings.TrimSpace(string(data))
	}

	if draft.Title == "" {
		draft.Title, defaulted.title = placeholderTitle, true
	}
	slug := newOpts.Slug
	if slug == "" {
		slug, defaulted.slug = changefile.FixmeSlug, true
	}

	dir := opts.changesDir()
	if err := opts.Fs.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}

	path, err := newPath(opts, dir, opts.today(), user, slug)
	if err != nil {
		return "", err
	}

	if err := draft.WriteFile(opts.Fs, path); err != nil {
		return "", err
	}

	if _, err := fmt.Fprintf(opts.Out, "wrote %s\n", path); err != nil {
		return "", err
	}

	// Every default that stood in for something only the author can supply, said while
	// they are still looking at the terminal rather than left for CI to raise.
	var todo []string
	if defaulted.slug {
		todo = append(todo, fmt.Sprintf("rename it to replace %s with a few words describing the change", changefile.FixmeSlug))
	}
	if defaulted.title {
		todo = append(todo, "replace the title, which is the changelog bullet readers will see")
	}
	if defaulted.user {
		todo = append(todo, fmt.Sprintf("put your username in place of %q, since $USER was empty", changefile.UnknownUser))
	}

	for _, note := range todo {
		if _, err := fmt.Fprintf(opts.Out, "  - %s\n", note); err != nil {
			return "", err
		}
	}
	return path, nil
}

// supplement `draft` with data from a `pr`
func seedFromPR(draft *changefile.Changefile, pr *PullRequest) {
	if draft.Title == "" {
		draft.Title = pr.Title
	}
	if draft.PRUrl == "" {
		draft.PRUrl = pr.URL
	}
	if len(draft.JiraTicketsClosed) == 0 {
		draft.JiraTicketsClosed = pr.JiraTags
	}
}

// builds the path for a new changefile given its component parts. errors if there's a file there already
func newPath(opts Options, dir, date, user, slug string) (string, error) {
	name := changefile.Name(date, user, slug)
	path := filepath.Join(dir, name)
	if filepath.Base(path) != name {
		return "", fmt.Errorf("changefile name must not contain path separators")
	}

	exists, err := afero.Exists(opts.Fs, path)
	if err != nil {
		return "", fmt.Errorf("checking %s: %w", path, err)
	}
	if exists {
		return "", fmt.Errorf("%s already exists; pass a different slug, or edit that file", path)
	}

	return path, nil
}
