package changelog

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inspectOutput(t *testing.T, fs afero.Fs, paths ...string) ([]Inspection, string, error) {
	t.Helper()

	var out bytes.Buffer
	err := Inspect(context.Background(), Options{Fs: fs, Out: &out}, paths)
	if err != nil {
		return nil, out.String(), err
	}

	var got []Inspection
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	return got, out.String(), nil
}

func TestInspect(t *testing.T) {
	fs := afero.NewMemMapFs()
	for path, content := range map[string]string{
		"major.change.md":   "---\ntitle: Major\nsemver_level: major\n---\n",
		"minor.change.md":   "---\ntitle: Minor\nsemver_level: minor\n---\n",
		"patch.change.md":   "---\ntitle: Patch\nsemver_level: patch\n---\n",
		"default.change.md": "---\ntitle: Default\n---\n",
		"quoted.change.md":  "---\ntitle: Quoted\nsemver_level: \"minor\"\n---\n",
		"crlf.change.md":    "---\r\ntitle: CRLF\r\nsemver_level: 'major'\r\n---\r\n",
	} {
		require.NoError(t, afero.WriteFile(fs, path, []byte(content), 0o644))
	}

	t.Run("one file", func(t *testing.T) {
		got, _, err := inspectOutput(t, fs, "major.change.md")
		require.NoError(t, err)
		assert.Equal(t, []Inspection{{Path: "major.change.md", SemverLevel: "major"}}, got)
	})

	t.Run("multiple files preserve order and effective levels", func(t *testing.T) {
		got, _, err := inspectOutput(t, fs,
			"default.change.md", "minor.change.md", "patch.change.md", "major.change.md")
		require.NoError(t, err)
		assert.Equal(t, []Inspection{
			{Path: "default.change.md", SemverLevel: "patch"},
			{Path: "minor.change.md", SemverLevel: "minor"},
			{Path: "patch.change.md", SemverLevel: "patch"},
			{Path: "major.change.md", SemverLevel: "major"},
		}, got)
	})

	t.Run("quoted YAML values", func(t *testing.T) {
		got, _, err := inspectOutput(t, fs, "quoted.change.md")
		require.NoError(t, err)
		assert.Equal(t, []Inspection{{Path: "quoted.change.md", SemverLevel: "minor"}}, got)
	})

	t.Run("CRLF front matter", func(t *testing.T) {
		got, _, err := inspectOutput(t, fs, "crlf.change.md")
		require.NoError(t, err)
		assert.Equal(t, []Inspection{{Path: "crlf.change.md", SemverLevel: "major"}}, got)
	})
}

func TestInspectFailuresDoNotWriteJSON(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "valid.change.md", []byte("---\ntitle: Valid\n---\n"), 0o644))
	require.NoError(t, afero.WriteFile(fs, "malformed.change.md", []byte("---\ntitle: [bad\n---\n"), 0o644))
	require.NoError(t, afero.WriteFile(fs, "no-frontmatter.change.md", []byte("# Not a changefile\n"), 0o644))

	for _, path := range []string{"missing.change.md", "malformed.change.md", "no-frontmatter.change.md"} {
		t.Run(path, func(t *testing.T) {
			_, stdout, err := inspectOutput(t, fs, "valid.change.md", path)
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, err.Error(), path)
		})
	}
}
