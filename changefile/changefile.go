package changefile

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/spf13/afero"
)

// every valid changefile filename ends with this extension.
const Extension = ".change.md"

// Changefile is the in-memory representation of a single parsed changefile: its frontmatter fields plus the
// markdown body that follows them.
type Changefile struct {
	// the header for the changelog bullet. Required. Likely matches the original PR.
	Title string `yaml:"title"`
	// the original pull request a change was part of.
	PRUrl string `yaml:"pr_url,omitempty"`
	// marks the change requiring a semver-major bump to release.
	IsBreaking bool `yaml:"is_breaking,omitempty"`
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

	// the name of the file this struct originated from
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

// Validate checks required fields and format constraints.
// It reports every problem it finds rather than stopping at the first.
// An empty slice means the changefile is valid.
func (c *Changefile) Validate() []error {
	var errs []error

	if strings.TrimSpace(c.Title) == "" {
		errs = append(errs, errors.New("title is required"))
	}

	if c.PRUrl != "" {
		if _, err := url.ParseRequestURI(c.PRUrl); err != nil {
			errs = append(errs, fmt.Errorf("pr_url is not a valid URL: %w", err))
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
