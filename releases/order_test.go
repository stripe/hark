package releases

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		// Comparing numerically rather than lexically is the whole point: as
		// strings "1.10.0" sorts below "1.9.0".
		{name: "numeric, not lexical", a: "1.10.0", b: "1.9.0", want: 1},
		{name: "major", a: "2.0.0", b: "10.0.0", want: -1},
		{name: "patch", a: "1.2.3", b: "1.2.4", want: -1},

		// A version needs all three components, so a short spelling is not another
		// way to say the same release — it is invalid, and sorts below.
		{name: "a two-component version is invalid", a: "1.2", b: "1.2.0", want: -1},
		{name: "two invalid versions are equal", a: "1.8", b: "8", want: 0},

		// A prerelease leads up to its GA release, so it sorts below it.
		{name: "prerelease below GA", a: "1.3.0-beta.1", b: "1.3.0", want: -1},
		{name: "alpha below beta", a: "1.3.0-alpha.9", b: "1.3.0-beta.1", want: -1},
		{name: "ordinals within a channel", a: "1.3.0-beta.2", b: "1.3.0-beta.10", want: -1},

		// The SDKs spell prereleases both ways.
		{name: "the compact spelling", a: "1.3.0b2", b: "1.3.0b10", want: -1},
		{name: "spellings agree", a: "1.3.0-beta.2", b: "1.3.0b2", want: 0},

		{name: "equal", a: "1.2.3", b: "1.2.3", want: 0},
		{name: "an unrecognized suffix is invalid, so it sorts below", a: "1.2.3+build7", b: "1.2.3", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Compare(tt.a, tt.b))
			// The comparison has to be antisymmetric, or a sort using it is
			// undefined.
			assert.Equal(t, -tt.want, Compare(tt.b, tt.a))
		})
	}
}

func TestInsert_IntoAnEmptyFile(t *testing.T) {
	f := &File{}

	assert.Equal(t, 0, f.Insert(Release{Version: "1.0.0", ReleasedOn: "2026-01-15"}))
	assert.Len(t, f.Releases, 1)
}

func TestInsert_NewestGoesFirst(t *testing.T) {
	f := &File{Releases: []Release{
		{Version: "1.1.0", ReleasedOn: "2026-02-01"},
		{Version: "1.0.0", ReleasedOn: "2026-01-15"},
	}}

	assert.Equal(t, 0, f.Insert(Release{Version: "1.2.0", ReleasedOn: "2026-03-01"}))
	assert.Equal(t, "1.2.0", f.Releases[0].Version)

	// An older release inserted afterwards slots in under it rather than displacing it.
	assert.Equal(t, 1, f.Insert(Release{Version: "1.1.1", ReleasedOn: "2026-02-05"}))
	assert.Equal(t, []string{"1.2.0", "1.1.1", "1.1.0", "1.0.0"}, versionStrings(f))
}

// The case AddVersion gets wrong: these repos keep old lines alive, so a patch
// backported after a newer release belongs where it happened, not at the top.
func TestInsert_BackportLandsByDate(t *testing.T) {
	f := &File{Releases: []Release{
		{Version: "87.0.0", ReleasedOn: "2026-09-02"},
		{Version: "86.4.0", ReleasedOn: "2026-08-20"},
	}}

	// Cut after 87.0.0 shipped, but dated before it.
	i := f.Insert(Release{Version: "86.4.1", ReleasedOn: "2026-08-25"})

	assert.Equal(t, 1, i)
	assert.Equal(t, []string{"87.0.0", "86.4.1", "86.4.0"}, versionStrings(f))
}

// The mirror of TestInsert_BackportLandsByDate: once a backport is sitting there, the
// entry above it in the file is not what it succeeded.
func TestPredecessor_SkipsPastABackport(t *testing.T) {
	f := &File{Releases: []Release{
		{Version: "15.1.0", ReleasedOn: "2025-04-30"},
		{Version: "13.5.1", ReleasedOn: "2025-04-21"},
		{Version: "15.0.0", ReleasedOn: "2025-04-09"},
		{Version: "13.5.0", ReleasedOn: "2025-02-24"},
	}}

	p := f.Predecessors()
	assert.Equal(t, "13.5.0", p["13.5.1"].Version)
	assert.Equal(t, "15.0.0", p["15.1.0"].Version)
	assert.Equal(t, "13.5.1", p["15.0.0"].Version)
}

func TestPredecessor_OldestHasNone(t *testing.T) {
	f := &File{Releases: []Release{
		{Version: "1.1.0", ReleasedOn: "2026-02-01"},
		{Version: "1.0.0", ReleasedOn: "2026-01-15"},
	}}

	assert.NotContains(t, f.Predecessors(), "1.0.0")
	assert.Empty(t, (&File{}).Predecessors())
}

// Prereleases sort below the GA release they lead up to.
func TestPredecessor_HandlesPrereleases(t *testing.T) {
	f := &File{Releases: []Release{
		{Version: "2.0.0", ReleasedOn: "2026-03-01"},
		{Version: "2.0.0-beta.2", ReleasedOn: "2026-02-20"},
		{Version: "2.0.0-beta.1", ReleasedOn: "2026-02-10"},
	}}

	p := f.Predecessors()
	assert.Equal(t, "2.0.0-beta.2", p["2.0.0"].Version)
	assert.Equal(t, "2.0.0-beta.1", p["2.0.0-beta.2"].Version)
}

func TestDuplicate(t *testing.T) {
	t.Run("no duplicates", func(t *testing.T) {
		f := &File{Releases: []Release{
			{Version: "1.1.0", ReleasedOn: "2026-02-01"},
			{Version: "1.0.0", ReleasedOn: "2026-01-15"},
		}}

		_, _, found := f.FirstDuplicate()
		assert.False(t, found)
	})

	// The pair need not be adjacent: two entries for one release usually carry
	// different dates, which puts them anywhere in a date-sorted file.
	t.Run("the same version listed twice", func(t *testing.T) {
		f := &File{Releases: []Release{
			{Version: "1.1.0", ReleasedOn: "2026-02-01"},
			{Version: "1.0.0", ReleasedOn: "2026-01-20"},
			{Version: "1.0.0", ReleasedOn: "2026-01-15"},
		}}

		a, b, found := f.FirstDuplicate()
		require.True(t, found)
		assert.Equal(t, "1.0.0", a.Version)
		assert.Equal(t, "1.0.0", b.Version)
	})

	// One release under two spellings is still one release, which a string
	// comparison would miss.
	t.Run("one release spelled two ways", func(t *testing.T) {
		f := &File{Releases: []Release{
			{Version: "1.3.0-beta.2", ReleasedOn: "2026-02-01"},
			{Version: "1.3.0b2", ReleasedOn: "2026-01-15"},
		}}

		a, b, found := f.FirstDuplicate()
		require.True(t, found)
		assert.ElementsMatch(t, []string{"1.3.0-beta.2", "1.3.0b2"}, []string{a.Version, b.Version})
	})

	// Invalid versions all compare equal to each other, and are reported for being
	// invalid rather than for being duplicates.
	t.Run("invalid versions are not duplicates of each other", func(t *testing.T) {
		f := &File{Releases: []Release{
			{Version: "not-a-version", ReleasedOn: "2026-02-01"},
			{Version: "2.1.0rc1", ReleasedOn: "2026-01-15"},
		}}

		_, _, found := f.FirstDuplicate()
		assert.False(t, found)
	})
}

func TestInsert_OldestGoesLast(t *testing.T) {
	f := &File{Releases: []Release{{Version: "1.0.0", ReleasedOn: "2026-01-15"}}}

	assert.Equal(t, 1, f.Insert(Release{Version: "0.9.0", ReleasedOn: "2025-12-01"}))
	assert.Equal(t, []string{"1.0.0", "0.9.0"}, versionStrings(f))
}

// Several releases a day is normal on a heavy release day, so the same-day tie has
// to be broken by version rather than left to chance.
func TestInsert_SameDayOrdersByVersionDescending(t *testing.T) {
	f := &File{Releases: []Release{
		{Version: "1.2.0", ReleasedOn: "2026-03-01"},
		{Version: "1.0.0", ReleasedOn: "2026-03-01"},
	}}

	assert.Equal(t, 1, f.Insert(Release{Version: "1.1.0", ReleasedOn: "2026-03-01"}))
	assert.Equal(t, []string{"1.2.0", "1.1.0", "1.0.0"}, versionStrings(f))
}

// A prerelease and its GA release often share a date.
func TestInsert_SameDayPrereleaseBelowGA(t *testing.T) {
	f := &File{Releases: []Release{{Version: "1.3.0", ReleasedOn: "2026-03-01"}}}

	assert.Equal(t, 1, f.Insert(Release{Version: "1.3.0-beta.1", ReleasedOn: "2026-03-01"}))
	assert.Equal(t, []string{"1.3.0", "1.3.0-beta.1"}, versionStrings(f))
}

// Insert places one entry; it must not quietly reorder the rest, because these
// files record releases that have already gone out.
func TestInsert_LeavesExistingOrderAlone(t *testing.T) {
	f := &File{Releases: []Release{
		{Version: "7.63.0", ReleasedOn: "2026-01-01"},
		{Version: "9.0.0", ReleasedOn: "2025-06-01"},
		{Version: "8.0.0", ReleasedOn: "2025-03-01"},
	}}

	f.Insert(Release{Version: "9.0.1", ReleasedOn: "2025-05-01"})

	assert.Equal(t, []string{"7.63.0", "9.0.0", "9.0.1", "8.0.0"}, versionStrings(f))
}

func versionStrings(f *File) []string {
	out := make([]string, 0, len(f.Releases))
	for _, v := range f.Releases {
		out = append(out, v.Version)
	}
	return out
}

func TestChannel(t *testing.T) {
	for _, tt := range []struct{ version, want string }{
		// GA is the absence of a suffix.
		{"22.6.0", ChannelGA},
		// Both spellings: SemVer for six SDKs, PEP 440 for python.
		{"22.7.0-beta.1", ChannelBeta},
		{"15.7.0b1", ChannelBeta},
		{"22.7.0-alpha.2", ChannelPrivatePreview},
		{"15.7.0a3", ChannelPrivatePreview},
	} {
		t.Run(tt.version, func(t *testing.T) {
			channel, ok := Channel(tt.version)
			require.True(t, ok)
			assert.Equal(t, tt.want, channel)
		})
	}
}

// A version Channel cannot read has no channel, rather than being reported as GA
// because that is where an unrecognised suffix happens to sort.
func TestChannel_RejectsWhatItCannotRead(t *testing.T) {
	for _, version := range []string{
		"2.1.0rc1",       // the one release candidate in the SDKs' history
		"1.3.0-rc.1",     // and the dotted spelling of one
		"1.0.0-nonsense", // any other suffix
		"v1.2.0",         // a tag, not a version
		"not-a-version",
		"",
		// All three components are required, so neither a short nor a long
		// spelling is a version hark will accept.
		"1.2",
		"1",
		"1.2.3.4",
		// Nor is a zero-padded one, which would be a second name for 1.2.3.
		"01.2.3",
		"1.02.3",
		"1.2.03",
		"1.2.3-beta.01",
		"1.2.3b01",
		// A prerelease always says which one it is, so an ordinal-less suffix is not
		// another way to write `-beta.0`.
		"1.2.3-beta",
		"1.2.3-alpha",
		// The long spelling takes its dot, so `-beta1` is not `-beta.1`.
		"1.2.3-beta1",
		"1.2.3-alpha2",
	} {
		t.Run(version, func(t *testing.T) {
			channel, ok := Channel(version)
			assert.False(t, ok, "should not be readable")
			assert.Empty(t, channel, "no channel to report")
			// The two agree, so a caller can use either as the guard.
			assert.False(t, IsVersionValid(version))
		})
	}
}

// Channels is the complete set Channel can answer, so validate can never reject a
// release for belonging to a channel a file is not allowed to record.
func TestChannels(t *testing.T) {
	assert.Equal(t, []string{ChannelGA, ChannelBeta, ChannelPrivatePreview}, Channels)

	for _, version := range []string{
		"1.2.0", "1.3.0-beta.1", "15.7.0b1", "1.3.0-alpha.1", "15.7.0a3",
	} {
		channel, ok := Channel(version)
		require.True(t, ok, "version %q", version)
		assert.Contains(t, Channels, channel, "version %q", version)
	}
}

func TestUnsorted(t *testing.T) {
	for _, tt := range []struct {
		name          string
		releases      []Release
		before, after string
		found         bool
	}{
		{
			name: "sorted",
			releases: []Release{
				{Version: "2.0.0", ReleasedOn: "2026-03-01"},
				{Version: "1.1.0", ReleasedOn: "2026-02-01"},
				{Version: "1.0.0", ReleasedOn: "2026-01-01"},
			},
		},
		{
			name: "dates ascending",
			releases: []Release{
				{Version: "1.0.0", ReleasedOn: "2026-01-01"},
				{Version: "2.0.0", ReleasedOn: "2026-03-01"},
			},
			before: "1.0.0", after: "2.0.0", found: true,
		},
		{
			name: "same day, versions ascending",
			releases: []Release{
				{Version: "1.0.0", ReleasedOn: "2026-01-01"},
				{Version: "1.1.0", ReleasedOn: "2026-01-01"},
			},
			before: "1.0.0", after: "1.1.0", found: true,
		},
		{
			name: "same day, versions descending",
			releases: []Release{
				{Version: "1.1.0", ReleasedOn: "2026-01-01"},
				{Version: "1.0.0", ReleasedOn: "2026-01-01"},
			},
		},
		{
			name: "same day, prerelease below its GA",
			releases: []Release{
				{Version: "1.1.0", ReleasedOn: "2026-01-01"},
				{Version: "1.1.0-beta.1", ReleasedOn: "2026-01-01"},
			},
		},
		{
			name: "a backport dated after a later major",
			releases: []Release{
				{Version: "1.0.1", ReleasedOn: "2026-03-01"},
				{Version: "2.0.0", ReleasedOn: "2026-02-01"},
			},
		},
		{
			name:     "empty",
			releases: nil,
		},
		{
			name:     "one",
			releases: []Release{{Version: "1.0.0", ReleasedOn: "2026-01-01"}},
		},
		{
			name: "reported pair is the first one out of order",
			releases: []Release{
				{Version: "3.0.0", ReleasedOn: "2026-04-01"},
				{Version: "1.0.0", ReleasedOn: "2026-01-01"},
				{Version: "2.0.0", ReleasedOn: "2026-02-01"},
				{Version: "0.9.0", ReleasedOn: "2025-01-01"},
			},
			before: "1.0.0", after: "2.0.0", found: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{Releases: tt.releases}
			before, after, found := f.Unsorted()

			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.before, before.Version)
			assert.Equal(t, tt.after, after.Version)
		})
	}
}

func TestUnsorted_AgreesWithInsert(t *testing.T) {
	f := &File{}
	for _, r := range []Release{
		{Version: "1.0.0", ReleasedOn: "2026-01-01"},
		{Version: "2.0.0", ReleasedOn: "2026-03-01"},
		{Version: "1.0.1", ReleasedOn: "2026-04-01"},
		{Version: "2.1.0-beta.1", ReleasedOn: "2026-03-01"},
	} {
		f.Insert(r)
	}

	_, _, found := f.Unsorted()
	assert.False(t, found, "Insert should never produce an order Unsorted rejects")
}

func TestCompare_InvalidVersions(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{"not-a-version", "1.0.0", -1},
		{"5.0.0-rc.1", "1.0.0", -1},
		{"2.1.0rc1", "2.1.0", -1},
		{"1.0.0", "2.1.0rc1", 1},
		{"not-a-version", "also-not", 0},
		{"2.1.0rc1", "1.3.0-rc.1", 0},
		{"", "1.0.0", -1},
	} {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, Compare(tt.a, tt.b))
			assert.Equal(t, -tt.want, Compare(tt.b, tt.a))
		})
	}
}

func TestCompare_IsATotalOrder(t *testing.T) {
	versions := []string{
		"2.1.0", "1.0.0", "1.0.0-beta.1", "2.1.0rc1", "not-a-version", "1.10.0", "",
	}

	for _, a := range versions {
		for _, b := range versions {
			ab, ba := Compare(a, b), Compare(b, a)
			assert.Equal(t, ab, -ba, "Compare(%q,%q)=%d but Compare(%q,%q)=%d", a, b, ab, b, a, ba)
		}
	}

	slices.SortFunc(versions, Compare)

	// Invalid versions compare equal, so their order among themselves is unspecified.
	assert.ElementsMatch(t, []string{"", "2.1.0rc1", "not-a-version"}, versions[:3])
	assert.Equal(t, []string{"1.0.0-beta.1", "1.0.0", "1.10.0", "2.1.0"}, versions[3:])
}
