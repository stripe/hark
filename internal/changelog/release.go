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
// The repo has to pass [Validate] first, so a release cannot bake a placeholder title or
// a half-written changefile into the changelog.
//
// An entry for the version may already exist. Values it records are kept, and anything passed
// that contradicts one is an error.
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

	// Validate everything before proceeding with the release
	validChangefiles, err := validate(ctx, opts)
	if err != nil {
		return fmt.Errorf("not releasing %s: %w", release.Version, err)
	}

	entry, err := placeRelease(releaseFile, release, opts.today())
	if err != nil {
		return err
	}
	inherit(entry, releaseFile)

	// filter all the valid changefiles to find the pending ones
	var pending []*changefile.Changefile
	for _, r := range validChangefiles {
		if r.Changefile.ReleasedInVersion == "" {
			pending = append(pending, r.Changefile)
		}
	}

	// The releases file is written first.
	// If a changefile write fails after this, the release exists with fewer changes than it should
	// (which is recoverable by re-running `release`).
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

	if releaseFile.Metadata.Language != "" {
		if seedErr := seedNextMigrationGuide(opts, entry.Version); seedErr != nil {
			if _, err := fmt.Fprintf(opts.Out, "warning: failed to proactively create the next migration guide (%v); you can safely ignore this. The release was not affected\n", seedErr); err != nil {
				return err
			}
		}
	}

	return Build(ctx, opts)
}

const migrationGuideTemplate = `# Migration guide for v%d

v%d of the SDK bumps the API version to ` + "`TKTK`" + `. See the API changelog for more information: TKTK

<!-- This is the migration guide for the next major version!
If you're making breaking changes, add a new h2 header with a nice title and write a detailed guide to help users upgrade.
You will almost certainly need before/after code examples and information about which users this change affects.
See: https://github.com/stripe/hark#writing-a-great-migration-guide
-->
`

// in general, we always want the next migration guide available, so we create it proactively when we're making releases
func seedNextMigrationGuide(opts Options, version string) error {
	// only write a new migration guides for GA
	if channel, _ := releases.Channel(version); channel != releases.ChannelGA {
		return nil
	}

	major, _ := releases.Major(version)
	next := major + 1
	path := opts.migrationGuidePath(next)

	exists, err := afero.Exists(opts.Fs, path)
	if err != nil {
		return fmt.Errorf("checking %s: %w", path, err)
	}
	if exists {
		return nil
	}

	dir := opts.migrationGuidesDir()
	if err := opts.Fs.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	guide := fmt.Sprintf(migrationGuideTemplate, next, next)
	if err := afero.WriteFile(opts.Fs, path, []byte(guide), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	_, err = fmt.Fprintf(opts.Out, "proactively seeded the %s migration guide\n", path)
	return err
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
		_, err = fmt.Fprintf(opts.Out, "there's not an intro for this release. If you want one, write it in %s and run `hark build`.\n", path)
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

	// The match is by meaning, not spelling, so an entry can be found under a name that isn't
	// the one passed. Recording both would break the file's one-entry-per-release invariant.
	if existing.Version != release.Version {
		return nil, fmt.Errorf("%s is already recorded as %s, which is the same release; cut it under that name",
			release.Version, existing.Version)
	}

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
//
// Versions are matched using a normalized value rather than by string match, since `1.0.0b1` and `1.0.0-beta.1` should be treated as duplicates
func indexOf(released *releases.File, version string) int {
	for i := range released.Releases {
		if releases.Compare(released.Releases[i].Version, version) == 0 {
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
