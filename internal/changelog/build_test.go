package changelog

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildFixture is a filesystem laid out the way hark expects, seeded with a
// releases file and changefiles. changefiles maps a changefile's name to its
// contents.
func buildFixture(t *testing.T, versionsJSON string, changefiles map[string]string) afero.Fs {
	t.Helper()

	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, versionsFixturePath, []byte(versionsJSON), 0o644))
	require.NoError(t, fs.MkdirAll(changesFixtureDir, 0o755))
	for name, content := range changefiles {
		require.NoError(t, afero.WriteFile(fs, changesFixtureDir+"/"+name, []byte(content), 0o644))
	}
	return fs
}

// The fixed layout, spelled out rather than derived, so the tests would notice a
// change to it.
const (
	versionsFixturePath = ".hark/releases.json"
	changesFixtureDir   = ".hark/changes"
	introsFixtureDir    = ".hark/intros"
	changelogPath       = "CHANGELOG.md"
)

// writeIntro puts a release's introduction where Build will look for it.
func writeIntro(t *testing.T, fs afero.Fs, version, text string) {
	t.Helper()

	path := introsFixtureDir + "/intro-" + version + ".md"
	require.NoError(t, afero.WriteFile(fs, path, []byte(text), 0o644))
}

// build runs Build over fs and returns the compiled changelog.
func build(t *testing.T, fs afero.Fs) string {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, Build(context.Background(), Options{Fs: fs, Out: &out}))

	got, err := afero.ReadFile(fs, changelogPath)
	require.NoError(t, err)
	return string(got)
}

// TestBuild_Golden documents the compiled changelog's format. It is the closest
// thing to a written spec for the output, so it asserts on exact bytes.
func TestBuild_Golden(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"2.0.0","released_on":"2024-03-01"},
		{"version":"1.0.0","released_on":"2024-01-15"}
	]}`, map[string]string{
		"2024-02-28-alice-add-widgets.change.md": "---\n" +
			"title: \"Add support for widgets\"\n" +
			"pr_url: \"https://github.com/stripe/stripe-go/pull/42\"\n" +
			"released_in_version: \"2.0.0\"\n" +
			"---\n\n- widgets can now be created\n- and destroyed\n",
		"2024-02-27-bob-drop-gizmos.change.md": "---\n" +
			"title: \"Remove the gizmo resource\"\n" +
			"pr_url: \"https://github.com/stripe/stripe-go/pull/41\"\n" +
			"is_breaking: true\n" +
			"section: \"⚠️ Removed\"\n" +
			"released_in_version: \"2.0.0\"\n---\n",
		"2024-01-14-carol-first.change.md": "---\n" +
			"title: \"Initial release\"\nreleased_in_version: \"1.0.0\"\n---\n",
	})
	writeIntro(t, fs, "2.0.0", "This release changes the pinned API version to 2024-02-01.")

	want := generatedNotice + `

# Changelog

## <a id="2-0-0"></a>2.0.0 - 2024-03-01
This release changes the pinned API version to 2024-02-01.

* [#42](https://github.com/stripe/stripe-go/pull/42) Add support for widgets
  - widgets can now be created
  - and destroyed

### ⚠️ Removed
* ⚠️ [#41](https://github.com/stripe/stripe-go/pull/41) Remove the gizmo resource

## <a id="1-0-0"></a>1.0.0 - 2024-01-15
* Initial release
`

	assert.Equal(t, want, build(t, fs))
}

func TestBuild_UnreleasedRendersFirst(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"2024-02-01-alice-pending.change.md": "---\ntitle: \"Not shipped yet\"\n---\n",
			"2024-01-14-bob-shipped.change.md":   "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Less(t, strings.Index(got, "## Unreleased"), strings.Index(got, "## <a id=\"1-0-0\"></a>1.0.0"))
	assert.Contains(t, got, "## Unreleased\n* Not shipped yet\n")
}

// With nothing unreleased there should be no Unreleased heading at all.
func TestBuild_OmitsUnreleasedWhenEmpty(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{"a.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n"})

	assert.NotContains(t, build(t, fs), "Unreleased")
}

// Every released version is represented even when nothing shipped in it, so the
// changelog is a complete record of releases.
func TestBuild_RendersVersionsWithNoChanges(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"1.1.0","released_on":"2024-02-01"},
		{"version":"1.0.0","released_on":"2024-01-15"}
	]}`, map[string]string{
		"a.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n",
	})

	got := build(t, fs)
	assert.Contains(t, got, "## <a id=\"1-1-0\"></a>1.1.0 - 2024-02-01\n\n## <a id=\"1-0-0\"></a>1.0.0 - 2024-01-15\n")
}

// The warning marker follows the level a change calls for, so a migrated changefile that
// records semver_level rather than is_breaking still reads as breaking.
func TestBuild_MajorChangesAreMarked(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"Remove the Orders resource\"\nsemver_level: major\nreleased_in_version: \"1.0.0\"\n---\n",
			"b.change.md": "---\ntitle: \"Add widgets\"\nsemver_level: minor\nreleased_in_version: \"1.0.0\"\n---\n",
			// TODO(semver-level): remove with is_breaking.
			"c.change.md": "---\ntitle: \"Remove the Charges resource\"\nis_breaking: true\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Contains(t, got, "* ⚠️ Remove the Orders resource\n")
	assert.Contains(t, got, "* Add widgets\n")
	assert.Contains(t, got, "* ⚠️ Remove the Charges resource\n")
}

// An entry recorded before it ships has no date to put in its heading, and still gets
// the anchor that links to it.
func TestBuild_ReleaseWithNoDateHeadsWithJustItsVersion(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":""}]}`,
		map[string]string{"a.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n"})

	assert.Contains(t, build(t, fs), "## <a id=\"1-0-0\"></a>1.0.0\n* Shipped\n")
}

// Some older changes cite the issue they closed rather than a pull request, and those
// link the same way — the number is what the reader wants either way.
func TestBuild_IssueURLLinksLikeAPullRequest(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"Fix retries\"\npr_url: \"https://github.com/stripe/stripe-go/issues/7\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"b.change.md": "---\ntitle: \"Add widgets\"\npr_url: \"https://github.com/stripe/stripe-go/pull/41/\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Contains(t, got, "* [#7](https://github.com/stripe/stripe-go/issues/7) Fix retries\n")
	// A trailing slash is still a well-formed link, not a URL we could not read a number from.
	assert.Contains(t, got, "* [#41](https://github.com/stripe/stripe-go/pull/41/) Add widgets\n")
}

// Two versions that differ only in where their dots fall get distinct anchors. GitHub
// drops dots when it derives one from a heading, so "11.0.0" and "1.10.0" would both
// reduce to "1100" — a collision four of the seven SDK changelogs contain.
func TestBuild_VersionAnchorsKeepDotsApart(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"11.0.0","released_on":"2024-03-01"},
		{"version":"1.10.0","released_on":"2024-01-15"}
	]}`, nil)

	got := build(t, fs)
	assert.Contains(t, got, `## <a id="11-0-0"></a>11.0.0 - 2024-03-01`)
	assert.Contains(t, got, `## <a id="1-10-0"></a>1.10.0 - 2024-01-15`)
}

// A version with only an intro still renders that prose.
func TestBuild_RendersIntroWithoutChanges(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{})
	writeIntro(t, fs, "1.0.0", "First paragraph.\n\nSecond paragraph.")

	assert.Contains(t, build(t, fs), "## <a id=\"1-0-0\"></a>1.0.0 - 2024-01-15\nFirst paragraph.\n\nSecond paragraph.\n")
}

// An intro is found only by its filename, so the one that matters is the one named
// after this release.
func TestBuild_IntroIsMatchedByVersion(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"2.0.0","released_on":"2024-03-01"},
		{"version":"1.0.0","released_on":"2024-01-15"}
	]}`, map[string]string{})
	writeIntro(t, fs, "1.0.0", "Only on the first release.")

	got := build(t, fs)
	assert.Contains(t, got, "## <a id=\"1-0-0\"></a>1.0.0 - 2024-01-15\nOnly on the first release.\n")
	assert.Contains(t, got, "## <a id=\"2-0-0\"></a>2.0.0 - 2024-03-01\n\n")
}

// An intro can be written before the release it belongs to. Build walks the
// releases file and asks for each release's intro, so one for a version that has
// not been cut yet is simply never asked for — and then appears on its own once
// that release is recorded.
func TestBuild_IgnoresAnIntroForAnUnreleasedVersion(t *testing.T) {
	versionsJSON := `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`
	changes := map[string]string{
		"a.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n",
	}

	fs := buildFixture(t, versionsJSON, changes)
	writeIntro(t, fs, "2.0.0", "Drafted for the next major.")

	assert.NotContains(t, build(t, fs), "Drafted for the next major.")

	// Once 2.0.0 is a release, the same file renders with no other change.
	fs = buildFixture(t, `{"releases":[
		{"version":"2.0.0","released_on":"2024-03-01"},
		{"version":"1.0.0","released_on":"2024-01-15"}
	]}`, changes)
	writeIntro(t, fs, "2.0.0", "Drafted for the next major.")

	assert.Contains(t, build(t, fs), "## <a id=\"2-0-0\"></a>2.0.0 - 2024-03-01\nDrafted for the next major.\n")
}

// Intros are optional, and most releases have none.
func TestBuild_NoIntrosAtAll(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{"a.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n"})

	// No .hark/intros directory exists at all.
	assert.Contains(t, build(t, fs), "## <a id=\"1-0-0\"></a>1.0.0 - 2024-01-15\n* Shipped\n")
}

// A trailing newline in the file must not become a blank line in the changelog.
func TestBuild_IntroTrailingNewlineIsTrimmed(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{"a.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n"})
	writeIntro(t, fs, "1.0.0", "Some prose.\n")

	assert.Contains(t, build(t, fs), "## <a id=\"1-0-0\"></a>1.0.0 - 2024-01-15\nSome prose.\n\n* Shipped\n")
}

// Leading whitespace goes too. Four spaces would otherwise make the first paragraph an
// indented code block.
func TestBuild_IntroLeadingWhitespaceIsTrimmed(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{"a.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n"})
	writeIntro(t, fs, "1.0.0", "\n    Some prose.")

	assert.Contains(t, build(t, fs), "## <a id=\"1-0-0\"></a>1.0.0 - 2024-01-15\nSome prose.\n\n* Shipped\n")
}

func TestBuild_SectionOrderIsCanonical(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"r\"\nsection: \"⚠️ Removed\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"b.change.md": "---\ntitle: \"a\"\nsection: \"Added\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"c.change.md": "---\ntitle: \"d\"\nsection: \"Deprecated\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"d.change.md": "---\ntitle: \"plain\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	// The unsectioned change leads, then sections in sectionOrder.
	for _, pair := range [][2]string{
		{"* plain", "### Added"},
		{"### Added", "### Deprecated"},
		{"### Deprecated", "### ⚠️ Removed"},
	} {
		assert.Less(t, strings.Index(got, pair[0]), strings.Index(got, pair[1]),
			"%q should precede %q", pair[0], pair[1])
	}
}

func TestBuild_UnknownSectionsSortLast(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"x\"\nsection: \"Zebras\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"b.change.md": "---\ntitle: \"y\"\nsection: \"⚠️ Removed\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Less(t, strings.Index(got, "### ⚠️ Removed"), strings.Index(got, "### Zebras"))
}

// Sections differing only in whitespace are one section. python's changelog has a
// double-spaced variant of a section title used elsewhere with one space.
func TestBuild_SectionMatchIsWhitespaceInsensitive(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"one\"\nsection: \"Added\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"b.change.md": "---\ntitle: \"two\"\nsection: \"Added  \"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Equal(t, 1, strings.Count(got, "### Added"))
	assert.Contains(t, got, "* one")
	assert.Contains(t, got, "* two")
}

func TestBuild_PRURLWithoutNumberIsKept(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"Something\"\npr_url: \"https://example.com/discussion\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	assert.Contains(t, build(t, fs), "* Something (https://example.com/discussion)\n")
}

// Bodies keep their internal structure; only the outer indent is added.
func TestBuild_BodyIndentation(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"Nested\"\nreleased_in_version: \"1.0.0\"\n---\n\n" +
				"- outer\n  - inner\n\n- after a blank\n",
		})

	assert.Contains(t, build(t, fs), "* Nested\n  - outer\n    - inner\n\n  - after a blank\n")
}

func TestBuild_UnknownVersionListsEveryOffender(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`,
		map[string]string{
			"a.change.md": "---\ntitle: \"a\"\nreleased_in_version: \"9.9.9\"\n---\n",
			"b.change.md": "---\ntitle: \"b\"\nreleased_in_version: \"8.8.8\"\n---\n",
		})

	var out bytes.Buffer
	err := Build(context.Background(), Options{Fs: fs, Out: &out})
	require.Error(t, err)

	// Both problems are reported, not just the first one encountered.
	assert.Contains(t, err.Error(), "9.9.9")
	assert.Contains(t, err.Error(), "8.8.8")
}

func TestBuild_RequiresVersionsFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, changesFixtureDir+"/a.change.md", []byte("---\ntitle: \"a\"\n---\n"), 0o644))

	var out bytes.Buffer
	err := Build(context.Background(), Options{Fs: fs, Out: &out})
	require.Error(t, err)
}

// The h1 is fixed rather than configurable: every one of these files is a changelog,
// and nothing was ever going to call it anything else.
func TestBuild_HeadingIsAlwaysChangelog(t *testing.T) {
	got := build(t, buildFixture(t, `{"releases":[]}`, map[string]string{}))

	assert.True(t, strings.HasPrefix(got, generatedNotice+"\n\n# Changelog\n"),
		"got:\n%s", got)
}

// The notice leads the file, above the h1, and is an HTML comment so it is invisible
// wherever the markdown is rendered — it is for whoever opened the file to edit it.
func TestBuild_LeadsWithTheGeneratedFileNotice(t *testing.T) {
	got := build(t, buildFixture(t, `{"releases":[]}`, nil))

	assert.True(t, strings.HasPrefix(got, "<!--\n"), "the notice has to be the first thing in the file")
	assert.Contains(t, got, "THIS IS A GENERATED FILE.")
	assert.Contains(t, got, "run `hark build`")
	assert.Contains(t, got, generatedNotice+"\n\n# Changelog\n")
}

func TestBuild_IsDeterministic(t *testing.T) {
	versionsJSON := `{"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`
	changefiles := map[string]string{
		"2024-01-10-a-one.change.md":   "---\ntitle: \"one\"\nreleased_in_version: \"1.0.0\"\n---\n",
		"2024-01-11-b-two.change.md":   "---\ntitle: \"two\"\nreleased_in_version: \"1.0.0\"\n---\n",
		"2024-01-12-c-three.change.md": "---\ntitle: \"three\"\nreleased_in_version: \"1.0.0\"\n---\n",
	}

	first := build(t, buildFixture(t, versionsJSON, changefiles))
	for range 5 {
		assert.Equal(t, first, build(t, buildFixture(t, versionsJSON, changefiles)))
	}

	// Oldest first, since changefile names lead with the change's date.
	assert.Less(t, strings.Index(first, "* one"), strings.Index(first, "* two"))
	assert.Less(t, strings.Index(first, "* two"), strings.Index(first, "* three"))
}

// pinnedFixture is two releases whose pinned API versions are given, so a test can
// say what the renderer should make of the pair.
func pinnedFixture(t *testing.T, newer, older string) afero.Fs {
	t.Helper()

	return buildFixture(t, `{"releases":[
		{"version":"2.0.0","released_on":"2024-03-01","pinned_api_version":"`+newer+`"},
		{"version":"1.0.0","released_on":"2024-01-15","pinned_api_version":"`+older+`"}
	]}`, map[string]string{
		"2024-02-28_alice_add-widgets.change.md": "---\n" +
			"title: \"Add support for widgets\"\nreleased_in_version: \"2.0.0\"\n---\n",
	})
}

const pinnedNotice = "This release changes the pinned API version to"

// The sentence is generated rather than written by hand so that it cannot disagree
// with the pinned_api_version it describes.
func TestBuild_GeneratesThePinnedAPINotice(t *testing.T) {
	got := build(t, pinnedFixture(t, "2024-02-01.acacia", "2023-10-16"))

	assert.Equal(t, generatedNotice+`

# Changelog

## <a id="2-0-0"></a>2.0.0 - 2024-03-01
This release changes the pinned API version to `+"`2024-02-01.acacia`"+`.

* Add support for widgets

## <a id="1-0-0"></a>1.0.0 - 2024-01-15
`, got)
}

// It says "changes", so a release pinning the same version as the one before it
// announces nothing.
func TestBuild_NoNoticeWhenThePinnedVersionIsUnchanged(t *testing.T) {
	assert.NotContains(t, build(t, pinnedFixture(t, "2024-02-01.acacia", "2024-02-01.acacia")), pinnedNotice)
}

func TestBuild_NoNoticeWhenTheReleasePinsNothing(t *testing.T) {
	assert.NotContains(t, build(t, pinnedFixture(t, "", "2023-10-16")), pinnedNotice)
}

// A backport is compared against the release it succeeded, not the one above it in the
// file. These are stripe-ruby's real releases: 13.5.1 was cut on the 13.x line after
// 15.0.0 had shipped, and it pins exactly what 13.5.0 pinned, so it announces nothing.
// Against the previous release by date it looked like a change back to acacia.
func TestBuild_BackportComparesAgainstItsOwnLine(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"15.1.0","released_on":"2025-04-30","pinned_api_version":"2025-04-30.basil"},
		{"version":"13.5.1","released_on":"2025-04-21","pinned_api_version":"2025-02-24.acacia"},
		{"version":"15.0.0","released_on":"2025-04-09","pinned_api_version":"2025-03-31.basil"},
		{"version":"13.5.0","released_on":"2025-02-24","pinned_api_version":"2025-02-24.acacia"}
	]}`, nil)

	got := build(t, fs)
	assert.Contains(t, got, "## <a id=\"13-5-1\"></a>13.5.1 - 2025-04-21\n\n")
	assert.NotContains(t, got, pinnedNotice+" `2025-02-24.acacia`")
	// The releases either side of it still announce their own changes.
	assert.Contains(t, got, pinnedNotice+" `2025-04-30.basil`")
	assert.Contains(t, got, pinnedNotice+" `2025-03-31.basil`")
}

// A blank previous value means the release before recorded no API version at all —
// it predates codegen, or is an early prerelease from before the SDKs pinned — so
// "changed to X" would assert something the data does not support.
func TestBuild_NoNoticeWhenThePreviousReleasePinnedNothing(t *testing.T) {
	assert.NotContains(t, build(t, pinnedFixture(t, "2024-02-01.acacia", "")), pinnedNotice)
}

// The oldest release has nothing to have changed from.
func TestBuild_NoNoticeForTheOldestRelease(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"1.0.0","released_on":"2024-01-15","pinned_api_version":"2023-10-16"}
	]}`, nil)

	assert.NotContains(t, build(t, fs), pinnedNotice)
}

// Unreleased changes pin nothing, and the group has no version to read.
func TestBuild_NoNoticeForUnreleasedChanges(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"1.0.0","released_on":"2024-01-15","pinned_api_version":"2023-10-16"}
	]}`, map[string]string{
		"2024-02-01_alice_pending.change.md": "---\ntitle: \"Not out yet\"\n---\n",
	})

	assert.NotContains(t, build(t, fs), pinnedNotice)
}

// The notice is generated and the intro is prose someone wrote; both belong above
// the bullets, as separate paragraphs, with the notice first.
func TestBuild_NoticeAndIntroBothRender(t *testing.T) {
	fs := pinnedFixture(t, "2024-02-01.acacia", "2023-10-16")
	writeIntro(t, fs, "2.0.0", "### Breaking Changes\n\nThe gizmo resource is gone.")

	assert.Contains(t, build(t, fs), "## <a id=\"2-0-0\"></a>2.0.0 - 2024-03-01\n"+
		"This release changes the pinned API version to `2024-02-01.acacia`.\n"+
		"\n"+
		"### Breaking Changes\n"+
		"\n"+
		"The gizmo resource is gone.\n"+
		"\n"+
		"* Add support for widgets\n")
}

// A release with a notice and no changes still separates cleanly from the next
// heading.
func TestBuild_NoticeWithNoChanges(t *testing.T) {
	fs := buildFixture(t, `{"releases":[
		{"version":"2.0.0","released_on":"2024-03-01","pinned_api_version":"2024-02-01.acacia"},
		{"version":"1.0.0","released_on":"2024-01-15","pinned_api_version":"2023-10-16"}
	]}`, nil)

	assert.Equal(t, generatedNotice+`

# Changelog

## <a id="2-0-0"></a>2.0.0 - 2024-03-01
This release changes the pinned API version to `+"`2024-02-01.acacia`"+`.

## <a id="1-0-0"></a>1.0.0 - 2024-01-15
`, build(t, fs))
}

// A change that only exists because the API spec moved goes last, whatever its
// filename. It is generated rather than written, and it is usually one bullet with a
// very long list of field additions under it.
func TestBuild_StripeAPIChangesSortLast(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-03-01"}]}`,
		map[string]string{
			// The newest by filename, so without the rule it would lead.
			"2024-02-28_bot_update-generated-code.change.md": "---\ntitle: \"Update generated code\"\n" +
				"is_stripe_api_change: true\nreleased_in_version: \"1.0.0\"\n---\n",
			"2024-02-27_alice_fix-retries.change.md": "---\ntitle: \"Fix retries\"\n" +
				"released_in_version: \"1.0.0\"\n---\n",
			"2024-02-26_bob_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"released_in_version: \"1.0.0\"\n---\n",
		})

	assert.Contains(t, build(t, fs), "## <a id=\"1-0-0\"></a>1.0.0 - 2024-03-01\n"+
		"* Add widgets\n"+
		"* Fix retries\n"+
		"* Update generated code\n")
}

// The rule holds inside a section too, since blocks() buckets the release's order
// rather than sorting again.
func TestBuild_StripeAPIChangesSortLastWithinASection(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-03-01"}]}`,
		map[string]string{
			"2024-02-28_bot_generated.change.md": "---\ntitle: \"Generated\"\n" +
				"is_stripe_api_change: true\nsection: \"Added\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"2024-02-27_alice_by-hand.change.md": "---\ntitle: \"By hand\"\n" +
				"section: \"Added\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	assert.Contains(t, build(t, fs), "### Added\n* By hand\n* Generated\n")
}

// Names differing only in case, a trailing colon, or whitespace are one section, and
// the heading renders as the first one was written.
func TestBuild_SectionNamesAreMatchedLoosely(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-03-01"}]}`,
		map[string]string{
			// The oldest sorts first, so its spelling is the one the heading uses.
			"2024-02-26_a_first.change.md": "---\ntitle: \"First\"\n" +
				"section: \"Breaking Changes\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"2024-02-27_b_colon.change.md": "---\ntitle: \"Colon\"\n" +
				"section: \"breaking changes:\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"2024-02-28_c_spaces.change.md": "---\ntitle: \"Spaces\"\n" +
				"section: \"Breaking  Changes\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Equal(t, 1, strings.Count(got, "###"), "the three spellings are one section")
	assert.Contains(t, got, "### Breaking Changes\n* First\n* Colon\n* Spaces\n")
}

// A breaking-changes section has to outrank Added, which is what listing its plural
// spelling in sectionOrder buys. Left unlisted it would sort after every recognised
// section, putting breaking changes at the bottom of the release.
func TestBuild_BreakingChangesOutrankAdded(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-03-01"}]}`,
		map[string]string{
			// Named so that filename order alone would put Added first.
			"2024-02-28_a_added.change.md": "---\ntitle: \"An addition\"\n" +
				"section: \"Added\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"2024-02-27_b_breaking.change.md": "---\ntitle: \"A break\"\n" +
				"section: \"Breaking changes\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Less(t, strings.Index(got, "### Breaking changes"), strings.Index(got, "### Added"))
}

// An unrecognised section still sorts after every recognised one, and unrecognised
// ones sort among themselves by name.
func TestBuild_UnlistedSectionsSortLastByName(t *testing.T) {
	fs := buildFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2024-03-01"}]}`,
		map[string]string{
			"2024-02-28_a_zebra.change.md": "---\ntitle: \"Z\"\n" +
				"section: \"Zebra facts\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"2024-02-27_b_apple.change.md": "---\ntitle: \"A\"\n" +
				"section: \"Apple facts\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"2024-02-26_c_removed.change.md": "---\ntitle: \"R\"\n" +
				"section: \"⚠️ Removed\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	got := build(t, fs)
	assert.Less(t, strings.Index(got, "### ⚠️ Removed"), strings.Index(got, "### Apple facts"))
	assert.Less(t, strings.Index(got, "### Apple facts"), strings.Index(got, "### Zebra facts"))
}

// A prerelease changelog lists only its own channel's releases, so on its own it
// reads as though the SDK had shipped nothing else. The note says where the rest is,
// linked — the repository is built from the language the releases file records.
func TestBuild_BetaChangelogLinksToGA(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"python","channel":"beta"},"releases":[{"version":"22.7.0-beta.1","released_on":"2026-09-08"}]}`,
		map[string]string{
			"2026-09-07_a_widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"pr_url: \"https://github.com/stripe/stripe-python/pull/42\"\n" +
				"released_in_version: \"22.7.0-beta.1\"\n---\n",
		})

	assert.Contains(t, build(t, fs), "# Changelog\n"+
		"\n"+
		"> This changelog only covers the **public preview** releases. Each release builds on "+
		"the most recent GA release; see those notes in "+
		"[the GA changelog](https://github.com/stripe/stripe-python/blob/master/CHANGELOG.md).\n"+
		"\n"+
		"## <a id=\"22-7-0-beta-1\"></a>22.7.0-beta.1 - 2026-09-08\n")
}

// Private preview points at GA and only GA: master merges into beta and into
// private-preview, and those two never merge into each other, so a private preview
// release does not build on public preview.
func TestBuild_PrivatePreviewChangelogLinksToGAOnly(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"go","channel":"private-preview"},"releases":[{"version":"22.7.0-alpha.2","released_on":"2026-09-08"}]}`,
		map[string]string{
			"2026-09-07_a_widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"pr_url: \"https://github.com/stripe/stripe-go/pull/42\"\n" +
				"released_in_version: \"22.7.0-alpha.2\"\n---\n",
		})

	got := build(t, fs)
	assert.Contains(t, got, "**private preview**")
	assert.Contains(t, got,
		"[the GA changelog](https://github.com/stripe/stripe-go/blob/master/CHANGELOG.md).")
	assert.NotContains(t, got, "public preview", "private preview does not build on public preview")
	assert.NotContains(t, got, "blob/beta/")
}

// With no language recorded there is no repository to build a URL from, so the note
// still says its piece, just without linking. `hark validate` reports the missing
// language separately; build renders what it can rather than failing.
func TestBuild_ChannelNoteWithoutALinkableRepo(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"channel":"beta"},"releases":[{"version":"22.7.0-beta.1","released_on":"2026-09-08"}]}`,
		map[string]string{
			"2026-09-07_a_widgets.change.md": "---\ntitle: \"Add widgets\"\n" +
				"released_in_version: \"22.7.0-beta.1\"\n---\n",
		})

	got := build(t, fs)
	assert.Contains(t, got, "see those notes in the GA changelog.\n")
	assert.NotContains(t, got, "https://")
}

// GA's changelog is the whole story on its own, so it says nothing.
func TestBuild_GAChangelogHasNoChannelNote(t *testing.T) {
	fs := buildFixture(t, `{"metadata":{"language":"go","channel":"ga"},"releases":[{"version":"22.7.0","released_on":"2026-09-08"}]}`, nil)

	got := build(t, fs)
	assert.NotContains(t, got, "preview")
	assert.Contains(t, got, "# Changelog\n\n## <a id=\"22-7-0\"></a>22.7.0 - 2026-09-08\n")
}

// A repo mid-setup has no releases to read a channel from, and must not be guessed at.
func TestBuild_NoChannelNoteWithoutReleases(t *testing.T) {
	assert.NotContains(t, build(t, buildFixture(t, `{"releases":[]}`, nil)), "preview")
}
