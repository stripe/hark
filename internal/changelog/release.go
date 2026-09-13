package changelog

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/afero"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

// Release cuts a new release from the unreleased changefiles.
// It stamps them with the version, adds the release to the releases file, and rebuilds the changelog.
//
// An entry for the version may already exist. Existing values are overwritten with new ones.
func Release(ctx context.Context, opts Options, release releases.Release) error {
	opts = opts.withDefaults()

	if release.Version == "" {
		return errors.New("release version is required")
	}

	versionChannel, ok := releases.Channel(release.Version)
	if !ok {
		return fmt.Errorf("%q is not a valid version: %s", release.Version, versionFormatHint)
	}

	releaseFile, err := releases.ReadFile(opts.Fs, opts.releasesPath())
	if err != nil {
		return err
	}

	// ensure the channel matches the file we're writing into
	if fileChannel := releaseFile.Metadata.Channel; slices.Contains(releases.Channels, fileChannel) &&
		versionChannel != fileChannel {
		return fmt.Errorf("%s is a %s version, but %s records %s releases",
			release.Version, versionChannel, releaseFile.SourcePath, fileChannel)
	}

	entry, err := placeRelease(releaseFile, release, opts.today())
	if err != nil {
		return err
	}
	inherit(entry, releaseFile)

	changes, err := changefile.ReadEvery(ctx, opts.Fs, opts.changesDir(), opts.ReadOptions)
	if err != nil {
		return err
	}

	var pending []*changefile.Changefile
	for _, c := range changes {
		if c.ReleasedInVersion == "" {
			pending = append(pending, c)
		}
	}

	// The releases file goes first.
	// If a changefile write fails after this, the release exists with fewer changes than it should
	// (which is recoverable by re-releasing this version).
	if err := releases.WriteFile(opts.Fs, opts.releasesPath(), releaseFile); err != nil {
		return err
	}

	for _, c := range pending {
		c.ReleasedInVersion = release.Version
		if err := c.WriteFile(opts.Fs, c.SourcePath); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(opts.Out, "released %s on %s (%d changes)\n",
		entry.Version, entry.ReleasedOn, len(pending)); err != nil {
		return err
	}

	// report whether or not an intro exists.
	if err := reportIntro(opts, entry.Version); err != nil {
		return err
	}

	return Build(ctx, opts)
}

// reportIntro says whether this release has an introduction, and where one would
// go if it does not.
func reportIntro(opts Options, version string) error {
	path := opts.introPath(version)

	exists, err := afero.Exists(opts.Fs, path)
	if err != nil {
		return fmt.Errorf("checking %s: %w", path, err)
	}

	if exists {
		_, err = fmt.Fprintf(opts.Out, "using intro %s\n", path)
	} else {
		_, err = fmt.Fprintf(opts.Out, "no intro found. Write one in %s and run `hark build` if you want it to be included in the changelog.\n", path)
	}
	return err
}

// placeRelease finds or creates releaseFile's entry for the version being cut,
// returning a pointer to it.
//
// A pre-existing entry is reconciled rather than replaced: values passed on the
// command line have to match what is stored, or fill a field that was left blank.
func placeRelease(releaseFile *releases.File, release releases.Release, today string) (*releases.Release, error) {
	index := indexOf(releaseFile, release.Version)
	if index < 0 {
		// The date has to be settled before the entry is placed, since it is what
		// decides where the entry goes.
		if release.ReleasedOn == "" {
			release.ReleasedOn = today
		}

		index = releaseFile.Insert(release)
		return &releaseFile.Releases[index], nil
	}
	existing := &releaseFile.Releases[index]

	fields := []struct {
		name     string
		stored   *string
		incoming string
	}{
		{"released_on", &existing.ReleasedOn, release.ReleasedOn},
		{"pinned_api_version", &existing.PinnedAPIVersion, release.PinnedAPIVersion},
		{"minimum_runtime_version", &existing.MinimumRuntimeVersion, release.MinimumRuntimeVersion},
	}

	// Report every conflict at once rather than making the caller rerun to find
	// the next one.
	var conflicts []error
	for _, f := range fields {
		switch {
		case f.incoming == "":
			// Nothing passed: keep what is stored.
		case *f.stored == "":
			*f.stored = f.incoming
		case *f.stored != f.incoming:
			conflicts = append(conflicts, fmt.Errorf("%s is %q, not %q", f.name, *f.stored, f.incoming))
		}
	}

	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%s already exists in the releases file and does not match: %w",
			release.Version, errors.Join(conflicts...))
	}

	// An entry added by hand may have recorded runtime metadata without a date.
	if existing.ReleasedOn == "" {
		existing.ReleasedOn = today
	}

	return existing, nil
}

// indexOf is the position of version in released, or -1 when it is not there.
func indexOf(released *releases.File, version string) int {
	for i := range released.Releases {
		if released.Releases[i].Version == version {
			return i
		}
	}
	return -1
}

// inherit copies the pinned_api_version and minimum_runtime_version from the release
// this one succeeded, if not already overridden.
func inherit(entry *releases.Release, released *releases.File) {
	prior := released.Predecessors()[entry.Version]
	if prior == nil {
		return
	}

	if entry.PinnedAPIVersion == "" {
		entry.PinnedAPIVersion = prior.PinnedAPIVersion
	}
	if entry.MinimumRuntimeVersion == "" {
		entry.MinimumRuntimeVersion = prior.MinimumRuntimeVersion
	}
}
