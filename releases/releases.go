// Package releases is responsible for the `releases.json` file that records every
// release a repo has published.
//
// The file looks like:
//
//	{
//	  "metadata": {
//	    "language": "go",
//	    "channel": "beta"
//	  },
//	  "releases": [
//	    {
//	      "version": "2.0.0",
//	      "released_on": "2024-01-15",
//	      "pinned_api_version": "2024-01-01",
//	      "minimum_runtime_version": "3.10"
//	    }
//	  ]
//	}
package releases

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/afero"
)

// Release represents a single release.
type Release struct {
	// e.g. "1.2.3" or "4.5.6-beta.1".
	Version string `json:"version"`
	// The ISO date of the release, e.g. "2026-09-11"
	ReleasedOn string `json:"released_on"`
	// the Stripe API version this release was pinned to. Empty for SDKs without pinned versions.
	PinnedAPIVersion string `json:"pinned_api_version,omitempty"`
	// the lowest version of the host language this release supports, e.g. "3.10" for Python. May be missing for some releases if we weren't able to backfill it with enough certainty.
	//
	// 6/7 of our SDKs have a single version identifier in this field; dotnet is the exception.
	// It tracks two different runtimes with independent floors. So it uses a comma-separated list of `label=value` segments like `"core=net6.0,framework=net462"`.
	// Either half may be absent, so consumers should parse based on label, not position in the string
	MinimumRuntimeVersion string `json:"minimum_runtime_version,omitempty"`
}

// Information about the `releases.json` file itself
type Metadata struct {
	Language   string `json:"language,omitempty"`
	Repository string `json:"repository,omitempty"`
	Channel    string `json:"channel"`
}

// File is the parsed contents of a releases JSON file. Versions are ordered by release date descending (ties broken by higher semver version first)
type File struct {
	Metadata Metadata  `json:"metadata"`
	Releases []Release `json:"releases"`

	// the full path that this struct originated from
	SourcePath string `json:"-"`
}

func ReadFile(fs afero.Fs, path string) (*File, error) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil, fmt.Errorf("reading releases file: %w", err)
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing releases JSON: %w", err)
	}

	f.SourcePath = path
	return &f, nil
}

func WriteFile(fs afero.Fs, path string, f *File) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing releases JSON: %w", err)
	}

	data = append(data, '\n')
	if err := afero.WriteFile(fs, path, data, 0644); err != nil {
		return fmt.Errorf("writing releases file: %w", err)
	}

	return nil
}

// for a given version, get the `Release` (if present)
func (f *File) Find(version string) *Release {
	for i := range f.Releases {
		if f.Releases[i].Version == version {
			return &f.Releases[i]
		}
	}
	return nil
}
