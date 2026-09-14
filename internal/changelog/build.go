package changelog

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/spf13/afero"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

// compiles every changefile in a repo into a single markdown file.
//
// Changes are grouped by their `released_in_version` property. Within a version, they're ordered by date ascending. Changes without a `released_in_version` are grouped under an "Unreleased" heading.
//
// Versions are rendered based on their position in `releases.json`.
func Build(ctx context.Context, opts Options) error {
	opts = opts.withDefaults()

	releaseFile, releaseGroups, err := loadAllData(ctx, opts)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	metadata := releaseFile.Metadata
	if err := render(&buf, metadata.Language, metadata.Channel, releaseGroups); err != nil {
		return err
	}

	path := opts.changelogPath()
	if err := afero.WriteFile(opts.Fs, path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	_, err = fmt.Fprintf(opts.Out, "wrote %s (%d versions)\n",
		path, len(releaseFile.Releases))
	return err
}

// loadAllData preps everything we need to render a changelog
func loadAllData(ctx context.Context, opts Options) (*releases.File, []releaseGroup, error) {
	changes, err := changefile.ReadAllOrFail(ctx, opts.Fs, opts.changesDir(), opts.ReadOptions)
	if err != nil {
		return nil, nil, err
	}

	released, err := releases.ReadFile(opts.Fs, opts.releasesPath())
	if err != nil {
		return nil, nil, err
	}

	groups, err := groupChanges(changes, released)
	if err != nil {
		return nil, nil, err
	}

	if err := loadIntros(opts, groups); err != nil {
		return nil, nil, err
	}

	return released, groups, nil
}

// For each release, attach the contents of its intro file if one exists.
func loadIntros(opts Options, groups []releaseGroup) error {
	for i := range groups {
		if groups[i].Release == nil {
			continue // the unreleased group has no version to name a file after
		}

		path := opts.introPath(groups[i].Release.Version)
		data, err := afero.ReadFile(opts.Fs, path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reading %s: %w", path, err)
		}

		groups[i].Intro = string(data)
	}

	return nil
}
