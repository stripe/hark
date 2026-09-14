package changelog

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/hark/changefile"
)

// runValidate runs Validate over fs and returns everything it printed along with its
// error.
func runValidate(t *testing.T, fs afero.Fs) (string, error) {
	t.Helper()

	var out bytes.Buffer
	err := Validate(context.Background(), Options{Fs: fs, Out: &out})
	return out.String(), err
}

// a valid changefile, for fixtures that need one alongside a broken one
const goodChangefile = "---\ntitle: \"Add widgets\"\nreleased_in_version: \"1.0.0\"\n---\n"

// gaMetadata is the metadata block every versions fixture needs, since Validate
// requires one and cross-checks its channel against the releases themselves.
const gaMetadata = `"metadata":{"language":"go","channel":"ga"},`

func TestValidate_CleanTree(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_add-widgets.change.md": goodChangefile,
			"2026-01-13_anniel_fix-retries.change.md": "---\ntitle: \"Fix retries\"\n---\n",
		})

	out, err := runValidate(t, fs)
	require.NoError(t, err)
	assert.Contains(t, out, "validated 2 changefiles")
}

// Two entries for one release would give it two changelog sections and two anchors,
// and leave every lookup by version depending on which one was found first.
func TestValidate_RejectsDuplicateReleases(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[
		{"version":"1.0.0","released_on":"2026-01-20"},
		{"version":"1.0.0","released_on":"2026-01-15"}
	]}`, map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "1.0.0 (2026-01-20) and 1.0.0 (2026-01-15) have the same version")
}

// `hark new` writes the placeholder title when the author gave nothing to use as one,
// so CI is what stops it reaching the changelog as a bullet.
func TestValidate_RejectsAPlaceholderTitle(t *testing.T) {
	for _, title := range []string{
		placeholderTitle,
		"FIXME",
		"FIXME: still deciding",
		// Written by hand, and just as much a placeholder.
		"fixme: come back to this",
	} {
		t.Run(title, func(t *testing.T) {
			fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
				map[string]string{
					"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"" + title + "\"\n---\n",
				})

			out, err := runValidate(t, fs)
			require.Error(t, err)
			assert.Contains(t, out, "title still starts with FIXME")
		})
	}
}

// A title that merely mentions the word is a real description, not a placeholder.
func TestValidate_AllowsFixmeLaterInATitle(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Remove a stale FIXME comment\"\n---\n",
		})

	out, err := runValidate(t, fs)
	require.NoError(t, err)
	assert.NotContains(t, out, "FIXME")
}

// A changefile's metadata describes its own pull request, so a link to another repo is
// either a typo or a file copied from a sibling SDK. This is what `check-github-links`
// used to do with awk over the pull request's diff.
func TestValidate_RejectsALinkToAnotherRepo(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"pr_url: \"https://github.com/stripe/stripe-python/pull/42\"\n---\n",
		})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "pr_url names stripe/stripe-python, but this repo is stripe/stripe-go")
}

// Issue links are held to the same rule, and are reported by index so the line is
// findable.
func TestValidate_RejectsAnIssueLinkToAnotherRepo(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"github_issues_resolved:\n" +
				"  - \"https://github.com/stripe/stripe-go/issues/1\"\n" +
				"  - \"https://github.com/stripe/stripe-ruby/issues/2\"\n---\n",
		})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "github_issues_resolved[1] names stripe/stripe-ruby")
	assert.NotContains(t, out, "github_issues_resolved[0]", "the first one is fine")
}

// The old action skipped anything that was not a GitHub URL, deferring to validate --
// which did not actually check. Now it does.
func TestValidate_RejectsALinkThatIsNotAGithubRepo(t *testing.T) {
	for _, link := range []string{
		"https://gitlab.com/stripe/stripe-go/pull/1",
		"https://github.com/stripe",
		"https://example.com/whatever",
	} {
		t.Run(link, func(t *testing.T) {
			fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
				map[string]string{
					"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
						"pr_url: \"" + link + "\"\n---\n",
				})

			out, err := runValidate(t, fs)
			require.Error(t, err)
			assert.Contains(t, out, "pr_url is not a github.com URL naming a repository")
		})
	}
}

// A link to this repo is the ordinary case and says nothing.
func TestValidate_AcceptsALinkToThisRepo(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"pr_url: \"https://github.com/stripe/stripe-go/pull/42\"\n" +
				"github_issues_resolved:\n  - \"https://github.com/stripe/stripe-go/issues/7\"\n---\n",
		})

	_, err := runValidate(t, fs)
	require.NoError(t, err)
}

// A malformed URL is reported once, by the field's own validation, rather than also as
// "not a github.com URL".
func TestValidate_ReportsAMalformedLinkOnlyOnce(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"pr_url: \"not-a-url\"\n---\n",
		})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "pr_url is not a valid URL")
	assert.NotContains(t, out, "not a github.com URL")
}

// The point of validate is that one run tells you everything to fix, so every
// problem in a file is reported, not just the first.
func TestValidate_ReportsEveryProblemInAFile(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"oops.change.md": "---\npr_url: \"not-a-url\"\nreleased_in_version: \"9.9.9\"\n---\n",
		})

	out, err := runValidate(t, fs)
	require.Error(t, err)

	assert.Contains(t, out, "title is required")
	assert.Contains(t, out, "pr_url")
	assert.Contains(t, out, "9.9.9")
	assert.Contains(t, out, "{date}")
	assert.Contains(t, err.Error(), "1/1 changefiles are invalid")
}

// Likewise across files: a file that will not parse must not hide the problems in
// the ones after it, which is why this cannot use changefile.ReadAll.
func TestValidate_ReportsProblemsAcrossFiles(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-11_xavdid_unparseable.change.md": "no frontmatter here\n",
			"2026-01-12_xavdid_no-title.change.md":    "---\nsection: \"Added\"\n---\n",
			"2026-01-13_xavdid_fine.change.md":        goodChangefile,
		})

	out, err := runValidate(t, fs)
	require.Error(t, err)

	assert.Contains(t, out, "unparseable")
	assert.Contains(t, out, "no-title")
	assert.NotContains(t, out, "2026-01-13_xavdid_fine")
	assert.Contains(t, err.Error(), "2/3 changefiles are invalid")
}

// The compiled changelog orders changes within a release by filename, so a
// name without a date sorts wrong without ever failing a build.
func TestValidate_CatchesABadFilename(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"my-change.change.md": goodChangefile})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "my-change.change.md")
}

// Build fails on an unknown released_in_version, but not until release time.
// Catching it in CI is the whole reason validate reads the releases file.
func TestValidate_CatchesAnUnknownVersion(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\nreleased_in_version: \"9.9.9\"\n---\n",
		})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, `released_in_version "9.9.9" is not in .hark/releases.json`)
}

// A misspelled version is reported as one. Looking it up first would report a release
// nobody has recorded, which is true but sends you to the wrong file to fix it.
func TestValidate_CatchesAMisspelledVersion(t *testing.T) {
	for _, version := range []string{"1.2", "01.0.0", "1.0.0-beta", "1.0.0-beta1"} {
		t.Run(version, func(t *testing.T) {
			fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
				map[string]string{
					"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\nreleased_in_version: \"" + version + "\"\n---\n",
				})

			out, err := runValidate(t, fs)
			require.Error(t, err)
			assert.Contains(t, out, `released_in_version "`+version+`" is not a valid version`)
			assert.NotContains(t, out, "is not in .hark/releases.json")
		})
	}
}

// An empty repo is a fine state to be in — it says so rather than erroring on the
// missing directory.
func TestValidate_NoChangefiles(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[]}`, map[string]string{})

	out, err := runValidate(t, fs)
	require.NoError(t, err)
	assert.Contains(t, out, "no changefiles found")
}

func TestValidate_MissingChangesDirectory(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, versionsFixturePath, []byte(`{"releases":[]}`), 0o644))

	_, err := runValidate(t, fs)
	require.Error(t, err)
}

// Reports are grouped under the file they belong to, in path order, so the output
// reads the same way twice.
func TestValidate_GroupsByFileInPathOrder(t *testing.T) {
	fs := buildFixture(t, `{"releases":[]}`, map[string]string{
		"2026-01-11_xavdid_first.change.md":  "---\nsection: \"Added\"\n---\n",
		"2026-01-12_xavdid_second.change.md": "---\nsection: \"Added\"\n---\n",
	})

	out, err := runValidate(t, fs)
	require.Error(t, err)

	assert.Less(t, strings.Index(out, "first"), strings.Index(out, "second"))
	assert.Contains(t, out, ".hark/changes/2026-01-11_xavdid_first.change.md\n  - title is required\n")
}

// An intro is found only by its filename, so a typo in it means the prose someone
// wrote silently never appears. Build cannot notice — it only looks for the names
// it expects — so this is the only thing that catches it.
func TestValidate_CatchesAMisnamedIntro(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})
	require.NoError(t, afero.WriteFile(fs, introsFixtureDir+"/1.0.0.md", []byte("Prose.\n"), 0o644))

	out, err := runValidate(t, fs)
	require.Error(t, err)

	assert.Contains(t, out, "1.0.0.md")
	assert.Contains(t, out, "intro-1.2.3.md")
	assert.Contains(t, err.Error(), "1/1 intros are invalid")
}

// A version-shaped name is the other half of being found: nothing will ever match an
// intro named after something that cannot be a release, so it is as dead as a typo'd one.
func TestValidate_CatchesAnIntroNamedAfterANonVersion(t *testing.T) {
	for _, name := range []string{"intro-1.md", "intro-1.2.md", "intro-next.md", "intro-1.0.0-rc.1.md"} {
		t.Run(name, func(t *testing.T) {
			fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
				map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})
			require.NoError(t, afero.WriteFile(fs, introsFixtureDir+"/"+name, []byte("Prose.\n"), 0o644))

			out, err := runValidate(t, fs)
			require.Error(t, err)
			assert.Contains(t, out, name)
			assert.Contains(t, err.Error(), "1/1 intros are invalid")
		})
	}
}

// Writing an intro before the release it belongs to is a normal way to work — the
// prose for a major version gets drafted well ahead of the cut — so an intro for a
// version that does not exist yet is accepted and simply sits unused.
func TestValidate_AcceptsAnIntroForAnUnreleasedVersion(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})
	writeIntro(t, fs, "2.0.0", "Drafted for the next major.\n")

	out, err := runValidate(t, fs)
	require.NoError(t, err)
	assert.Contains(t, out, "validated 1 changefiles")
}

func TestValidate_AcceptsAWellNamedIntro(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})
	writeIntro(t, fs, "1.0.0", "Prose.\n")

	out, err := runValidate(t, fs)
	require.NoError(t, err)
	assert.Contains(t, out, "validated 1 changefiles")
}

// Bad changefiles and bad intros are separate counts, so neither hides the other.
func TestValidate_ReportsBothKindsOfProblem(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_broken.change.md": "---\nsection: \"Added\"\n---\n"})
	require.NoError(t, afero.WriteFile(fs, introsFixtureDir+"/1.0.0.md", []byte("Prose.\n"), 0o644))

	_, err := runValidate(t, fs)
	require.Error(t, err)
	// One error per category, joined — so neither line can crowd the other out.
	assert.Contains(t, err.Error(), "1/1 changefiles are invalid")
	assert.Contains(t, err.Error(), "1/1 intros are invalid")
}

// Validating 1000+ changefiles is the expected case, so the parallel path has to
// keep the reports lined up with their files.
func TestValidate_ManyFilesWithBoundedWorkers(t *testing.T) {
	changefiles := map[string]string{}
	for i := range 50 {
		name := "2026-01-01_xavdid_change-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".change.md"
		if i%10 == 0 {
			changefiles[name] = "---\nsection: \"Added\"\n---\n" // no title
			continue
		}
		changefiles[name] = "---\ntitle: \"Fine\"\n---\n"
	}

	fs := buildFixture(t, `{`+gaMetadata+`"releases":[]}`, changefiles)

	var out bytes.Buffer
	err := Validate(context.Background(), Options{Fs: fs, Out: &out, ReadOptions: changefile.ReadOptions{Workers: 4}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5/50 changefiles are invalid")
}

// The language becomes a repository name, so it has to be one hark knows about: a
// typo would otherwise render links to a repository that does not exist.
func TestValidate_RejectsAnUnknownLanguage(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"golang","channel":"ga"},`+
		`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, `metadata.language "golang" is not one of`)
	// Listed in the canonical SDK order, not alphabetically.
	assert.Contains(t, out, "java, python, ruby, php, go, node, dotnet")
}

func TestValidate_AcceptsEverySDKLanguage(t *testing.T) {
	for _, language := range sdkLanguages {
		t.Run(language, func(t *testing.T) {
			fs := buildFixture(t, `{"metadata":{"language":"`+language+`","channel":"ga"},`+
				`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
				map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})

			_, err := runValidate(t, fs)
			require.NoError(t, err)
		})
	}
}

// The intro count is out of every intro, not just the bad ones — a denominator equal
// to the numerator would say nothing.
func TestValidate_IntroCountIsOutOfAllIntros(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})
	writeIntro(t, fs, "1.0.0", "Well named.\n")
	require.NoError(t, afero.WriteFile(fs, introsFixtureDir+"/nope.md", []byte("x\n"), 0o644))
	require.NoError(t, afero.WriteFile(fs, introsFixtureDir+"/also-wrong.md", []byte("x\n"), 0o644))

	_, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2/3 intros are invalid")
}

// A releases file can be wrong in several ways at once, and all of them are reported
// — the summary names the file rather than counting, since every problem is already
// listed above it.
func TestValidate_ReportsEveryMetadataProblem(t *testing.T) {
	// An unknown language, a channel that disagrees with the releases, and a broken
	// changefile alongside them.
	fs := buildFixture(t, `{"metadata":{"language":"golang","channel":"ga"},`+
		`"releases":[{"version":"1.0.0-beta.1","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_broken.change.md": "---\nsection: \"Added\"\n---\n"})

	out, err := runValidate(t, fs)
	require.Error(t, err)

	assert.Contains(t, out, `metadata.language "golang" is not one of`)
	assert.Contains(t, out, `metadata.channel is "ga" but 1/1 releases belong to a different channel (including 1.0.0-beta.1)`)

	// And the metadata summary is not crowded out by the changefile one.
	assert.Contains(t, err.Error(), "1/1 changefiles are invalid")
	assert.Contains(t, err.Error(), ".hark/releases.json has invalid metadata")
}

// A file whose releases disagree among themselves used to go unchecked: deriving one
// channel for the whole file gave no answer to compare against, so the check was
// skipped. Every release is asked individually now, so the odd ones out are named.
func TestValidate_ReportsReleasesInTheWrongChannel(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"go","channel":"ga"},"releases":[`+
		`{"version":"1.2.0","released_on":"2026-03-01"},`+
		`{"version":"1.1.0-beta.1","released_on":"2026-02-01"},`+
		`{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, `metadata.channel is "ga" but 1/3 releases belong to a different channel (including 1.1.0-beta.1)`)
}

// A channel that is not a channel at all, as against one that merely disagrees.
func TestValidate_RejectsAnUnknownChannel(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"go","channel":"stable"},`+
		`"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, `metadata.channel "stable" is not one of: ga, beta, private-preview`)
	// And it does not go on to blame the releases, which are fine.
	assert.NotContains(t, out, "belong to a different channel")
}

// Only the first few offenders are named: a metadata block copied from another branch
// makes every release mismatch, and hundreds of lines would bury the rest.
func TestValidate_CapsTheReleasesItNames(t *testing.T) {
	var entries []string
	for i := 9; i >= 0; i-- {
		entries = append(entries, fmt.Sprintf(`{"version":"1.%d.0-beta.1","released_on":"2026-01-%02d"}`, i, i+1))
	}
	fs := buildFixture(t, `{"metadata":{"language":"go","channel":"ga"},"releases":[`+
		strings.Join(entries, ",")+`]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n---\n"})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "10/10 releases belong to a different channel")
	// Three named, not ten.
	assert.Equal(t, 3, strings.Count(out, "-beta.1"))
}

// A version hark cannot read is rejected rather than quietly ordered as GA. node
// tagged a 2.1.0rc1 in 2013 and never announced it; if one ever reached a releases
// file it would sort above the betas it followed.
func TestValidate_RejectsAnUnreadableVersion(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"node","channel":"ga"},"releases":[`+
		`{"version":"2.1.0","released_on":"2013-01-02"},`+
		`{"version":"2.1.0rc1","released_on":"2013-01-01"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n---\n"})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "1/2 releases are not valid versions (including 2.1.0rc1)")

	// Reported once, as unreadable — not also as being in the wrong channel.
	assert.NotContains(t, out, "belong to a different channel")
}

// An unreadable version has no channel to compare, so the two checks do not both fire
// for the same entry — but a genuinely misplaced release alongside it still does.
func TestValidate_SeparatesUnreadableFromMisplaced(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"go","channel":"ga"},"releases":[`+
		`{"version":"1.2.0","released_on":"2026-03-01"},`+
		`{"version":"1.1.0-beta.1","released_on":"2026-02-01"},`+
		`{"version":"not-a-version","released_on":"2026-01-15"}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n---\n"})

	out, err := runValidate(t, fs)
	require.Error(t, err)
	assert.Contains(t, out, "1/3 releases are not valid versions (including not-a-version)")
	assert.Contains(t, out, "1/3 releases belong to a different channel (including 1.1.0-beta.1)")
}

// released_on is a lexical sort key and a rendered heading, so a value that isn't a date
// silently misorders the changelog rather than failing anywhere.
func TestValidate_RejectsAReleaseDateThatIsNotADate(t *testing.T) {
	for _, date := range []string{"2026-99-99", "2026-1-15", "01-15-2026", "yesterday", "2026-01-15T00:00:00Z"} {
		t.Run(date, func(t *testing.T) {
			fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":"`+date+`"}]}`,
				map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})

			out, err := runValidate(t, fs)
			require.Error(t, err)
			assert.Contains(t, out, "1/1 releases have a released_on that is not a date")
			assert.Contains(t, out, fmt.Sprintf("1.0.0 (%q)", date))
		})
	}
}

// An entry can be recorded before it is dated: `release` fills the date in, and a release
// without one renders under a heading of just its version.
func TestValidate_AcceptsAReleaseWithNoDate(t *testing.T) {
	fs := buildFixture(t, `{`+gaMetadata+`"releases":[{"version":"1.0.0","released_on":""}]}`,
		map[string]string{"2026-01-14_xavdid_add-widgets.change.md": goodChangefile})

	out, err := runValidate(t, fs)
	require.NoError(t, err)
	assert.Contains(t, out, "validated 1 changefiles")
}
