package changefile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantYAML string
		wantBody string
		wantErr  error
	}{
		{
			name:     "basic frontmatter with body",
			input:    "---\ntitle: hello\n---\nsome body content\n",
			wantYAML: "title: hello\n",
			wantBody: "some body content\n",
		},
		{
			name:     "frontmatter only, no body",
			input:    "---\ntitle: hello\n---\n",
			wantYAML: "title: hello\n",
			wantBody: "",
		},
		{
			name:     "multiline frontmatter",
			input:    "---\ntitle: hello\nsemver_level: major\n---\nbody\n",
			wantYAML: "title: hello\nsemver_level: major\n",
			wantBody: "body\n",
		},
		{
			name:     "body with multiple lines",
			input:    "---\ntitle: test\n---\nline 1\nline 2\nline 3\n",
			wantYAML: "title: test\n",
			wantBody: "line 1\nline 2\nline 3\n",
		},
		{
			name:    "missing opening delimiter",
			input:   "title: hello\n---\nbody\n",
			wantErr: ErrNoFrontmatter,
		},
		{
			name:    "missing closing delimiter",
			input:   "---\ntitle: hello\nbody without closing\n",
			wantErr: ErrUnclosedFrontmatter,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: ErrNoFrontmatter,
		},
		{
			name:     "frontmatter with closing at EOF (no trailing newline)",
			input:    "---\ntitle: hi\n---",
			wantYAML: "title: hi\n",
			wantBody: "",
		},
		{
			name:     "leading blank lines are ignored",
			input:    "\n\n---\ntitle: hello\n---\nbody\n",
			wantYAML: "title: hello\n",
			wantBody: "body\n",
		},
		{
			// a leading newline with nothing else is still missing its delimiter
			name:    "only a leading newline",
			input:   "\n",
			wantErr: ErrNoFrontmatter,
		},
		{
			name:     "a value ending in the delimiter does not close the frontmatter",
			input:    "---\ntitle: \"the --- separator\"\nsection: ---\n---\nbody\n",
			wantYAML: "title: \"the --- separator\"\nsection: ---\n",
			wantBody: "body\n",
		},
		{
			name:     "an indented delimiter does not close the frontmatter",
			input:    "---\ntitle: >\n  one\n  ---\n  two\n---\nbody\n",
			wantYAML: "title: >\n  one\n  ---\n  two\n",
			wantBody: "body\n",
		},
		{
			name:     "a delimiter in the body is left alone",
			input:    "---\ntitle: hi\n---\nbody\n\n---\n\nmore body\n",
			wantYAML: "title: hi\n",
			wantBody: "body\n\n---\n\nmore body\n",
		},
		{
			name:     "empty frontmatter",
			input:    "---\n---\nbody\n",
			wantYAML: "",
			wantBody: "body\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml, body, err := SplitFrontmatter([]byte(tt.input))

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantYAML, string(yaml))
			assert.Equal(t, tt.wantBody, body)
		})
	}
}
