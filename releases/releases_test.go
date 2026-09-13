package releases

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	content := `{
  "releases": [
    {
      "version": "2.0.0",
      "released_on": "2024-01-15",
      "pinned_api_version": "2024-01-01",
      "minimum_runtime_version": "3.10"
    },
    {
      "version": "1.0.0",
      "released_on": "2023-06-01"
    }
  ]
}`
	require.NoError(t, afero.WriteFile(fs, "/releases.json", []byte(content), 0644))

	vf, err := ReadFile(fs, "/releases.json")
	require.NoError(t, err)

	assert.Len(t, vf.Releases, 2)
	assert.Equal(t, "2.0.0", vf.Releases[0].Version)
	assert.Equal(t, "2024-01-15", vf.Releases[0].ReleasedOn)
	assert.Equal(t, "2024-01-01", vf.Releases[0].PinnedAPIVersion)
	assert.Equal(t, "3.10", vf.Releases[0].MinimumRuntimeVersion)
	assert.Equal(t, "1.0.0", vf.Releases[1].Version)
	assert.Equal(t, "", vf.Releases[1].PinnedAPIVersion)
	assert.Equal(t, "", vf.Releases[1].MinimumRuntimeVersion)
}

func TestReadFile_Empty(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/releases.json", []byte(`{"releases": []}`), 0644))

	vf, err := ReadFile(fs, "/releases.json")
	require.NoError(t, err)
	assert.Empty(t, vf.Releases)
}

func TestReadFile_NotFound(t *testing.T) {
	fs := afero.NewMemMapFs()

	_, err := ReadFile(fs, "/nope.json")
	require.Error(t, err)
}

func TestReadFile_InvalidJSON(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/bad.json", []byte(`{not json`), 0644))

	_, err := ReadFile(fs, "/bad.json")
	require.Error(t, err)
}

func TestWriteFileRoundtrip(t *testing.T) {
	fs := afero.NewMemMapFs()

	original := &File{
		Releases: []Release{
			{
				Version:               "1.0.0",
				ReleasedOn:            "2024-01-01",
				PinnedAPIVersion:      "2024-01-01",
				MinimumRuntimeVersion: "3.10",
			},
		},
	}

	require.NoError(t, WriteFile(fs, "/out.json", original))

	read, err := ReadFile(fs, "/out.json")
	require.NoError(t, err)

	// verify the original path, then drop it for later comparison; the in-memory file didn't have a path
	assert.Equal(t, "/out.json", read.SourcePath)
	read.SourcePath = ""

	assert.Equal(t, original, read)
}

func TestWriteFile_OmitsEmptyOptionalFields(t *testing.T) {
	fs := afero.NewMemMapFs()

	vf := &File{Releases: []Release{{Version: "1.0.0", ReleasedOn: "2024-01-01"}}}
	require.NoError(t, WriteFile(fs, "/out.json", vf))

	data, err := afero.ReadFile(fs, "/out.json")
	require.NoError(t, err)

	assert.NotContains(t, string(data), "minimum_version")
	assert.NotContains(t, string(data), "api_version")
	// An introduction is a file now, never a field here.
	assert.NotContains(t, string(data), "intro")
	assert.NotContains(t, string(data), "prelude")
}

func TestFind(t *testing.T) {
	vf := &File{
		Releases: []Release{
			{Version: "2.0.0", ReleasedOn: "2024-06-01"},
			{Version: "1.0.0", ReleasedOn: "2024-01-01"},
		},
	}

	found := vf.Find("1.0.0")
	require.NotNil(t, found)
	assert.Equal(t, "2024-01-01", found.ReleasedOn)

	assert.Nil(t, vf.Find("3.0.0"))
}

func TestFind_ReturnsMutableEntry(t *testing.T) {
	vf := &File{Releases: []Release{{Version: "1.0.0", ReleasedOn: "2024-01-01"}}}

	vf.Find("1.0.0").MinimumRuntimeVersion = "3.10"

	assert.Equal(t, "3.10", vf.Releases[0].MinimumRuntimeVersion)
}
