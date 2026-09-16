package releases

import (
	"cmp"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// A regex to match prerelease suffixes like `-beta.2` or `a3`
var prereleaseRegex = regexp.MustCompile(`^(?:-(alpha|beta)\.(0|[1-9][0-9]*)|(a|b)(0|[1-9][0-9]*))$`)

// match a semver version without any suffix
var baseVersionRegex = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.(?:0|[1-9][0-9]*)){2}$`)

const (
	ChannelGA             = "ga"
	ChannelBeta           = "beta"
	ChannelPrivatePreview = "private-preview"
)

// The valid release channels for an SDK
var Channels = []string{ChannelGA, ChannelBeta, ChannelPrivatePreview}

// so we can sort releases relative to each other
const (
	rankAlpha = 1
	rankBeta  = 2
	rankGA    = 3
)

// Given a version, return the [Channels] it belongs to.
// `ok` is false for a version that [IsVersionValid] rejects.
func Channel(version string) (channel string, ok bool) {
	if !IsVersionValid(version) {
		return "", false
	}

	_, rank, _ := splitRelease(version)
	switch rank {
	case rankAlpha:
		return ChannelPrivatePreview, true
	case rankBeta:
		return ChannelBeta, true
	default:
		return ChannelGA, true
	}
}

// parse out a version's leading component. `ok` is false for a version that [IsVersionValid] rejects.
func Major(version string) (major int, ok bool) {
	if !IsVersionValid(version) {
		return 0, false
	}

	nums, _, _ := splitRelease(version)
	return nums[0], true
}

// compare two version strings for sorting. Each component is compared numerically. Invalid versions are sorted to the bottom.
func Compare(a, b string) int {
	switch aValid, bValid := IsVersionValid(a), IsVersionValid(b); {
	case !aValid && !bValid:
		return 0
	case !aValid:
		return -1
	case !bValid:
		return 1
	}

	na, ra, oa := splitRelease(a)
	nb, rb, ob := splitRelease(b)

	if c := slices.Compare(na, nb); c != 0 {
		return c
	}
	if c := cmp.Compare(ra, rb); c != 0 {
		return c
	}
	return cmp.Compare(oa, ob)
}

// separates a version's leading numeric components from whatever follows.
func splitSuffix(v string) (base, rest string) {
	i := 0
	for i < len(v) && (v[i] >= '0' && v[i] <= '9' || v[i] == '.') {
		i++
	}
	base = strings.TrimSuffix(v[:i], ".") // a trailing dot belongs to the suffix
	return base, v[len(base):]
}

// IsVersionValid reports whether its input is a valid version string, like "1.2.3" or "4.5.6-alpha.2" or "7.8.9b2".
// This mostly follows semver, but we allow fewer odd shapes
func IsVersionValid(v string) bool {
	base, rest := splitSuffix(v)
	if !baseVersionRegex.MatchString(base) {
		return false
	}
	return rest == "" || prereleaseRegex.MatchString(rest)
}

// splitRelease breaks a release into its numeric components, prerelease rank, and
// prerelease ordinal. "22.6.0-alpha.2" -> ([22,6,0], rankAlpha, 2).
func splitRelease(v string) (nums []int, rank, ordinal int) {
	base, rest := splitSuffix(v)

	nums = splitNums(base)
	if rest == "" {
		return nums, rankGA, 0
	}

	m := prereleaseRegex.FindStringSubmatch(rest)
	if m == nil {
		return nums, rankGA, 0 // unrecognized suffix: order as GA
	}

	kind, ord := m[1], m[2]
	if kind == "" {
		kind, ord = m[3], m[4]
	}

	rank = rankGA
	switch kind {
	case "alpha", "a":
		rank = rankAlpha
	case "beta", "b":
		rank = rankBeta
	}

	n, _ := strconv.Atoi(ord)
	return nums, rank, n
}

// splitNums parses the leading dotted numeric components of a version, stopping at the first component that is not purely numeric.
func splitNums(s string) []int {
	var out []int
	for part := range strings.SplitSeq(s, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

// Insert places a new release into its place in a sorted list of releases, returning the insertion index.
func (f *File) Insert(r Release) int {
	i := sort.Search(len(f.Releases), func(i int) bool {
		return less(r, f.Releases[i])
	})

	f.Releases = slices.Insert(f.Releases, i, r)
	return i
}

// returns the first adjacent pair of releases that are in the wrong order and whether there was one
func (f *File) Unsorted() (before, after Release, found bool) {
	for i := 1; i < len(f.Releases); i++ {
		if less(f.Releases[i], f.Releases[i-1]) {
			return f.Releases[i-1], f.Releases[i], true
		}
	}
	return Release{}, Release{}, false
}

// builds a map of version -> *Release that directly proceeded it. Used when deciding if the pinned API version changed between releases.
// Have to calculate rather than just check the previous release chronologically because we occasionally backport versions (even in the pinned API version era!)
// so the "last" release may not be the one this is based on.
func (f *File) Predecessors() map[string]*Release {
	sorted := make([]*Release, len(f.Releases))
	for i := range f.Releases {
		sorted[i] = &f.Releases[i]
	}
	slices.SortFunc(sorted, func(a, b *Release) int { return Compare(b.Version, a.Version) })

	predecessors := make(map[string]*Release, len(sorted))
	for i := 1; i < len(sorted); i++ {
		predecessors[sorted[i-1].Version] = sorted[i]
	}
	return predecessors
}

// returns the first pair of [Release]s that share a version.
//
// Entries that [IsVersionValid] rejects are skipped, since those all compare equal to each other and are reported as invalid on their own.
func (f *File) FirstDuplicate() (a, b Release, found bool) {
	valid := make([]Release, 0, len(f.Releases))
	for _, r := range f.Releases {
		if IsVersionValid(r.Version) {
			valid = append(valid, r)
		}
	}
	slices.SortFunc(valid, func(x, y Release) int { return Compare(x.Version, y.Version) })

	for i := 1; i < len(valid); i++ {
		if Compare(valid[i].Version, valid[i-1].Version) == 0 {
			return valid[i-1], valid[i], true
		}
	}
	return Release{}, Release{}, false
}

// reports whether [Release] `a` sorts before `b`
func less(a, b Release) bool {
	if a.ReleasedOn != b.ReleasedOn {
		// Dates are YYYY-MM-DD, so string order is date order.
		return a.ReleasedOn > b.ReleasedOn
	}
	return Compare(a.Version, b.Version) > 0
}
