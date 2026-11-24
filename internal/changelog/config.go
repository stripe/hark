package changelog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

type changelogConfig struct {
	HideEmptyReleases bool `json:"hideEmptyReleases"`
}

func readChangelogConfig(opts Options) (changelogConfig, error) {
	var config struct {
		Changelog changelogConfig `json:"changelog"`
	}
	root := opts.Root
	path := filepath.Join(root, HarkDir, "config.json")
	data, err := afero.ReadFile(opts.Fs, path)
	if os.IsNotExist(err) {
		absolute, absErr := filepath.Abs(root)
		if absErr != nil {
			return config.Changelog, absErr
		}
		for candidate := absolute; ; candidate = filepath.Dir(candidate) {
			if _, statErr := opts.Fs.Stat(filepath.Join(candidate, ".git")); statErr == nil {
				path = filepath.Join(candidate, HarkDir, "config.json")
				data, err = afero.ReadFile(opts.Fs, path)
				break
			} else if !os.IsNotExist(statErr) {
				return config.Changelog, statErr
			}
			if candidate == filepath.Dir(candidate) {
				break
			}
		}
	}
	if os.IsNotExist(err) {
		return config.Changelog, nil
	}
	if err != nil {
		return config.Changelog, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config.Changelog, fmt.Errorf("reading %s: %w", path, err)
	}
	return config.Changelog, nil
}
