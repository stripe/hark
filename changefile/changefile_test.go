package changefile

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Changefile
		wantErr bool
	}{
		{
			name: "all fields",
			input: `---
title: "Added a new method to stripeClient"
pr_url: "https://github.com/stripe/stripe-go/pulls/123"
is_breaking: true
is_stripe_api_change: true
jira_tickets_closed:
  - DEVSDK-123
  - DEVSDK-456
github_issues_resolved:
  - "https://github.com/stripe/stripe-go/issues/7"
  - "https://github.com/stripe/stripe-go/issues/9"
section: "cool new things"
released_in_version: "1.2.3"
---

- some extra details
- about this change
`,
			want: &Changefile{
				Title:             "Added a new method to stripeClient",
				PRUrl:             "https://github.com/stripe/stripe-go/pulls/123",
				IsBreaking:        true,
				IsStripeAPIChange: true,
				JiraTicketsClosed: []string{"DEVSDK-123", "DEVSDK-456"},
				GithubIssuesResolved: []string{
					"https://github.com/stripe/stripe-go/issues/7",
					"https://github.com/stripe/stripe-go/issues/9",
				},
				Section:           "cool new things",
				ReleasedInVersion: "1.2.3",
				Body:              "- some extra details\n- about this change",
			},
		},
		{
			name: "required fields only",
			input: `---
title: "Simple change"
---
`,
			want: &Changefile{
				Title: "Simple change",
			},
		},
		{
			name: "with body prose",
			input: `---
title: "A change"
---

This is some prose explaining the change in detail.
`,
			want: &Changefile{
				Title: "A change",
				Body:  "This is some prose explaining the change in detail.",
			},
		},
		{
			name:    "invalid yaml",
			input:   "---\n: : : invalid\n---\n",
			wantErr: true,
		},
		{
			name:    "no frontmatter",
			input:   "just some markdown\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Title, got.Title)
			assert.Equal(t, tt.want.PRUrl, got.PRUrl)
			assert.Equal(t, tt.want.IsBreaking, got.IsBreaking)
			assert.Equal(t, tt.want.JiraTicketsClosed, got.JiraTicketsClosed)
			assert.Equal(t, tt.want.GithubIssuesResolved, got.GithubIssuesResolved)
			assert.Equal(t, tt.want.Section, got.Section)
			assert.Equal(t, tt.want.ReleasedInVersion, got.ReleasedInVersion)
			assert.Equal(t, tt.want.Body, got.Body)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		changefile Changefile
		wantErrs   int
	}{
		{
			name:       "valid with all fields",
			changefile: Changefile{Title: "A title", PRUrl: "https://example.com/pr/1"},
			wantErrs:   0,
		},
		{
			name:       "valid with title only",
			changefile: Changefile{Title: "A title"},
			wantErrs:   0,
		},
		{
			name:       "missing title",
			changefile: Changefile{},
			wantErrs:   1,
		},
		{
			name:       "empty title (whitespace)",
			changefile: Changefile{Title: "   "},
			wantErrs:   1,
		},
		{
			name:       "invalid pr_url",
			changefile: Changefile{Title: "A title", PRUrl: "not-a-url"},
			wantErrs:   1,
		},
		{
			name:       "missing title and invalid pr_url",
			changefile: Changefile{PRUrl: "bad"},
			wantErrs:   2,
		},
		{
			name: "valid github_issues_resolved",
			changefile: Changefile{Title: "A title", GithubIssuesResolved: []string{
				"https://github.com/stripe/stripe-go/issues/7",
				"https://github.com/stripe/stripe-go/issues/9",
			}},
			wantErrs: 0,
		},
		{
			name:       "invalid github_issues_resolved",
			changefile: Changefile{Title: "A title", GithubIssuesResolved: []string{"7"}},
			wantErrs:   1,
		},
		{
			// Every bad entry is reported, not just the first.
			name: "several invalid github_issues_resolved",
			changefile: Changefile{Title: "A title", GithubIssuesResolved: []string{
				"7", "https://github.com/stripe/stripe-go/issues/9", "#11",
			}},
			wantErrs: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.changefile.Validate()
			assert.Len(t, errs, tt.wantErrs)
		})
	}
}

func TestSerializeRoundtrip(t *testing.T) {
	original := &Changefile{
		Title:                "Test change",
		PRUrl:                "https://github.com/stripe/stripe-go/pulls/1",
		IsBreaking:           true,
		JiraTicketsClosed:    []string{"DEVSDK-100"},
		GithubIssuesResolved: []string{"https://github.com/stripe/stripe-go/issues/7"},
		Section:              "features",
		Body:                 "- extra detail here",
	}

	serialized, err := original.Serialize()
	require.NoError(t, err)

	parsed, err := Parse(serialized)
	require.NoError(t, err)

	assert.Equal(t, original.Title, parsed.Title)
	assert.Equal(t, original.PRUrl, parsed.PRUrl)
	assert.Equal(t, original.IsBreaking, parsed.IsBreaking)
	assert.Equal(t, original.JiraTicketsClosed, parsed.JiraTicketsClosed)
	assert.Equal(t, original.GithubIssuesResolved, parsed.GithubIssuesResolved)
	assert.Equal(t, original.Section, parsed.Section)
	assert.Equal(t, original.Body, parsed.Body)
}

func TestReadFile(t *testing.T) {
	fs := afero.NewMemMapFs()

	content := `---
title: "FS test"
section: "testing"
---

Body from file.
`
	require.NoError(t, afero.WriteFile(fs, "/changes/test.change.md", []byte(content), 0644))

	cf, err := ReadFile(fs, "/changes/test.change.md")
	require.NoError(t, err)

	assert.Equal(t, "FS test", cf.Title)
	assert.Equal(t, "testing", cf.Section)
	assert.Equal(t, "Body from file.", cf.Body)
	assert.Equal(t, "/changes/test.change.md", cf.SourcePath)
}

func TestReadFile_NotFound(t *testing.T) {
	fs := afero.NewMemMapFs()

	_, err := ReadFile(fs, "/does/not/exist.change.md")
	require.Error(t, err)
}

func TestWriteFileRoundtrip(t *testing.T) {
	fs := afero.NewMemMapFs()

	original := &Changefile{
		Title:                "Written change",
		Section:              "features",
		JiraTicketsClosed:    []string{"DEVSDK-1"},
		GithubIssuesResolved: []string{"https://github.com/stripe/stripe-go/issues/11"},
		Body:                 "- a detail",
	}

	path := "/changes/written" + Extension
	require.NoError(t, original.WriteFile(fs, path))

	read, err := ReadFile(fs, path)
	require.NoError(t, err)

	assert.Equal(t, original.Title, read.Title)
	assert.Equal(t, original.Section, read.Section)
	assert.Equal(t, original.JiraTicketsClosed, read.JiraTicketsClosed)
	assert.Equal(t, original.GithubIssuesResolved, read.GithubIssuesResolved)
	assert.Equal(t, original.Body, read.Body)
	assert.Equal(t, path, read.SourcePath)
}
