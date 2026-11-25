package changelog

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadChangelogConfigInheritsFromRepositoryRoot(t *testing.T) {
	const config = `{"changelog":{"hideEmptyReleases":true}}`
	packageDir := filepath.Join("packages", "example")

	t.Run("relative in-memory root", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		repo := "checkout"
		root := filepath.Join(repo, packageDir)
		require.NoError(t, fs.MkdirAll(root, 0o755))
		require.NoError(t, fs.MkdirAll(filepath.Join(repo, ".git"), 0o755))
		require.NoError(t, afero.WriteFile(fs, filepath.Join(repo, HarkDir, "config.json"), []byte(config), 0o644))

		got, err := readChangelogConfig(Options{Fs: fs, Root: root})
		require.NoError(t, err)
		assert.True(t, got.HideEmptyReleases)
	})

	t.Run("real checkout with package as working directory", func(t *testing.T) {
		fs := afero.NewOsFs()
		repo := t.TempDir()
		root := filepath.Join(repo, packageDir)
		require.NoError(t, fs.MkdirAll(root, 0o755))
		require.NoError(t, fs.MkdirAll(filepath.Join(repo, ".git"), 0o755))
		require.NoError(t, fs.MkdirAll(filepath.Join(repo, HarkDir), 0o755))
		require.NoError(t, afero.WriteFile(fs, filepath.Join(repo, HarkDir, "config.json"), []byte(config), 0o644))
		t.Chdir(root)

		got, err := readChangelogConfig(Options{}.withDefaults())
		require.NoError(t, err)
		assert.True(t, got.HideEmptyReleases)
	})
}
