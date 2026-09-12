package changefile

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validChangefile(title string) []byte {
	return fmt.Appendf(nil, "---\ntitle: %q\n---\n", title)
}

func TestReadAll_FindsNestedFiles(t *testing.T) {
	fs := afero.NewMemMapFs()

	require.NoError(t, afero.WriteFile(fs, "/root/a.change.md", validChangefile("A"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/sub/b.change.md", validChangefile("B"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/sub/deep/c.change.md", validChangefile("C"), 0644))

	results, err := ReadAll(context.Background(), fs, "/root", ReadOptions{Workers: 2})
	require.NoError(t, err)

	assert.Len(t, results, 3)

	titles := make(map[string]bool)
	for _, cf := range results {
		titles[cf.Title] = true
	}
	assert.True(t, titles["A"])
	assert.True(t, titles["B"])
	assert.True(t, titles["C"])
}

func TestReadAll_IgnoresNonChangefiles(t *testing.T) {
	fs := afero.NewMemMapFs()

	require.NoError(t, afero.WriteFile(fs, "/root/a.change.md", validChangefile("A"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/readme.md", []byte("# Hello"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/notes.txt", []byte("notes"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/change.md", []byte("not a changefile"), 0644))

	results, err := ReadAll(context.Background(), fs, "/root", ReadOptions{Workers: 2})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "A", results[0].Title)
}

func TestReadAll_EmptyDirectory(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll("/empty", 0755))

	results, err := ReadAll(context.Background(), fs, "/empty", ReadOptions{Workers: 2})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestReadAll_InvalidFile(t *testing.T) {
	fs := afero.NewMemMapFs()

	require.NoError(t, afero.WriteFile(fs, "/root/bad.change.md", []byte("no frontmatter here"), 0644))

	_, err := ReadAll(context.Background(), fs, "/root", ReadOptions{Workers: 2})
	require.Error(t, err)
}

func TestReadAll_ManyFiles(t *testing.T) {
	fs := afero.NewMemMapFs()

	for i := range 100 {
		path := fmt.Sprintf("/root/change-%d.change.md", i)
		require.NoError(t, afero.WriteFile(fs, path, validChangefile(fmt.Sprintf("Change %d", i)), 0644))
	}

	results, err := ReadAll(context.Background(), fs, "/root", ReadOptions{Workers: 4})
	require.NoError(t, err)

	assert.Len(t, results, 100)
}

func TestReadAll_SortedPathOrder(t *testing.T) {
	fs := afero.NewMemMapFs()

	require.NoError(t, afero.WriteFile(fs, "/root/b.change.md", validChangefile("B"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/a.change.md", validChangefile("A"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/sub/c.change.md", validChangefile("C"), 0644))

	results, err := ReadAll(context.Background(), fs, "/root", ReadOptions{Workers: 3})
	require.NoError(t, err)

	paths := make([]string, len(results))
	for i, cf := range results {
		paths[i] = cf.SourcePath
	}
	assert.Equal(t, []string{"/root/a.change.md", "/root/b.change.md", "/root/sub/c.change.md"}, paths)
}

func TestFindAll(t *testing.T) {
	fs := afero.NewMemMapFs()

	require.NoError(t, afero.WriteFile(fs, "/root/b.change.md", validChangefile("B"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/a.change.md", validChangefile("A"), 0644))
	require.NoError(t, afero.WriteFile(fs, "/root/readme.md", []byte("# Hello"), 0644))

	paths, err := FindAll(fs, "/root")
	require.NoError(t, err)

	assert.Equal(t, []string{"/root/a.change.md", "/root/b.change.md"}, paths)
}

func TestFindAll_MissingRoot(t *testing.T) {
	fs := afero.NewMemMapFs()

	_, err := FindAll(fs, "/nope")
	require.Error(t, err)
}

func TestReadAll_DefaultWorkers(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/root/a.change.md", validChangefile("A"), 0644))

	results, err := ReadAll(context.Background(), fs, "/root", ReadOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
