package changelog

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

const warningMarker = "⚠️"

const unreleasedHeading = "Unreleased"

// This is a basic ordering for existing `section` values.
//
// TODO(section-ordering) - Long-term we can probably rethink how we sort these, but it serves its purpose of minimizing the diff in the existing changelogs for now.
// simplest is probably a `key` in the changefile and then a `{key: 'something', title: 'Something cool'}[]` property in the release
var sectionOrder = []string{
	"⚠️ Breaking changes due to changes in the Stripe API",
	"⚠️ Other Breaking changes in the SDK",
	"Backwards incompatible changes",
	"Breaking change",
	"Breaking changes",
	"Version pinning",
	"Added",
	"Additions",
	"⚠️ Changed",
	"Changes",
	"Deprecated",
	"Deprecation",
	"⚠️ Removed",
	"⚠️ Renamed",
	"Other changes",
}

// sectionRanks indexes sectionOrder for lookup, keyed by canonical form.
var sectionRanks = func() map[string]int {
	m := make(map[string]int, len(sectionOrder))
	for i, s := range sectionOrder {
		m[canonicalSection(s)] = i
	}
	return m
}()

// whitespaceRun matches any run of whitespace, for collapsing before comparison.
var whitespaceRun = regexp.MustCompile(`\s+`)

// prNumberRegex pulls the pull request number off the end of a PR URL.
var prNumberRegex = regexp.MustCompile(`/(?:pull|issues)/(\d+)/?$`)

// listItemStartRegex matches a list-item at the start of a line (with optional leading padding)
var listItemStartRegex = regexp.MustCompile(`^ {0,3}(?:[-*+]|1[.)])[ \t]+\S`)

// groups everything we need to render a release
type releaseGroup struct {
	Release *releases.Release
	Changes []*changefile.Changefile
	Intro   string
	// PrevPinnedAPIVersion is the API version pinned by the release this one succeeded
	// by version (see [releases.File.Predecessors]), not the previous release by date.
	// It's used to decide if this release announces a change of API version;
	// see [releaseGroup.pinnedAPINotice].
	PrevPinnedAPIVersion string
}

const pinnedAPINoticeFormat = "This release changes the pinned API version to `%s`."

// calculates whether a given releaseBlock should announce a new pinned API version
func (g releaseGroup) pinnedAPINotice() string {
	if g.Release == nil {
		return "" // unreleased changes pin nothing
	}
	current := g.Release.PinnedAPIVersion
	if current == "" || g.PrevPinnedAPIVersion == "" || current == g.PrevPinnedAPIVersion {
		return ""
	}
	return fmt.Sprintf(pinnedAPINoticeFormat, current)
}

// preamble is everything rendered between a release's heading and its first bullet
func (g releaseGroup) preamble() string {
	parts := make([]string, 0, 2)
	if notice := g.pinnedAPINotice(); notice != "" {
		parts = append(parts, notice)
	}
	if intro := g.intro(); intro != "" {
		parts = append(parts, intro)
	}
	return strings.Join(parts, "\n\n")
}

// heading is the text of the group's `##` line.
func (g releaseGroup) heading() string {
	if g.Release == nil {
		return unreleasedHeading
	}
	if g.Release.ReleasedOn == "" {
		return g.Release.Version
	}
	return g.Release.Version + " - " + g.Release.ReleasedOn
}

// versionAnchor is a stable id we can use to link to a specific changelog header without knowing its release date
func versionAnchor(version string) string {
	return strings.ReplaceAll(version, ".", "-")
}

func (g releaseGroup) intro() string {
	return strings.TrimSpace(g.Intro)
}

// A section header and its changes. Changes without a `section` are stored in a struct with an empty `Section`.
type changeSection struct {
	Header  string
	Changes []*changefile.Changefile
}

// normalizes a section title for comparison so we can use consistent sorting on them.
//
// TODO(section-ordering): we should just cleanup old section names rather than do all this munging
func canonicalSection(section string) string {
	section = whitespaceRun.ReplaceAllString(strings.TrimSpace(section), " ")
	section = strings.TrimSpace(strings.TrimSuffix(section, ":"))
	return strings.ToLower(section)
}

func sectionRank(section string) int {
	if rank, ok := sectionRanks[canonicalSection(section)]; ok {
		return rank
	}
	return len(sectionOrder)
}

// renders the markdown link that leads a change's bullet,
// e.g. `[#123](https://github.com/stripe/stripe-go/pull/123)`.
func markdownPrLink(link string) (result string, ok bool) {
	m := prNumberRegex.FindStringSubmatch(link)
	if m == nil {
		return "", false
	}
	return fmt.Sprintf("[#%s](%s)", m[1], link), true
}

// groupChanges buckets changes by the version they were released in, returning
// one group per entry in released plus, when there are unreleased changes, a
// leading group for those.
//
// Every released version gets a group even when nothing shipped in it, so the
// compiled changelog is a complete record of releases.
func groupChanges(changes []*changefile.Changefile, releaseFile *releases.File) ([]releaseGroup, error) {
	byVersion := make(map[string][]*changefile.Changefile)
	var unreleased []*changefile.Changefile
	var untrackedVersions []string

	for _, c := range changes {
		if c.ReleasedInVersion == "" {
			unreleased = append(unreleased, c)
			continue
		}
		if releaseFile.Find(c.ReleasedInVersion) == nil {
			untrackedVersions = append(untrackedVersions, fmt.Sprintf("%s: released_in_version %q is not in the releases file", c.SourcePath, c.ReleasedInVersion))
			continue
		}
		byVersion[c.ReleasedInVersion] = append(byVersion[c.ReleasedInVersion], c)
	}

	if len(untrackedVersions) > 0 {
		sort.Strings(untrackedVersions)
		return nil, fmt.Errorf("unknown versions:\n  %s", strings.Join(untrackedVersions, "\n  "))
	}

	groups := make([]releaseGroup, 0, len(releaseFile.Releases)+1)
	if len(unreleased) > 0 {
		groups = append(groups, releaseGroup{Changes: sortChanges(unreleased)})
	}

	predecessors := releaseFile.Predecessors()

	for i := range releaseFile.Releases {
		r := &releaseFile.Releases[i]
		var prevPinned string
		if prev := predecessors[r.Version]; prev != nil {
			prevPinned = prev.PinnedAPIVersion
		}
		groups = append(groups, releaseGroup{
			Release:              r,
			Changes:              sortChanges(byVersion[r.Version]),
			PrevPinnedAPIVersion: prevPinned,
		})
	}

	return groups, nil
}

// orders changes within a release based on their filename ascending (which uses date, then username, then slug).
// Changes that are entirely driven from the spec are sorted to the bottom
//
// TODO(release-ordering): we might want better control over how these are ordered
// best option is probably a `precedence` or `priority` int in the changefile so we can mark changes are "put this high in the list"
func sortChanges(changes []*changefile.Changefile) []*changefile.Changefile {
	sorted := slices.Clone(changes)
	sort.SliceStable(sorted, func(i, j int) bool {
		if a, b := sorted[i].IsStripeAPIChange, sorted[j].IsStripeAPIChange; a != b {
			return !a
		}
		return sorted[i].SourcePath < sorted[j].SourcePath
	})
	return sorted
}

// splits a group's changes into the un-sectioned group followed by 0+ `sectionWithChanges`s
func (g releaseGroup) sections() []changeSection {
	bySection := make(map[string][]*changefile.Changefile)
	var sectionNames []string

	for _, c := range g.Changes {
		key := canonicalSection(c.Section)
		if _, seen := bySection[key]; !seen {
			sectionNames = append(sectionNames, key)
		}
		bySection[key] = append(bySection[key], c)
	}

	// Sort sections by rank, then by title so unlisted sections have a stable order. The
	// unsectioned block has the empty key, which always sorts first.
	sort.SliceStable(sectionNames, func(i, j int) bool {
		a, b := sectionNames[i], sectionNames[j]
		if a == "" || b == "" {
			return a == ""
		}
		if ra, rb := sectionRank(a), sectionRank(b); ra != rb {
			return ra < rb
		}
		return a < b
	})

	sections := make([]changeSection, 0, len(sectionNames))
	for _, key := range sectionNames {
		changes := bySection[key]
		// Render the section as it was written, not as it was canonicalized.
		section := key
		if key != "" {
			section = strings.TrimSpace(changes[0].Section)
		}
		sections = append(sections, changeSection{Header: section, Changes: changes})
	}
	return sections
}

const generatedNotice = "<!--\n" +
	"THIS IS A GENERATED FILE. Any changes you make to it directly will be blown away.\n" +
	"Instead, edit a corresponding `.change.md` file and run `hark build`.\n" +
	"-->"

var sdkLanguages = []string{"java", "python", "ruby", "php", "go", "node", "dotnet"}

func sdkRepo(language string) string {
	if !slices.Contains(sdkLanguages, language) {
		return ""
	}
	return "stripe/stripe-" + language
}

// changelogRef names another branch's changelog: a link when the repository could be
// worked out, and plain prose when it could not.
func changelogRef(label, language string) string {
	repo := sdkRepo(language)
	if repo == "" {
		return label
	}
	return fmt.Sprintf("[%s](https://github.com/%s/blob/master/CHANGELOG.md)", label, repo)
}

// a standing note a prerelease channel's changelog opens with pointing readers to the GA changelog.
func channelNotice(channel, language string) string {
	var name string
	switch channel {
	case releases.ChannelBeta:
		name = "public preview"
	case releases.ChannelPrivatePreview:
		name = "private preview"
	default:
		return ""
	}

	// Both channels point at GA and only GA, because both branches take their changes
	// from master and neither takes them from the other.
	return fmt.Sprintf(
		"> This changelog only covers the **%s** releases. Each release builds on the most recent GA release; see those notes in %s.",
		name, changelogRef("the GA changelog", language))
}

// render writes the whole changelog: the generated-file notice, an h1 title, the
// channel's standing note if it has one, then each group separated by a blank line.
func render(w io.Writer, language, channel string, groups []releaseGroup) error {
	if _, err := fmt.Fprintf(w, "%s\n\n", generatedNotice); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "# Changelog\n"); err != nil {
		return err
	}
	if notice := channelNotice(channel, language); notice != "" {
		if _, err := fmt.Fprintf(w, "\n%s\n", notice); err != nil {
			return err
		}
	}
	for _, g := range groups {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := renderReleaseBlock(w, g); err != nil {
			return err
		}
	}
	return nil
}

// renderReleaseBlock writes one release: its heading, its preamble, its unsectioned
// changes, then each section.
func renderReleaseBlock(w io.Writer, g releaseGroup) error {
	// An invisible anchor, so we can deeplink to a release without knowing its release date
	var anchor string
	if g.Release != nil {
		anchor = fmt.Sprintf("<a id=%q></a>", versionAnchor(g.Release.Version))
	}

	if _, err := fmt.Fprintf(w, "## %s%s\n", anchor, g.heading()); err != nil {
		return err
	}

	preamble := g.preamble()
	if preamble != "" {
		if _, err := fmt.Fprintf(w, "%s\n", preamble); err != nil {
			return err
		}
	}

	for _, s := range g.sections() {
		switch {
		case s.Header != "":
			if _, err := fmt.Fprintf(w, "\n### %s\n", s.Header); err != nil {
				return err
			}
		case preamble != "":
			// Separate the prose from the bullets that follow it.
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}

		for _, c := range s.Changes {
			if err := renderChange(w, c); err != nil {
				return err
			}
		}
	}

	return nil
}

// renderChange writes one change as a top-level bullet, with its body nested
// underneath.
func renderChange(w io.Writer, c *changefile.Changefile) error {
	var b strings.Builder

	b.WriteString("* ")

	if c.Level() == changefile.SemverLevelMajor {
		b.WriteString(warningMarker)
		b.WriteString(" ")
	}

	prLink, prWellFormed := markdownPrLink(c.PRUrl)
	if prWellFormed {
		b.WriteString(prLink)
		b.WriteString(" ")
	}

	b.WriteString(strings.TrimSpace(c.Title))

	// A pr_url we cannot pull a number out of still belongs in the output.
	if c.PRUrl != "" && !prWellFormed {
		b.WriteString(" (")
		b.WriteString(c.PRUrl)
		b.WriteString(")")
	}

	if _, err := fmt.Fprintf(w, "%s\n", b.String()); err != nil {
		return err
	}

	if body := indentBody(c.Body); body != "" {
		if bodyNeedsBlankLine(c.Body) {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s\n", body); err != nil {
			return err
		}
	}
	return nil
}

// bodyNeedsBlankLine reports whether a body has to be separated from the parent bullet (so it gets its own block)
//
// Everything besides a list item needs a blank line proceeding it
func bodyNeedsBlankLine(body string) bool {
	firstLine, _, _ := strings.Cut(strings.TrimLeft(body, "\n"), "\n")
	return !listItemStartRegex.MatchString(firstLine)
}

// indentBody nests a change's body under its bullet by prefixing every non-blank
// line with leading space. Relative indentation within the body is preserved, so
// nested bullets and fenced code survive.
func indentBody(body string) string {
	body = strings.Trim(body, "\n")
	if body == "" {
		return ""
	}

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
