package changelog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"github.com/stripe/hark/changefile"
)

// data about a GH pull request used in the creation of a changefile
type PullRequest struct {
	URL      string
	Title    string
	JiraTags []string
}

// interface to manage our boundary with the `gh` CLI
type PullRequestFinder interface {
	CurrentPR(ctx context.Context) (*PullRequest, error)
}

// A ticket reference in a larger block of text
var jiraTagRegex = regexp.MustCompile(`\b` + changefile.JiraTagPattern + `\b`)

type ghFinder struct{}

// mirrors the JSON that `gh` emits for the fields we ask for.
type ghPullRequest struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
}

// CurrentPR shells out to `gh pr view` for the current branch's pull request.
//
// A non-zero exit is reported as "no pull request" rather than a true error, because
// that's the `gh` behavior both when the branch has no PR or the user isn't logged in.
func (ghFinder) CurrentPR(ctx context.Context) (*PullRequest, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view",
		"--json", "url,title,headRefName")

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return nil, fmt.Errorf("gh: %s", firstLine(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("running gh: %w", err)
	}

	var pr ghPullRequest
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, fmt.Errorf("parsing gh output: %w", err)
	}

	return &PullRequest{
		URL:      pr.URL,
		Title:    pr.Title,
		JiraTags: jiraTags(pr.HeadRefName),
	}, nil
}

// jiraTags collects the distinct ticket references, in the order they first appear.
func jiraTags(branch string) []string {
	var tags []string
	for _, tag := range jiraTagRegex.FindAllString(branch, -1) {
		if !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	return tags
}

// firstLine is the first non-blank line of s, for quoting a subprocess's
// complaint without repeating its whole usage text.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "no pull request found for the current branch"
}
