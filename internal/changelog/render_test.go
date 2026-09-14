package changelog

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

// aChange is a changefile with only the fields a render test cares about. The path is
// what changes within a release are ordered by, so it is always given.
func aChange(path, title string) *changefile.Changefile {
	return &changefile.Changefile{Title: title, SourcePath: path}
}

// renderBlock is one release group's markdown.
func renderBlock(t *testing.T, g releaseGroup) string {
	t.Helper()

	var b bytes.Buffer
	require.NoError(t, renderReleaseBlock(&b, g))
	return b.String()
}

// renderOneChange is a single bullet's markdown.
func renderOneChange(t *testing.T, c *changefile.Changefile) string {
	t.Helper()

	var b bytes.Buffer
	require.NoError(t, renderChange(&b, c))
	return b.String()
}

func TestVersionAnchor(t *testing.T) {
	for version, want := range map[string]string{
		"1.2.3":        "1-2-3",
		"11.0.0":       "11-0-0",
		"1.10.0":       "1-10-0",
		"1.2.3-beta.1": "1-2-3-beta-1",
		"1.0.0b1":      "1-0-0b1",
	} {
		assert.Equal(t, want, versionAnchor(version))
	}
}

func TestCanonicalSection(t *testing.T) {
	for section, want := range map[string]string{
		"Added":              "added",
		"added":              "added",
		"Changes:":           "changes",
		"  Deprecated  ":     "deprecated",
		"Breaking\tchanges":  "breaking changes",
		"Breaking\n changes": "breaking changes",
		"⚠️  Removed":        "⚠️ removed",
		"":                   "",
	} {
		assert.Equal(t, want, canonicalSection(section), "canonicalSection(%q)", section)
	}
}

func TestSectionRank(t *testing.T) {
	// Every listed section keeps the rank its position gives it, whatever spelling it
	// arrives in.
	for i, section := range sectionOrder {
		assert.Equal(t, i, sectionRank(section), section)
		assert.Equal(t, i, sectionRank(strings.ToUpper(section)+":  "), section)
	}

	// Anything unlisted sorts after all of them.
	assert.Equal(t, len(sectionOrder), sectionRank("Zebras"))
	assert.Equal(t, len(sectionOrder), sectionRank(""))
}

// Two entries in sectionOrder that canonicalize the same way would collapse into one
// rank, silently giving one of them the other's position.
func TestSectionOrderHasNoDuplicates(t *testing.T) {
	assert.Len(t, sectionRanks, len(sectionOrder))
}

func TestMarkdownPrLink(t *testing.T) {
	tests := []struct {
		link string
		want string
	}{
		{"https://github.com/stripe/stripe-go/pull/123", "[#123](https://github.com/stripe/stripe-go/pull/123)"},
		{"https://github.com/stripe/stripe-go/pull/123/", "[#123](https://github.com/stripe/stripe-go/pull/123/)"},
		{"https://github.com/stripe/stripe-go/issues/7", "[#7](https://github.com/stripe/stripe-go/issues/7)"},
	}
	for _, tt := range tests {
		got, ok := markdownPrLink(tt.link)
		assert.True(t, ok, tt.link)
		assert.Equal(t, tt.want, got)
	}

	// Anything we cannot pull a number out of is reported as such, so the caller can
	// fall back to printing the URL.
	for _, link := range []string{
		"",
		"https://example.com/discussion",
		"https://github.com/stripe/stripe-go/pull/",
		"https://github.com/stripe/stripe-go/pull/abc",
		"https://github.com/stripe/stripe-go/pull/123/files",
	} {
		got, ok := markdownPrLink(link)
		assert.False(t, ok, link)
		assert.Empty(t, got)
	}
}

func TestReleaseGroupHeading(t *testing.T) {
	// Changes that have not shipped are grouped under a heading of their own.
	assert.Equal(t, unreleasedHeading, releaseGroup{}.heading())

	// A recorded release with no date yet is headed by just its version.
	assert.Equal(t, "1.0.0", releaseGroup{Release: &releases.Release{Version: "1.0.0"}}.heading())

	assert.Equal(t, "1.0.0 - 2026-01-15",
		releaseGroup{Release: &releases.Release{Version: "1.0.0", ReleasedOn: "2026-01-15"}}.heading())
}

func TestReleaseGroupIntro(t *testing.T) {
	assert.Empty(t, releaseGroup{}.intro())
	assert.Empty(t, releaseGroup{Intro: "\n  \n"}.intro())
	assert.Equal(t, "Prose.", releaseGroup{Intro: "\n\nProse.\n\n"}.intro())
}

func TestPinnedAPINotice(t *testing.T) {
	group := func(current, previous string) releaseGroup {
		return releaseGroup{
			Release:              &releases.Release{Version: "1.0.0", PinnedAPIVersion: current},
			PrevPinnedAPIVersion: previous,
		}
	}

	assert.Equal(t, "This release changes the pinned API version to `2026-01-01`.",
		group("2026-01-01", "2025-01-01").pinnedAPINotice())

	// Unreleased changes pin nothing, and there is nothing to announce unless both
	// sides are known and they differ.
	assert.Empty(t, releaseGroup{}.pinnedAPINotice())
	assert.Empty(t, group("", "2025-01-01").pinnedAPINotice())
	assert.Empty(t, group("2026-01-01", "").pinnedAPINotice())
	assert.Empty(t, group("2026-01-01", "2026-01-01").pinnedAPINotice())
}

func TestReleaseGroupPreamble(t *testing.T) {
	release := &releases.Release{Version: "1.0.0", PinnedAPIVersion: "2026-01-01"}

	notice := "This release changes the pinned API version to `2026-01-01`."

	// Neither, either, or both — and when both, the notice leads and a blank line
	// separates them.
	assert.Empty(t, releaseGroup{Release: &releases.Release{Version: "1.0.0"}}.preamble())
	assert.Equal(t, notice,
		releaseGroup{Release: release, PrevPinnedAPIVersion: "2025-01-01"}.preamble())
	assert.Equal(t, "Prose.", releaseGroup{Intro: "Prose.\n"}.preamble())
	assert.Equal(t, notice+"\n\nProse.",
		releaseGroup{Release: release, PrevPinnedAPIVersion: "2025-01-01", Intro: "Prose.\n"}.preamble())
}

func TestSortChanges(t *testing.T) {
	spec := &changefile.Changefile{Title: "spec", SourcePath: "a.change.md", IsStripeAPIChange: true}
	changes := []*changefile.Changefile{
		spec,
		aChange("c.change.md", "c"),
		aChange("b.change.md", "b"),
	}

	sorted := sortChanges(changes)

	// Ordered by path, except that a spec-driven change goes last however it is named.
	assert.Equal(t, []string{"b.change.md", "c.change.md", "a.change.md"}, changePaths(sorted))

	// The caller's slice is left as it was.
	assert.Equal(t, []string{"a.change.md", "c.change.md", "b.change.md"}, changePaths(changes))
}

// changePaths is the source path of each change, which is what sortChanges orders by.
func changePaths(changes []*changefile.Changefile) []string {
	paths := make([]string, len(changes))
	for i, c := range changes {
		paths[i] = c.SourcePath
	}
	return paths
}

func TestReleaseGroupSections(t *testing.T) {
	unsectioned := aChange("a.change.md", "no section")
	added := &changefile.Changefile{Title: "added", SourcePath: "b.change.md", Section: "Added"}
	// The same section, spelled differently: these belong together under one header.
	alsoAdded := &changefile.Changefile{Title: "also added", SourcePath: "c.change.md", Section: "added:"}
	breaking := &changefile.Changefile{Title: "breaking", SourcePath: "d.change.md", Section: "Breaking changes"}
	unlisted := &changefile.Changefile{Title: "unlisted", SourcePath: "e.change.md", Section: "Zebras"}

	g := releaseGroup{Changes: []*changefile.Changefile{added, unlisted, unsectioned, breaking, alsoAdded}}
	sections := g.sections()

	// The unsectioned block leads, then listed sections by rank, then unlisted ones.
	require.Len(t, sections, 4)
	assert.Equal(t, []string{"", "Breaking changes", "Added", "Zebras"},
		[]string{sections[0].Header, sections[1].Header, sections[2].Header, sections[3].Header})

	// A section is headed as the first change in it spelled it, not as it was compared.
	assert.Equal(t, []string{"b.change.md", "c.change.md"}, changePaths(sections[2].Changes))
	assert.Equal(t, []string{"a.change.md"}, changePaths(sections[0].Changes))

	assert.Empty(t, releaseGroup{}.sections())
}

func TestGroupChanges(t *testing.T) {
	releaseFile := &releases.File{Releases: []releases.Release{
		{Version: "1.1.0", ReleasedOn: "2026-02-01", PinnedAPIVersion: "2026-02-01.acacia"},
		{Version: "1.0.0", ReleasedOn: "2026-01-15", PinnedAPIVersion: "2026-01-01.acacia"},
	}}

	pending := aChange("a.change.md", "not shipped yet")
	shipped := &changefile.Changefile{Title: "shipped", SourcePath: "b.change.md", ReleasedInVersion: "1.1.0"}

	groups, err := groupChanges([]*changefile.Changefile{shipped, pending}, releaseFile)
	require.NoError(t, err)

	// Unreleased leads, then one group per release — including 1.0.0, which shipped
	// nothing but is still part of the record.
	require.Len(t, groups, 3)
	assert.Nil(t, groups[0].Release)
	assert.Equal(t, []string{"a.change.md"}, changePaths(groups[0].Changes))

	assert.Equal(t, "1.1.0", groups[1].Release.Version)
	assert.Equal(t, []string{"b.change.md"}, changePaths(groups[1].Changes))
	// The release this one succeeded by version, which is where the pinned notice
	// compares against.
	assert.Equal(t, "2026-01-01.acacia", groups[1].PrevPinnedAPIVersion)

	assert.Equal(t, "1.0.0", groups[2].Release.Version)
	assert.Empty(t, groups[2].Changes)
	assert.Empty(t, groups[2].PrevPinnedAPIVersion, "the oldest release succeeded nothing")
}

// With nothing pending there is no unreleased group at all, rather than an empty one.
func TestGroupChangesOmitsAnEmptyUnreleasedGroup(t *testing.T) {
	releaseFile := &releases.File{Releases: []releases.Release{{Version: "1.0.0", ReleasedOn: "2026-01-15"}}}
	shipped := &changefile.Changefile{Title: "shipped", SourcePath: "a.change.md", ReleasedInVersion: "1.0.0"}

	groups, err := groupChanges([]*changefile.Changefile{shipped}, releaseFile)
	require.NoError(t, err)

	require.Len(t, groups, 1)
	assert.NotNil(t, groups[0].Release)
}

// A change claiming a release nobody recorded cannot be placed, and every offender is
// named at once so the fix is one pass.
func TestGroupChangesRejectsUnknownVersions(t *testing.T) {
	releaseFile := &releases.File{Releases: []releases.Release{{Version: "1.0.0", ReleasedOn: "2026-01-15"}}}

	_, err := groupChanges([]*changefile.Changefile{
		{Title: "b", SourcePath: "b.change.md", ReleasedInVersion: "9.9.9"},
		{Title: "a", SourcePath: "a.change.md", ReleasedInVersion: "8.8.8"},
	}, releaseFile)

	require.Error(t, err)
	assert.Equal(t, "unknown versions:\n"+
		"  a.change.md: released_in_version \"8.8.8\" is not in the releases file\n"+
		"  b.change.md: released_in_version \"9.9.9\" is not in the releases file", err.Error())
}

func TestSdkRepo(t *testing.T) {
	assert.Equal(t, "stripe/stripe-go", sdkRepo("go"))
	assert.Equal(t, "stripe/stripe-dotnet", sdkRepo("dotnet"))

	// A language we don't publish an SDK for gets no repo rather than a guessed one.
	for _, language := range []string{"", "rust", "Go", "stripe-go"} {
		assert.Empty(t, sdkRepo(language), language)
	}
}

func TestChangelogRef(t *testing.T) {
	assert.Equal(t, "[the GA changelog](https://github.com/stripe/stripe-python/blob/master/CHANGELOG.md)",
		changelogRef("the GA changelog", "python"))

	// Without a repo to point at, the label is left as prose rather than linked nowhere.
	assert.Equal(t, "the GA changelog", changelogRef("the GA changelog", ""))
}

func TestChannelNotice(t *testing.T) {
	beta := channelNotice(releases.ChannelBeta, "python")
	assert.Contains(t, beta, "**public preview**")
	assert.Contains(t, beta, "[the GA changelog](https://github.com/stripe/stripe-python/blob/master/CHANGELOG.md)")
	assert.True(t, strings.HasPrefix(beta, "> "), "the notice is a blockquote")

	// Private preview builds on GA too, not on public preview.
	private := channelNotice(releases.ChannelPrivatePreview, "go")
	assert.Contains(t, private, "**private preview**")
	assert.NotContains(t, private, "public preview")

	// GA is the changelog the others point at, so it stands alone. An unknown or missing
	// channel says nothing rather than guessing.
	for _, channel := range []string{releases.ChannelGA, "", "nightly"} {
		assert.Empty(t, channelNotice(channel, "go"), channel)
	}
}

func TestIndentBody(t *testing.T) {
	assert.Empty(t, indentBody(""))
	assert.Empty(t, indentBody("\n\n"))
	assert.Equal(t, "  one line", indentBody("one line"))

	// Surrounding blank lines go, and the body's own structure stays: nested bullets keep
	// their relative depth and a blank line separating them stays blank (not whitespace).
	assert.Equal(t, "  - outer\n    - inner\n\n  - after a blank",
		indentBody("\n- outer\n  - inner\n   \n- after a blank\n"))

	// A fenced block survives, which is why the indent is uniform.
	assert.Equal(t, "  ```go\n  x := 1\n  ```", indentBody("```go\nx := 1\n```"))
}

func TestRenderChange(t *testing.T) {
	assert.Equal(t, "* Add widgets\n", renderOneChange(t, aChange("a.change.md", "  Add widgets  ")))

	assert.Equal(t, "* ⚠️ Remove widgets\n", renderOneChange(t, &changefile.Changefile{
		Title: "Remove widgets", SemverLevel: changefile.SemverLevelMajor,
	}))

	assert.Equal(t, "* [#12](https://github.com/stripe/stripe-go/pull/12) Add widgets\n",
		renderOneChange(t, &changefile.Changefile{
			Title: "Add widgets", PRUrl: "https://github.com/stripe/stripe-go/pull/12",
		}))

	// A link we cannot read a number out of still belongs in the output, just not as a
	// numbered link.
	assert.Equal(t, "* Add widgets (https://example.com/discussion)\n",
		renderOneChange(t, &changefile.Changefile{
			Title: "Add widgets", PRUrl: "https://example.com/discussion",
		}))

	// The body is nested under the bullet it belongs to.
	assert.Equal(t, "* Add widgets\n  Some detail.\n",
		renderOneChange(t, &changefile.Changefile{Title: "Add widgets", Body: "Some detail.\n"}))

	// Everything at once, in order: marker, link, title, body.
	assert.Equal(t, "* ⚠️ [#12](https://github.com/stripe/stripe-go/pull/12) Remove widgets\n  Some detail.\n",
		renderOneChange(t, &changefile.Changefile{
			Title:       "Remove widgets",
			PRUrl:       "https://github.com/stripe/stripe-go/pull/12",
			SemverLevel: changefile.SemverLevelMajor,
			Body:        "Some detail.\n",
		}))
}

func TestRenderReleaseBlock(t *testing.T) {
	// An anchor we can deeplink to, ahead of a heading whose text includes the date.
	assert.Equal(t, "## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\n* Add widgets\n",
		renderBlock(t, releaseGroup{
			Release: &releases.Release{Version: "1.0.0", ReleasedOn: "2026-01-15"},
			Changes: []*changefile.Changefile{aChange("a.change.md", "Add widgets")},
		}))

	// Unreleased changes have no release to anchor to.
	assert.Equal(t, "## Unreleased\n* Add widgets\n",
		renderBlock(t, releaseGroup{Changes: []*changefile.Changefile{aChange("a.change.md", "Add widgets")}}))

	// A release that shipped nothing is still its own heading.
	assert.Equal(t, "## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\n",
		renderBlock(t, releaseGroup{Release: &releases.Release{Version: "1.0.0", ReleasedOn: "2026-01-15"}}))
}

// Prose and bullets need a blank line between them, whether the bullets are under a
// section header or not.
func TestRenderReleaseBlockSeparatesProseFromBullets(t *testing.T) {
	release := &releases.Release{Version: "1.0.0", ReleasedOn: "2026-01-15"}

	assert.Equal(t, "## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\nProse.\n\n* Add widgets\n",
		renderBlock(t, releaseGroup{
			Release: release,
			Intro:   "Prose.",
			Changes: []*changefile.Changefile{aChange("a.change.md", "Add widgets")},
		}))

	assert.Equal(t, "## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\nProse.\n\n### Added\n* Add widgets\n",
		renderBlock(t, releaseGroup{
			Release: release,
			Intro:   "Prose.",
			Changes: []*changefile.Changefile{
				{Title: "Add widgets", SourcePath: "a.change.md", Section: "Added"},
			},
		}))

	// Without prose, a section header still opens with a blank line, and unsectioned
	// bullets follow the heading directly.
	assert.Equal(t, "## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\n* Add widgets\n\n### Added\n* Add more\n",
		renderBlock(t, releaseGroup{
			Release: release,
			Changes: []*changefile.Changefile{
				aChange("a.change.md", "Add widgets"),
				{Title: "Add more", SourcePath: "b.change.md", Section: "Added"},
			},
		}))
}

func TestRender(t *testing.T) {
	groups := []releaseGroup{
		{Changes: []*changefile.Changefile{aChange("a.change.md", "Not shipped yet")}},
		{
			Release: &releases.Release{Version: "1.0.0", ReleasedOn: "2026-01-15"},
			Changes: []*changefile.Changefile{aChange("b.change.md", "Add widgets")},
		},
	}

	var b bytes.Buffer
	require.NoError(t, render(&b, "go", releases.ChannelGA, groups))

	// The whole file: the generated-file warning, one h1, then a blank line before each
	// group.
	assert.Equal(t, generatedNotice+"\n\n# Changelog\n"+
		"\n## Unreleased\n* Not shipped yet\n"+
		"\n## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\n* Add widgets\n", b.String())
}

// A prerelease changelog opens with a standing note, between the title and the releases.
func TestRenderIncludesTheChannelNotice(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, render(&b, "python", releases.ChannelBeta, []releaseGroup{
		{Release: &releases.Release{Version: "1.0.0-beta.1", ReleasedOn: "2026-01-15"}},
	}))

	assert.Equal(t, generatedNotice+"\n\n# Changelog\n"+
		"\n"+channelNotice(releases.ChannelBeta, "python")+"\n"+
		"\n## <a id=\"1-0-0-beta-1\"></a>1.0.0-beta.1 - 2026-01-15\n", b.String())
}

// A repo with nothing recorded yet renders a changelog that is just its header, rather
// than failing or writing a stray blank line.
func TestRenderWithNoGroups(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, render(&b, "go", releases.ChannelGA, nil))

	assert.Equal(t, generatedNotice+"\n\n# Changelog\n", b.String())
}

var errWriteFailed = errors.New("write failed")

// failingWriter counts the writes it is given and fails on the nth one.
type failingWriter struct {
	writes int
	failOn int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failOn {
		return 0, errWriteFailed
	}
	return len(p), nil
}

// Every write is checked, so a disk filling up partway through leaves a failed build
// rather than a truncated CHANGELOG.md that reads as complete.
func TestRenderReportsAFailedWrite(t *testing.T) {
	groups := []releaseGroup{
		{Changes: []*changefile.Changefile{aChange("a.change.md", "Not shipped yet")}},
		{
			Release:              &releases.Release{Version: "1.0.0", ReleasedOn: "2026-01-15", PinnedAPIVersion: "2026-01-01"},
			PrevPinnedAPIVersion: "2025-01-01",
			Intro:                "Prose.",
			// One bullet in a section and one without, so both of the ways a section can
			// open are written.
			Changes: []*changefile.Changefile{
				aChange("b.change.md", "Fix retries"),
				{Title: "Add widgets", SourcePath: "c.change.md", Section: "Added", Body: "Some detail.\n"},
			},
		},
	}

	// A clean render first, to learn how many writes reaching the end takes.
	counted := &failingWriter{}
	require.NoError(t, render(counted, "python", releases.ChannelBeta, groups))
	require.NotZero(t, counted.writes)

	// Failing at any point along the way is reported.
	for failOn := 1; failOn <= counted.writes; failOn++ {
		err := render(&failingWriter{failOn: failOn}, "python", releases.ChannelBeta, groups)
		require.ErrorIs(t, err, errWriteFailed, "a failure on write %d was swallowed", failOn)
	}
}
