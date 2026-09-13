package changelog

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/afero"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

// fileReport is everything wrong with one changefile.
type fileReport struct {
	Path   string
	Errors []error
}

const versionFormatHint = "expected major.minor.patch, optionally followed by an alpha or beta suffix"

// validates the holistic state state of a repo. See the individual check* functions for what we look for
func Validate(ctx context.Context, opts Options) error {
	_, err := validate(ctx, opts)
	return err
}

// validate powers the public [Validate] and returns all of the validated changefiles
func validate(ctx context.Context, opts Options) ([]changefile.ReadResult, error) {
	opts = opts.withDefaults()

	// read every changefile, keeping each one's outcome rather than stopping at the
	// first that will not parse
	changefiles, err := changefile.ReadAll(ctx, opts.Fs, opts.changesDir(), opts.ReadOptions)
	if err != nil {
		return nil, err
	}
	if len(changefiles) == 0 {
		if _, err := fmt.Fprintf(opts.Out, "no changefiles found in %s\n", opts.changesDir()); err != nil {
			return nil, err
		}
	}
	releasesFile, err := releases.ReadFile(opts.Fs, opts.releasesPath())
	if err != nil {
		return nil, err
	}

	// generate reports
	changefileReports := validateChangefiles(changefiles, releasesFile)
	introReports, err := checkIntros(opts)
	if err != nil {
		return nil, err
	}
	metadataReports := checkMetadata(releasesFile)

	// print details of issues
	numBadChangefiles, err := printReports(opts, changefileReports)
	if err != nil {
		return nil, err
	}
	numBadIntros, err := printReports(opts, introReports)
	if err != nil {
		return nil, err
	}
	numBadMetadata, err := printReports(opts, metadataReports)
	if err != nil {
		return nil, err
	}

	// summarize and error out if we found anything
	var errs []error
	if numBadChangefiles > 0 {
		errs = append(errs, fmt.Errorf("%d/%d changefiles are invalid", numBadChangefiles, len(changefiles)))
	}
	if numBadIntros > 0 {
		errs = append(errs, fmt.Errorf("%d/%d intros are invalid", numBadIntros, len(introReports)))
	}
	if numBadMetadata > 0 {
		errs = append(errs, fmt.Errorf("%s has invalid metadata", releasesFile.SourcePath))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// return successfully
	_, err = fmt.Fprintf(opts.Out, "validated %d changefiles\n", len(changefiles))
	return changefiles, err
}

// printReports writes every report that had an error, returning the number it wrote.
func printReports(opts Options, reports []fileReport) (int, error) {
	var found int

	for _, r := range reports {
		if len(r.Errors) == 0 {
			continue
		}
		found++

		if _, err := fmt.Fprintf(opts.Out, "%s\n", r.Path); err != nil {
			return 0, err
		}
		for _, e := range r.Errors {
			if _, err := fmt.Fprintf(opts.Out, "  - %s\n", e); err != nil {
				return 0, err
			}
		}
	}

	return found, nil
}

func firstFew(versions []string) string {
	if len(versions) > 3 {
		versions = versions[:3]
	}
	return strings.Join(versions, ", ")
}

// checks the values in the top-level `metadata` property, that every release is a
// version hark can read, and that all of them match the file's channel
func checkMetadata(releasesFile *releases.File) []fileReport {
	if releasesFile == nil {
		return nil
	}

	var errs []error
	switch language := releasesFile.Metadata.Language; {
	case language == "":
		errs = append(errs, errors.New("metadata.language is required, but missing"))
	case !slices.Contains(sdkLanguages, language):
		errs = append(errs, fmt.Errorf("metadata.language %q is not one of: %s",
			language, strings.Join(sdkLanguages, ", ")))
	}

	channel := releasesFile.Metadata.Channel
	channelKnown := slices.Contains(releases.Channels, channel)
	switch {
	case channel == "":
		errs = append(errs, errors.New("metadata.channel is required, but missing"))
	case !channelKnown:
		errs = append(errs, fmt.Errorf("metadata.channel %q is not one of: %s",
			channel, strings.Join(releases.Channels, ", ")))
	}

	// check each release's version for validity and that it's in the right channel
	var unreadable, wrongChannel []string
	for _, release := range releasesFile.Releases {
		versionChannel, ok := releases.Channel(release.Version)
		switch {
		case !ok:
			unreadable = append(unreadable, release.Version)
		case channelKnown && versionChannel != channel:
			wrongChannel = append(wrongChannel, release.Version)
		}
	}

	total := len(releasesFile.Releases)
	if len(unreadable) > 0 {
		errs = append(errs, fmt.Errorf("%d/%d releases are not valid versions (including %s)",
			len(unreadable), total, firstFew(unreadable)))
	}
	if len(wrongChannel) > 0 {
		errs = append(errs, fmt.Errorf("metadata.channel is %q but %d/%d releases belong to a different channel (including %s)",
			channel, len(wrongChannel), total, firstFew(wrongChannel)))
	}

	if before, after, unsorted := releasesFile.Unsorted(); unsorted {
		errs = append(errs, fmt.Errorf("releases are out of order: %s (%s) should not come before %s (%s); newest date first, ties by highest version",
			before.Version, before.ReleasedOn, after.Version, after.ReleasedOn))
	}

	if a, b, duplicate := releasesFile.FirstDuplicate(); duplicate {
		errs = append(errs, fmt.Errorf("%s (%s) and %s (%s) have the same version string. Version strings should be unique",
			a.Version, a.ReleasedOn, b.Version, b.ReleasedOn))
	}

	if len(errs) == 0 {
		return nil
	}
	return []fileReport{{Path: releasesFile.SourcePath, Errors: errs}}
}

// looks for intros that will never be rendered because their name does not follow the `intro-1.2.3.md`.
// It allows for intros without a corresponding entry in the releases file so we can pre-write intros.
func checkIntros(opts Options) ([]fileReport, error) {
	entries, err := afero.ReadDir(opts.Fs, opts.introsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no intros is an acceptable state
		}
		return nil, fmt.Errorf("reading %s: %w", opts.introsDir(), err)
	}

	var reports []fileReport
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		report := fileReport{Path: filepath.Join(opts.introsDir(), e.Name())}
		if _, ok := IntroVersion(e.Name()); !ok {
			report.Errors = []error{
				fmt.Errorf("name should look like %s, so this file is ignored", IntroName("1.2.3")),
			}
		}
		reports = append(reports, report)
	}

	return reports, nil
}

// returns one [fileReport] per changefile, in sorted path order.
func validateChangefiles(results []changefile.ReadResult, releasesFile *releases.File) []fileReport {
	// Go has no `[].map(...)`, so here we are
	reports := make([]fileReport, len(results))
	for i, r := range results {
		reports[i] = validateChangefile(r, releasesFile)
	}
	return reports
}

// validateChangefile collects everything wrong with one changefile. If a file can't be parsed, that's the only error reported (since we can't determine anything else)
func validateChangefile(result changefile.ReadResult, releasesFile *releases.File) fileReport {
	errs := changefile.ValidateName(result.Path)

	if result.Err != nil {
		return fileReport{Path: result.Path, Errors: append(errs, result.Err)}
	}
	cf := result.Changefile

	errs = append(errs, cf.Validate()...)

	// Checked before looking it up, so a misspelled version is reported as one rather
	// than as a release nobody has recorded -- which is true, but sends you to the
	// wrong file to fix it.
	switch {
	case cf.ReleasedInVersion == "":
		// nothing to check: the change has not shipped yet
	case !releases.IsVersionValid(cf.ReleasedInVersion):
		errs = append(errs, fmt.Errorf("released_in_version %q is not a valid version: %s",
			cf.ReleasedInVersion, versionFormatHint))
	case releasesFile != nil && releasesFile.Find(cf.ReleasedInVersion) == nil:
		errs = append(errs, fmt.Errorf("released_in_version %q is not in %s",
			cf.ReleasedInVersion, releasesFile.SourcePath))
	}

	errs = append(errs, checkLinks(cf, releasesFile)...)

	return fileReport{Path: result.Path, Errors: errs}
}

// a link-valued frontmatter field, named the way the frontmatter names it so an error
// points at the line to fix.
type link struct {
	field string
	url   string
}

func changefileLinks(cf *changefile.Changefile) []link {
	var links []link
	if cf.PRUrl != "" {
		links = append(links, link{"pr_url", cf.PRUrl})
	}
	for i, issue := range cf.GithubIssuesResolved {
		links = append(links, link{fmt.Sprintf("github_issues_resolved[%d]", i), issue})
	}
	return links
}

// reports metadata that names a GitHub pr/issue outside this repo.
func checkLinks(cf *changefile.Changefile, releasesFile *releases.File) []error {
	if releasesFile == nil {
		return nil
	}
	repo := sdkRepo(releasesFile.Metadata.Language)
	if repo == "" {
		return nil
	}

	var errs []error
	for _, l := range changefileLinks(cf) {
		u, err := url.ParseRequestURI(l.url)
		if err != nil {
			continue // not a URL at all, which is enforced elsewhere. ignore here
		}

		cited, ok := githubRepo(u)
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%s is not a github.com URL naming a repository", l.field))
		case cited != repo:
			errs = append(errs, fmt.Errorf("%s names %s, but this repo is %s", l.field, cited, repo))
		}
	}
	return errs
}

// githubRepo is the "owner/name" a GitHub URL points at.
func githubRepo(u *url.URL) (string, bool) {
	if u.Host != "github.com" {
		return "", false
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}
