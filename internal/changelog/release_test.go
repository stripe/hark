package changelog

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

// releaseFixture is a repo with the given releases file and changefiles, and a
// clock stopped on a known date.
func releaseFixture(t *testing.T, versionsJSON string, changefiles map[string]string) (afero.Fs, Options) {
	t.Helper()

	// Release validates the repo before it writes anything, and validate wants metadata.
	// Tests that care about the channel pass their own; the rest get GA so they can be
	// about the release itself.
	if !strings.Contains(versionsJSON, `"metadata"`) {
		versionsJSON = strings.Replace(versionsJSON, "{", "{"+gaMetadata, 1)
	}

	fs := buildFixture(t, versionsJSON, changefiles)
	return fs, Options{
		Fs:  fs,
		Out: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, 9, 9, 12, 0, 0, 0, time.Local) },
	}
}

// releasedVersions is the releases file as it stands on fs.
func releasedVersions(t *testing.T, fs afero.Fs) *releases.File {
	t.Helper()

	f, err := releases.ReadFile(fs, versionsFixturePath)
	require.NoError(t, err)
	return f
}

const pendingChangefile = "---\ntitle: \"Add widgets\"\n---\n"

func TestRelease_RequiresVersion(t *testing.T) {
	_, opts := releaseFixture(t, `{"releases":[]}`, nil)

	require.Error(t, Release(context.Background(), opts, releases.Release{}))
}

// The whole operation: pending changes get stamped, the version gets recorded, and
// the changelog reflects both.
func TestRelease_StampsRecordsAndRebuilds(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile,
	})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.0.0"}))

	// stamped
	cf := read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md")
	assert.Equal(t, "1.0.0", cf.ReleasedInVersion)

	// recorded, dated today
	entry := releasedVersions(t, fs).Find("1.0.0")
	require.NotNil(t, entry)
	assert.Equal(t, "2026-09-09", entry.ReleasedOn)

	// rebuilt
	changelog, err := afero.ReadFile(fs, changelogPath)
	require.NoError(t, err)
	assert.Contains(t, string(changelog), "## <a id=\"1-0-0\"></a>1.0.0 - 2026-09-09\n* Add widgets\n")
}

// A comment stays in the changefile that stamping rewrites, and stays out of the
// changelog that stamping rebuilds.
func TestRelease_KeepsCommentsInTheChangefileOnly(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": "---\ntitle: \"Add widgets\"\n---\n\n" +
			"<!-- reviewers: is this clear? -->\nSome detail.\n",
	})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.0.0"}))

	cf := read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md")
	assert.Equal(t, "<!-- reviewers: is this clear? -->\nSome detail.", cf.Body)

	changelog, err := afero.ReadFile(fs, changelogPath)
	require.NoError(t, err)
	assert.Contains(t, string(changelog), "* Add widgets\n\n  Some detail.\n")
	assert.NotContains(t, string(changelog), "reviewers")
}

// Changes that already shipped belong to the release they shipped in.
func TestRelease_LeavesAlreadyReleasedChangesAlone(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_shipped.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n",
			"2026-09-08_xavdid_pending.change.md": pendingChangefile,
		})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.1.0"}))

	assert.Equal(t, "1.0.0", read(t, fs, changesFixtureDir+"/2026-01-14_xavdid_shipped.change.md").ReleasedInVersion)
	assert.Equal(t, "1.1.0", read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_pending.change.md").ReleasedInVersion)
}

// Both of these describe a standing state of the SDK rather than something that
// happened in this release, so they persist until they change.
func TestRelease_InheritsPinnedAndRuntimeVersions(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"1.0.0","released_on":"2026-01-15","pinned_api_version":"2026-01-01.dahlia","minimum_runtime_version":"3.9"}
	]}`, nil)

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.1.0"}))

	entry := releasedVersions(t, fs).Find("1.1.0")
	require.NotNil(t, entry)
	assert.Equal(t, "2026-01-01.dahlia", entry.PinnedAPIVersion)
	assert.Equal(t, "3.9", entry.MinimumRuntimeVersion)
}

// A backport inherits from its own line, not from whatever shipped most recently.
// stripe-ruby's real shape: 13.5.1 was cut after 15.0.0 was already out, so the entry
// directly below it in the date-sorted file is a 15.x release whose pinned version and
// runtime floor have nothing to do with 13.x.
func TestRelease_BackportInheritsFromItsOwnLine(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"15.0.0","released_on":"2026-04-09","pinned_api_version":"2026-03-31.basil","minimum_runtime_version":"3.10"},
		{"version":"13.5.0","released_on":"2026-02-24","pinned_api_version":"2026-02-24.acacia","minimum_runtime_version":"3.9"}
	]}`, nil)

	// Dated after 13.5.0 but before 15.0.0, so it lands between them in the file.
	require.NoError(t, Release(context.Background(), opts, releases.Release{
		Version: "13.5.1", ReleasedOn: "2026-04-01",
	}))

	entry := releasedVersions(t, fs).Find("13.5.1")
	require.NotNil(t, entry)
	assert.Equal(t, "2026-02-24.acacia", entry.PinnedAPIVersion, "should follow 13.5.0, not 15.0.0")
	assert.Equal(t, "3.9", entry.MinimumRuntimeVersion, "should follow 13.5.0, not 15.0.0")
}

func TestRelease_ExplicitValuesOverrideWhatWouldBeInherited(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"1.0.0","released_on":"2026-01-15","pinned_api_version":"2026-01-01.dahlia","minimum_runtime_version":"3.9"}
	]}`, nil)

	require.NoError(t, Release(context.Background(), opts, releases.Release{
		Version:               "1.1.0",
		PinnedAPIVersion:      "2026-08-26.dahlia",
		MinimumRuntimeVersion: "3.10",
	}))

	entry := releasedVersions(t, fs).Find("1.1.0")
	require.NotNil(t, entry)
	assert.Equal(t, "2026-08-26.dahlia", entry.PinnedAPIVersion)
	assert.Equal(t, "3.10", entry.MinimumRuntimeVersion)
}

// A backported patch inherits from the release it followed, not from whatever
// happens to be newest.
func TestRelease_InheritsFromTheReleaseBelowIt(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"87.0.0","released_on":"2026-09-02","pinned_api_version":"2026-08-26.dahlia","minimum_runtime_version":"3.10"},
		{"version":"86.4.0","released_on":"2026-08-20","pinned_api_version":"2026-07-29.dahlia","minimum_runtime_version":"3.9"}
	]}`, nil)

	// Dated between the two, so it lands under 87.0.0 and above 86.4.0.
	require.NoError(t, Release(context.Background(), opts, releases.Release{
		Version:    "86.4.1",
		ReleasedOn: "2026-08-25",
	}))

	entry := releasedVersions(t, fs).Find("86.4.1")
	require.NotNil(t, entry)
	assert.Equal(t, "2026-07-29.dahlia", entry.PinnedAPIVersion)
	assert.Equal(t, "3.9", entry.MinimumRuntimeVersion)
}

// AddVersion would put this at the top; it belongs where it happened.
func TestRelease_BackportLandsByDate(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"87.0.0","released_on":"2026-09-02"},
		{"version":"86.4.0","released_on":"2026-08-20"}
	]}`, nil)

	require.NoError(t, Release(context.Background(), opts, releases.Release{
		Version:    "86.4.1",
		ReleasedOn: "2026-08-25",
	}))

	got := releasedVersions(t, fs)
	assert.Equal(t, "86.4.1", got.Releases[1].Version)
}

// Someone can add the entry by hand ahead of the release to record runtime
// metadata. Cutting the release should use what they wrote.
func TestRelease_UsesAPreCreatedEntry(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"1.1.0","released_on":"2026-09-01","pinned_api_version":"2026-08-26.dahlia","minimum_runtime_version":"3.10"},
		{"version":"1.0.0","released_on":"2026-01-15"}
	]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile,
	})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.1.0"}))

	got := releasedVersions(t, fs)
	entry := got.Find("1.1.0")
	require.NotNil(t, entry)

	// Its own values, not today's date and not inherited ones.
	assert.Equal(t, "2026-09-01", entry.ReleasedOn)
	assert.Equal(t, "2026-08-26.dahlia", entry.PinnedAPIVersion)
	assert.Equal(t, "3.10", entry.MinimumRuntimeVersion)

	// And the pending change still got stamped with it.
	assert.Equal(t, "1.1.0", read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)
}

// A release compiles the changelog readers see and stamps every pending changefile, so a
// repo that does not validate is not one to release from.
func TestRelease_RefusesWhenTheRepoDoesNotValidate(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": "---\ntitle: \"" + placeholderTitle + "\"\n---\n",
	})

	err := Release(context.Background(), opts, releases.Release{Version: "1.0.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not releasing 1.0.0")

	// And nothing was written: no entry recorded, no changefile stamped, no changelog.
	assert.Empty(t, releasedVersions(t, fs).Releases)
	assert.Empty(t, read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)

	exists, err := afero.Exists(fs, changelogPath)
	require.NoError(t, err)
	assert.False(t, exists, "the changelog should not have been compiled")
}

// These files record releases that have already gone out, so an existing entry keeps its
// position even when its date says it should sort elsewhere.
//
// Against placeRelease rather than Release: showing that nothing re-sorts needs a file
// that is already out of order, which is a state Release now refuses to run on at all.
func TestPlaceRelease_APreCreatedEntryKeepsItsPosition(t *testing.T) {
	file := &releases.File{Releases: []releases.Release{
		{Version: "1.0.0", ReleasedOn: "2026-01-15"},
		{Version: "1.1.0", ReleasedOn: "2026-09-01"},
	}}

	entry, err := placeRelease(file, releases.Release{Version: "1.1.0"}, "2026-09-09")
	require.NoError(t, err)

	assert.Equal(t, "1.1.0", entry.Version)
	assert.Equal(t, []string{"1.0.0", "1.1.0"},
		[]string{file.Releases[0].Version, file.Releases[1].Version})
	assert.Len(t, file.Releases, 2, "the entry should be reused, not duplicated")
}

// A blank field on a pre-created entry is one the release is expected to fill in.
func TestRelease_PatchesABlankFieldOnAPreCreatedEntry(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"1.1.0","released_on":"2026-09-01","minimum_runtime_version":"3.10"}
	]}`, nil)

	require.NoError(t, Release(context.Background(), opts, releases.Release{
		Version:          "1.1.0",
		PinnedAPIVersion: "2026-08-26.dahlia",
	}))

	entry := releasedVersions(t, fs).Find("1.1.0")
	require.NotNil(t, entry)
	assert.Equal(t, "2026-08-26.dahlia", entry.PinnedAPIVersion)
	assert.Equal(t, "3.10", entry.MinimumRuntimeVersion)
}

// Disagreeing with what is already recorded means one of the two is wrong, and
// guessing which would silently rewrite a release's metadata.
func TestRelease_RefusesConflictingValues(t *testing.T) {
	_, opts := releaseFixture(t, `{"releases":[
		{"version":"1.1.0","released_on":"2026-09-01","pinned_api_version":"2026-08-26.dahlia"}
	]}`, nil)

	err := Release(context.Background(), opts, releases.Release{
		Version:          "1.1.0",
		PinnedAPIVersion: "2026-07-29.dahlia",
	})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "pinned_api_version")
	assert.Contains(t, err.Error(), "2026-08-26.dahlia")
	assert.Contains(t, err.Error(), "2026-07-29.dahlia")
}

// Every conflict at once, so the caller does not have to rerun to find the next.
func TestRelease_ReportsEveryConflict(t *testing.T) {
	_, opts := releaseFixture(t, `{"releases":[
		{"version":"1.1.0","released_on":"2026-09-01","pinned_api_version":"2026-08-26.dahlia","minimum_runtime_version":"3.10"}
	]}`, nil)

	err := Release(context.Background(), opts, releases.Release{
		Version:               "1.1.0",
		PinnedAPIVersion:      "2026-07-29.dahlia",
		MinimumRuntimeVersion: "3.9",
	})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "pinned_api_version")
	assert.Contains(t, err.Error(), "minimum_runtime_version")
}

// A rejected release must leave the repo exactly as it was, or the next attempt
// starts from a half-cut state.
func TestRelease_AConflictWritesNothing(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[
		{"version":"1.1.0","released_on":"2026-09-01","pinned_api_version":"2026-08-26.dahlia"}
	]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile,
	})

	err := Release(context.Background(), opts, releases.Release{
		Version:          "1.1.0",
		PinnedAPIVersion: "2026-07-29.dahlia",
	})
	require.Error(t, err)

	assert.Empty(t, read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)

	exists, err := afero.Exists(fs, changelogPath)
	require.NoError(t, err)
	assert.False(t, exists, "the changelog should not have been rebuilt")
}

// Some releases shipped with nothing user-facing in them. They still belong in the
// record, rendered as a heading with no bullets under it.
func TestRelease_WithNoPendingChanges(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`,
		map[string]string{
			"2026-01-14_xavdid_shipped.change.md": "---\ntitle: \"Shipped\"\nreleased_in_version: \"1.0.0\"\n---\n",
		})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.1.0"}))

	changelog, err := afero.ReadFile(fs, changelogPath)
	require.NoError(t, err)
	assert.Contains(t, string(changelog), "## <a id=\"1-1-0\"></a>1.1.0 - 2026-09-09\n\n## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\n")
}

// An intro written ahead of the release is picked up by version, with nothing
// recording that it exists.
func TestRelease_PicksUpAnIntroWrittenAhead(t *testing.T) {
	var out bytes.Buffer
	fs, opts := releaseFixture(t, `{"releases":[]}`, nil)
	opts.Out = &out
	writeIntro(t, fs, "2.0.0", "The one where the client got a rewrite.")

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "2.0.0"}))

	changelog, err := afero.ReadFile(fs, changelogPath)
	require.NoError(t, err)
	assert.Contains(t, string(changelog), "The one where the client got a rewrite.")

	// And the captain is told it was found, since nothing else would confirm it.
	assert.Contains(t, out.String(), "using intro .hark/intros/intro-2.0.0.md")
}

// An intro is optional, but silence would leave the captain wondering whether the
// prose they wrote is going to show up — so say where one would go.
func TestRelease_SaysWhereAnIntroWouldGo(t *testing.T) {
	var out bytes.Buffer
	_, opts := releaseFixture(t, `{"releases":[]}`, nil)
	opts.Out = &out

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "2.0.0"}))

	assert.Contains(t, out.String(), ".hark/intros/intro-2.0.0.md")
}

// The intro belongs to the release it names, not to whichever is being cut.
func TestRelease_DoesNotBorrowAnotherReleasesIntro(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[{"version":"1.0.0","released_on":"2026-01-15"}]}`, nil)
	writeIntro(t, fs, "1.0.0", "Belongs to 1.0.0.")

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.1.0"}))

	changelog, err := afero.ReadFile(fs, changelogPath)
	require.NoError(t, err)
	assert.Contains(t, string(changelog), "## <a id=\"1-1-0\"></a>1.1.0 - 2026-09-09\n\n## <a id=\"1-0-0\"></a>1.0.0 - 2026-01-15\nBelongs to 1.0.0.\n")
}

func TestRelease_ReportsWhatItDid(t *testing.T) {
	var out bytes.Buffer
	fs, opts := releaseFixture(t, `{"releases":[]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile,
		"2026-09-07_xavdid_fix-retries.change.md": "---\ntitle: \"Fix retries\"\n---\n",
	})
	opts.Out = &out
	_ = fs

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.0.0"}))

	assert.Contains(t, out.String(), "released 1.0.0 on 2026-09-09 (2 changes)")
	assert.Contains(t, out.String(), "wrote CHANGELOG.md")
}

// Rewriting a changefile round-trips it through Serialize, which must not lose
// anything the author wrote.
func TestRelease_PreservesChangefileContents(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": "---\n" +
			"title: Add support for widgets\n" +
			"pr_url: https://github.com/stripe/stripe-go/pull/123\n" +
			"semver_level: major\n" +
			"is_stripe_api_change: true\n" +
			"jira_tickets_closed:\n  - DEVSDK-456\n" +
			"section: ⚠️ Removed\n" +
			"---\n\n- widgets can now be created\n  - and destroyed\n",
	})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.0.0"}))

	cf := read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md")
	assert.Equal(t, "Add support for widgets", cf.Title)
	assert.Equal(t, "https://github.com/stripe/stripe-go/pull/123", cf.PRUrl)
	assert.Equal(t, changefile.SemverLevelMajor, cf.SemverLevel)
	assert.True(t, cf.IsStripeAPIChange)
	assert.Equal(t, []string{"DEVSDK-456"}, cf.JiraTicketsClosed)
	assert.Equal(t, "⚠️ Removed", cf.Section)
	assert.Equal(t, "- widgets can now be created\n  - and destroyed", cf.Body)
	assert.Equal(t, "1.0.0", cf.ReleasedInVersion)
}

// Without a releases file there is no way to order or date releases, so there is
// nothing to cut against.
func TestRelease_RequiresAVersionsFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	var out bytes.Buffer

	err := Release(context.Background(), Options{Fs: fs, Out: &out}, releases.Release{Version: "1.0.0"})
	require.Error(t, err)
}

// An unreadable version is refused before anything is written, rather than being
// recorded and stamped onto every pending changefile for validate to reject later.
func TestRelease_RefusesAnInvalidVersion(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`, map[string]string{
		"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile,
	})

	err := Release(context.Background(), opts, releases.Release{Version: "2.1.0rc1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid version")

	// Nothing recorded, and the changefile left pending.
	assert.Empty(t, releasedVersions(t, fs).Releases)
	assert.Empty(t, read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)
}

// Each branch's releases file holds one channel, so cutting a GA version on a beta
// branch is a mistake rather than a GA release.
func TestRelease_RefusesAVersionFromAnotherChannel(t *testing.T) {
	fs, opts := releaseFixture(t,
		`{"metadata":{"language":"go","channel":"beta"},"releases":[]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})

	err := Release(context.Background(), opts, releases.Release{Version: "1.2.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1.2.0 is a ga version, but .hark/releases.json records beta releases")

	assert.Empty(t, releasedVersions(t, fs).Releases)
}

// And the matching one is recorded as normal.
func TestRelease_AcceptsAVersionInTheFilesChannel(t *testing.T) {
	fs, opts := releaseFixture(t,
		`{"metadata":{"language":"go","channel":"beta"},"releases":[]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.2.0-beta.1"}))

	recorded := releasedVersions(t, fs).Releases
	require.Len(t, recorded, 1)
	assert.Equal(t, "1.2.0-beta.1", recorded[0].Version)
	assert.Equal(t, "1.2.0-beta.1",
		read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)
}

// `1.0.0b1` and `1.0.0-beta.1` are one release spelled two ways, so cutting one while the
// other is recorded would put two entries in the file for a single release — which validate
// rejects, but only after the release had been written.
func TestRelease_RefusesAVersionAlreadyRecordedUnderAnotherSpelling(t *testing.T) {
	fs, opts := releaseFixture(t,
		`{"metadata":{"language":"python","channel":"beta"},"releases":[{"version":"1.0.0-beta.1","released_on":"2026-09-08"}]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})

	err := Release(context.Background(), opts, releases.Release{Version: "1.0.0b1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1.0.0b1 is already recorded as 1.0.0-beta.1")

	// The one entry stands, and the pending changefile was not stamped with either spelling.
	recorded := releasedVersions(t, fs).Releases
	require.Len(t, recorded, 1)
	assert.Equal(t, "1.0.0-beta.1", recorded[0].Version)
	assert.Empty(t, read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)
}

// The spelling already in the file is of course accepted, so a rerun of the same release is
// still the no-op it was.
func TestRelease_AcceptsTheSpellingAlreadyRecorded(t *testing.T) {
	fs, opts := releaseFixture(t,
		`{"metadata":{"language":"python","channel":"beta"},"releases":[{"version":"1.0.0b1","released_on":"2026-09-08"}]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "1.0.0b1"}))

	recorded := releasedVersions(t, fs).Releases
	require.Len(t, recorded, 1)
	assert.Equal(t, "1.0.0b1", recorded[0].Version)
	assert.Equal(t, "1.0.0b1",
		read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)
}

// guidePath is where the migration guide for a major version would be.
func guidePath(major int) string {
	return HarkDir + "/" + MigrationGuidesDir + "/" + MigrationGuideName(major)
}

// So there's always a file to write the next breaking change into, releasing a version
// opens the guide for the major after it.
func TestRelease_SeedsTheNextMajorsMigrationGuide(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "2.3.4"}))

	guide, err := afero.ReadFile(fs, guidePath(3))
	require.NoError(t, err)

	// It opens with a title naming the major it covers, so a reader who found the file on
	// its own knows which upgrade it is about. The rest is the instructions for writing it.
	firstLine, rest, _ := strings.Cut(string(guide), "\n")
	assert.True(t, strings.HasPrefix(firstLine, "# "), "leads with an h1, got %q", firstLine)
	assert.Contains(t, firstLine, "v3")
	assert.NotEmpty(t, strings.TrimSpace(rest))

	// The author is told where it went, since nothing else in hark reads the directory.
	assert.Contains(t, opts.Out.(*bytes.Buffer).String(), guidePath(3))
}

// The major in the title is the one the file is named for, not the one just released.
func TestRelease_MigrationGuideTitleMatchesItsName(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "9.0.1"}))

	guide, err := afero.ReadFile(fs, guidePath(10))
	require.NoError(t, err)
	assert.Contains(t, string(guide), "v10")
	assert.NotContains(t, string(guide), "v9")
}

// Whatever is in the guide already is someone's work in progress, and every release after
// the first would otherwise walk over it.
func TestRelease_LeavesAnExistingMigrationGuideAlone(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})
	require.NoError(t, afero.WriteFile(fs, guidePath(3), []byte("## Upgrading to v3\n"), 0o644))

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "2.3.4"}))

	guide, err := afero.ReadFile(fs, guidePath(3))
	require.NoError(t, err)
	assert.Equal(t, "## Upgrading to v3\n", string(guide))
}

// refusesToCreate is a filesystem that won't create one path, so a test can watch what
// happens when seeding the migration guide fails.
type refusesToCreate struct {
	afero.Fs
	path string
}

func (r refusesToCreate) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if name == r.path {
		return nil, errors.New("disk is on fire")
	}
	return r.Fs.OpenFile(name, flag, perm)
}

// The release is already recorded by the time the guide is seeded, so failing to seed one
// reports the trouble instead of reporting a release that did happen as an error.
func TestRelease_SurvivesAFailureToSeedTheMigrationGuide(t *testing.T) {
	fs, opts := releaseFixture(t, `{"releases":[]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})
	opts.Fs = refusesToCreate{Fs: fs, path: guidePath(3)}

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "2.3.4"}))

	// The release stands, and the changelog was still rebuilt.
	assert.Equal(t, "2.3.4",
		read(t, fs, changesFixtureDir+"/2026-09-08_xavdid_add-widgets.change.md").ReleasedInVersion)
	changelog, err := afero.ReadFile(fs, changelogPath)
	require.NoError(t, err)
	assert.Contains(t, string(changelog), "* Add widgets")

	// The trouble is said out loud rather than swallowed: which file, and what went wrong
	// with it. The wording around those is free to change.
	out := opts.Out.(*bytes.Buffer).String()
	assert.Contains(t, out, guidePath(3))
	assert.Contains(t, out, "disk is on fire")
}

// A prerelease is previewing a major that hasn't shipped, so the guide it needs is its own
// major's — which the last GA release before it seeded.
func TestRelease_PrereleasesSeedNoMigrationGuide(t *testing.T) {
	fs, opts := releaseFixture(t,
		`{"metadata":{"language":"go","channel":"beta"},"releases":[]}`,
		map[string]string{"2026-09-08_xavdid_add-widgets.change.md": pendingChangefile})

	require.NoError(t, Release(context.Background(), opts, releases.Release{Version: "3.0.0-beta.1"}))

	for _, major := range []int{3, 4} {
		exists, err := afero.Exists(fs, guidePath(major))
		require.NoError(t, err)
		assert.False(t, exists, "v%d", major)
	}
}
