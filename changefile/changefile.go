package changefile

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/spf13/afero"
)

// every valid changefile filename ends with this extension.
const Extension = ".change.md"

// JiraTagPattern is the shape of a Jira ticket reference, like "DEVSDK-123" or "RUN_DEVSDK-456".
//
// A pattern rather than a compiled regexp, because the two uses want different anchors:
// validating a stored value wants the whole string, while pulling references out of a
// branch name wants word boundaries. Sharing the shape is what keeps `hark new` from
// writing a tag `hark validate` would then reject.
const JiraTagPattern = `[A-Z][A-Z_0-9]+-[0-9]+`

var jiraTagRegex = regexp.MustCompile(`^` + JiraTagPattern + `$`)

// The size of the version bump a change calls for when it ships.
const (
	SemverLevelMajor = "major"
	SemverLevelMinor = "minor"
	SemverLevelPatch = "patch"
)

var SemverLevels = []string{SemverLevelMajor, SemverLevelMinor, SemverLevelPatch}

// Changefile is the in-memory representation of a single parsed changefile: its frontmatter fields plus the
// markdown body that follows them.
type Changefile struct {
	// the header for the changelog bullet. Required. Likely matches the original PR.
	Title string `yaml:"title"`
	// the original pull request a change was part of.
	PRUrl string `yaml:"pr_url,omitempty"`
	// optional compatibility level for the change. missing is considered `patch`
	SemverLevel string `yaml:"semver_level,omitempty"`
	// set on changes caused by and updated spec. Won't be shown on docs.stripe.com and will be listed last in generated changelogs.
	IsStripeAPIChange bool `yaml:"is_stripe_api_change,omitempty"`
	// a list of jira tags (like `DEVSDK-123`) that can be closed when this change is released.
	JiraTicketsClosed []string `yaml:"jira_tickets_closed,omitempty"`
	// a list of full urls pointing to GH issues in this repo.
	GithubIssuesResolved []string `yaml:"github_issues_resolved,omitempty"`
	// A `Section` groups this change under a section header (along with all other changes that share a section).
	Section string `yaml:"section,omitempty"`
	// the version this change shipped in. Empty until the change is released.
	ReleasedInVersion string `yaml:"released_in_version,omitempty"`

	// the markdown content after the frontmatter.
	Body string `yaml:"-"`

	// the full path that this struct originated from
	SourcePath string `yaml:"-"`
}

// Parse reads raw file bytes and returns a Changefile.
// It does not validate field constraints (e.g. the validity of a url);
// call [Changefile.Validate] for that.
func Parse(content []byte) (*Changefile, error) {
	yamlBytes, body, err := SplitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	var cf Changefile
	if err := yaml.Unmarshal(yamlBytes, &cf); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	cf.Body = strings.TrimSpace(body)
	return &cf, nil
}

// the version bump this change calls for. handles computing the default for missing fields
func (c *Changefile) Level() string {
	if c.SemverLevel == "" {
		return SemverLevelPatch
	}
	return c.SemverLevel
}

// Validate checks required fields and format constraints.
// It reports every problem it finds rather than stopping at the first.
// An empty slice means the changefile is valid.
func (c *Changefile) Validate() []error {
	var errs []error

	switch title := strings.TrimSpace(c.Title); {
	case title == "":
		errs = append(errs, errors.New("title is required"))
	case strings.HasPrefix(strings.ToUpper(title), FixmeSlug):
		errs = append(errs, fmt.Errorf(
			"title still starts with %s; replace it with the changelog bullet you want readers to see", FixmeSlug))
	}

	if c.PRUrl != "" {
		if _, err := url.ParseRequestURI(c.PRUrl); err != nil {
			errs = append(errs, fmt.Errorf("pr_url is not a valid URL: %w", err))
		}
	}

	// An empty level is a patch, but an invalid level is an error
	if c.SemverLevel != "" && !slices.Contains(SemverLevels, c.SemverLevel) {
		errs = append(errs, fmt.Errorf("semver_level %q is not one of: %s",
			c.SemverLevel, strings.Join(SemverLevels, ", ")))
	}

	for i, tag := range c.JiraTicketsClosed {
		if !jiraTagRegex.MatchString(tag) {
			errs = append(errs, fmt.Errorf(
				"jira_tickets_closed[%d] %q is not a valid ticket reference, like DEVSDK-123", i, tag))
		}
	}

	for i, issue := range c.GithubIssuesResolved {
		if _, err := url.ParseRequestURI(issue); err != nil {
			errs = append(errs, fmt.Errorf("github_issues_resolved[%d] is not a valid URL: %w", i, err))
		}
	}

	return errs
}

func (c *Changefile) Serialize() ([]byte, error) {
	yamlBytes, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("serializing YAML: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	if c.Body != "" {
		buf.WriteString("\n")
		buf.WriteString(c.Body)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

func (c *Changefile) WriteFile(fs afero.Fs, path string) error {
	data, err := c.Serialize()
	if err != nil {
		return err
	}

	if err := afero.WriteFile(fs, path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// Reads & parses a changefile from `path`. Doesn't enforce field constraints (use [Changefile.Validate] for that)
func ReadFile(fs afero.Fs, path string) (*Changefile, error) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	cf, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("in %s: %w", path, err)
	}

	cf.SourcePath = path
	return cf, nil
}
