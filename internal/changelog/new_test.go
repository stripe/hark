package changelog

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/hark/changefile"
)

// fakeFinder stands in for the gh CLI, so no test shells out.
type fakeFinder struct {
	pr  *PullRequest
	err error
}

func (f fakeFinder) CurrentPR(context.Context) (*PullRequest, error) { return f.pr, f.err }

// newFixture is an empty repo and a clock stopped on a known date.
func newFixture(t *testing.T) (afero.Fs, *bytes.Buffer, Options) {
	t.Helper()

	fs := afero.NewMemMapFs()
	var out bytes.Buffer
	opts := Options{
		Fs:  fs,
		Out: &out,
		Now: func() time.Time { return time.Date(2026, 9, 9, 12, 0, 0, 0, time.Local) },
	}
	return fs, &out, opts
}

// testUser stands in for $USER, so no expected filename depends on who runs the tests.
const testUser = "xavdid"

// shellUser is a [NewOptions.LookupUser] that reports name.
func shellUser(name string) func() string {
	return func() string { return name }
}

// newChange runs New with the ambient dependencies faked out. A test that forgets to
// supply them gets a finder reporting no pull request and a fixed $USER, rather than
// shelling out to the real gh or reading whoever's environment.
func newChange(t *testing.T, opts Options, draft changefile.Changefile, newOpts NewOptions) (string, error) {
	t.Helper()

	if newOpts.PRs == nil {
		newOpts.PRs = fakeFinder{}
	}
	if newOpts.LookupUser == nil {
		newOpts.LookupUser = shellUser(testUser)
	}
	return New(context.Background(), opts, draft, newOpts)
}

// read parses the changefile at path.
func read(t *testing.T, fs afero.Fs, path string) *changefile.Changefile {
	t.Helper()

	cf, err := changefile.ReadFile(fs, path)
	require.NoError(t, err)
	return cf
}

func TestNew_SeedsFromThePullRequest(t *testing.T) {
	fs, _, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{}, NewOptions{
		PRs: fakeFinder{pr: &PullRequest{
			URL:      "https://github.com/stripe/stripe-go/pull/123",
			Title:    "Add support for widgets",
			JiraTags: []string{"DEVSDK-456"},
		}},
		LookupUser: shellUser(testUser),
	})
	require.NoError(t, err)

	// The pull request supplies the title, but not the slug: nothing derives one, so
	// it stays FIXME until renamed. The user comes from the shell either way.
	assert.Equal(t, ".hark/changes/2026-09-09_xavdid_FIXME.change.md", path)

	cf := read(t, fs, path)
	assert.Equal(t, "Add support for widgets", cf.Title)
	assert.Equal(t, "https://github.com/stripe/stripe-go/pull/123", cf.PRUrl)
	assert.Equal(t, []string{"DEVSDK-456"}, cf.JiraTicketsClosed)
}

// The filename attributes the change to the shell user, not to whoever opened the
// pull request. On a Stripe laptop that is already the spelling we want, where a
// GitHub login would need "-stripe" trimmed off it first.
func TestNew_NamesTheFileAfterTheShellUser(t *testing.T) {
	_, _, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{}, NewOptions{
		Slug:       "add-widgets",
		PRs:        fakeFinder{pr: &PullRequest{Title: "Add widgets"}},
		LookupUser: shellUser("anniel"),
	})
	require.NoError(t, err)

	assert.Equal(t, ".hark/changes/2026-09-09_anniel_add-widgets.change.md", path)
}

// A container may have no $USER at all, which is not a reason to refuse to write the
// changefile.
func TestNew_FallsBackWhenTheShellHasNoUser(t *testing.T) {
	_, _, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{Title: "Add widgets"}, NewOptions{
		Slug:       "add-widgets",
		LookupUser: shellUser(""),
	})
	require.NoError(t, err)

	assert.Equal(t, ".hark/changes/2026-09-09_unknown_add-widgets.change.md", path)
}

// The advice keys off what hark had to invent, not off what the values ended up being.
// An author whose shell user happens to be the fallback spelling supplied it themselves,
// so there is nothing for them to fix.
func TestNew_NoAdviceWhenTheAuthorSuppliedTheDefaultSpelling(t *testing.T) {
	_, out, opts := newFixture(t)

	_, err := newChange(t, opts, changefile.Changefile{
		Title: changefile.UnknownUser,
		PRUrl: "https://github.com/stripe/stripe-go/pull/1",
	}, NewOptions{Slug: "add-widgets", LookupUser: shellUser(changefile.UnknownUser)})
	require.NoError(t, err)

	assert.NotContains(t, out.String(), "$USER was empty")
	assert.NotContains(t, out.String(), "replace the title")
	assert.NotContains(t, out.String(), "rename it")
}

// Fields passed explicitly were a deliberate choice, so the pull request must not
// overwrite them.
func TestNew_ExplicitFieldsBeatThePullRequest(t *testing.T) {
	fs, _, opts := newFixture(t)

	draft := changefile.Changefile{
		Title:             "A better description",
		PRUrl:             "https://github.com/stripe/stripe-go/pull/999",
		JiraTicketsClosed: []string{"DEVSDK-1"},
		Section:           "Added",
	}
	path, err := newChange(t, opts, draft, NewOptions{
		PRs: fakeFinder{pr: &PullRequest{
			URL:      "https://github.com/stripe/stripe-go/pull/123",
			Title:    "Add support for widgets",
			JiraTags: []string{"DEVSDK-456"},
		}},
	})
	require.NoError(t, err)

	cf := read(t, fs, path)
	assert.Equal(t, "A better description", cf.Title)
	assert.Equal(t, "https://github.com/stripe/stripe-go/pull/999", cf.PRUrl)
	assert.Equal(t, []string{"DEVSDK-1"}, cf.JiraTicketsClosed)
	assert.Equal(t, "Added", cf.Section)
}

// A changefile is usually written before the pull request exists, so there being
// none is ordinary rather than a failure.
func TestNew_WithoutAPullRequest(t *testing.T) {
	fs, _, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{}, NewOptions{
		Slug:       "add-widgets",
		PRs:        fakeFinder{},
		LookupUser: shellUser(testUser),
	})
	require.NoError(t, err)

	assert.Equal(t, ".hark/changes/2026-09-09_xavdid_add-widgets.change.md", path)

	// The slug names the file, but it is not a description of the change, so the title
	// stays a placeholder for the author to replace.
	cf := read(t, fs, path)
	assert.Equal(t, placeholderTitle, cf.Title)
	assert.Empty(t, cf.PRUrl)
}

// Neither a missing gh nor a logged-out one should stop someone mid-commit.
func TestNew_LookupFailureIsNotFatal(t *testing.T) {
	fs, out, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{Title: "Add widgets"}, NewOptions{
		PRs: fakeFinder{err: errors.New("gh: could not find any commits between master and HEAD")},
	})
	require.NoError(t, err)

	exists, err := afero.Exists(fs, path)
	require.NoError(t, err)
	assert.True(t, exists)

	// But the author should be told why their pr_url is empty.
	assert.Contains(t, out.String(), "could not find a pull request")
	assert.Contains(t, out.String(), "continuing without one")
}

// With no title and no slug there is nothing to describe the change, so the file
// says so rather than being named something meaningless.
func TestNew_PlaceholderTitleWhenNothingIsKnown(t *testing.T) {
	fs, _, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{}, NewOptions{})
	require.NoError(t, err)

	assert.Equal(t, placeholderTitle, read(t, fs, path).Title)
}

// A caller that passed both a title and a pull request URL has supplied everything the
// lookup fills, so there is nothing to ask gh for.
func TestNew_NoLookupWhenNothingIsMissing(t *testing.T) {
	_, out, opts := newFixture(t)

	// A finder that fails the test if it is consulted.
	finder := fakeFinder{err: errors.New("should not have been called")}

	_, err := newChange(t, opts, changefile.Changefile{
		Title: "Add widgets",
		PRUrl: "https://github.com/stripe/stripe-go/pull/123",
	}, NewOptions{Slug: "add-widgets", PRs: finder})
	require.NoError(t, err)

	// And no note about a pull request the caller never asked us to find.
	assert.NotContains(t, out.String(), "could not find a pull request")
}

// A title on its own is not everything: the pull request URL is still worth looking up.
func TestNew_LooksUpWhenOnlyTheTitleIsKnown(t *testing.T) {
	fs, _, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{Title: "A better description"}, NewOptions{
		Slug: "add-widgets",
		PRs:  fakeFinder{pr: &PullRequest{URL: "https://github.com/stripe/stripe-go/pull/123", Title: "Add widgets"}},
	})
	require.NoError(t, err)

	cf := read(t, fs, path)
	assert.Equal(t, "A better description", cf.Title, "the explicit title still wins")
	assert.Equal(t, "https://github.com/stripe/stripe-go/pull/123", cf.PRUrl)
}

// With no slug given the name carries FIXME, which `hark validate` rejects — the
// author is told to rename it rather than handed a phrase generated from their title.
func TestNew_DefaultsTheSlugToFixme(t *testing.T) {
	_, out, opts := newFixture(t)

	path, err := newChange(t, opts,
		changefile.Changefile{Title: "⚠️ Removed the Orders resource"},
		NewOptions{LookupUser: shellUser(testUser)})
	require.NoError(t, err)

	assert.Equal(t, ".hark/changes/2026-09-09_xavdid_FIXME.change.md", path)
	assert.NotEmpty(t, changefile.ValidateName(path), "the default name should not validate")
	assert.Contains(t, out.String(), "rename it")
}

// An explicit slug is used as given, and is the way to get a valid name in one step.
func TestNew_UsesAnExplicitSlug(t *testing.T) {
	_, out, opts := newFixture(t)

	path, err := newChange(t, opts,
		changefile.Changefile{Title: "⚠️ Removed the Orders resource"},
		NewOptions{Slug: "removed-orders-resource", LookupUser: shellUser(testUser)})
	require.NoError(t, err)

	assert.Equal(t, ".hark/changes/2026-09-09_xavdid_removed-orders-resource.change.md", path)
	assert.Empty(t, changefile.ValidateName(path))
	assert.NotContains(t, out.String(), "rename it")
}

// The date leading the filename is today's, read from the same clock everything else uses.
func TestNew_DatesTheNameToday(t *testing.T) {
	_, _, opts := newFixture(t)

	path, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets"}, NewOptions{Slug: "add-widgets"})
	require.NoError(t, err)

	assert.Equal(t, ".hark/changes/2026-09-09_xavdid_add-widgets.change.md", path)
	assert.Empty(t, changefile.ValidateName(path))
}

// A slug is one segment of a filename, so anything with a separator in it would land
// somewhere other than where the name says (including outside .hark/changes entirely,
// where nothing would ever read it). The charset rule is what catches these now, before
// the name is ever joined to a directory.
func TestNew_RefusesASlugWithPathSeparators(t *testing.T) {
	for _, slug := range []string{"a/b", "../other", "/../../other", "nested/dir/change", "../"} {
		t.Run(slug, func(t *testing.T) {
			fs, _, opts := newFixture(t)

			_, err := newChange(t, opts,
				changefile.Changefile{Title: "Add widgets"}, NewOptions{Slug: slug})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "letters, numbers, and hyphens")

			// Nothing was written, in .hark/changes or anywhere above it.
			for _, dir := range []string{changesFixtureDir, ".hark", ".", ".."} {
				entries, err := afero.ReadDir(fs, dir)
				if err != nil {
					continue // the directory was never created, which is just as empty
				}
				for _, e := range entries {
					assert.NotContains(t, e.Name(), changefile.Extension, "wrote %s/%s", dir, e.Name())
				}
			}
		})
	}
}

// The slug goes straight into a filename, so it's held to the same charset the name is:
// caught here, while the author is still at the terminal, rather than by CI later.
func TestNew_RefusesAnUnusableSlug(t *testing.T) {
	for _, slug := range []string{"add widgets", "add_widgets", "add.widgets", "add!"} {
		t.Run(slug, func(t *testing.T) {
			fs, _, opts := newFixture(t)

			_, err := newChange(t, opts,
				changefile.Changefile{Title: "Add widgets"}, NewOptions{Slug: slug})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "letters, numbers, and hyphens")

			exists, err := afero.DirExists(fs, changesFixtureDir)
			require.NoError(t, err)
			assert.False(t, exists, "wrote something before rejecting the slug")
		})
	}
}

// Rejecting a bad slug happens before the pull request lookup, since a name that can't
// be written makes anything the lookup found moot.
func TestNew_RefusesAnUnusableSlugBeforeLookingUpThePR(t *testing.T) {
	_, out, opts := newFixture(t)

	// a draft with no title or URL is what would otherwise send New to the finder
	_, err := newChange(t, opts, changefile.Changefile{},
		NewOptions{Slug: "add widgets", PRs: fakeFinder{err: errors.New("gh is unavailable")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "letters, numbers, and hyphens")

	// the finder reports failure by printing, so silence means it was never consulted
	assert.Empty(t, out.String())
}

// A user, which automation passes and the shell otherwise supplies, is a segment of the
// same name and is held to the same rule.
func TestNew_RefusesAUserWithPathSeparators(t *testing.T) {
	_, _, opts := newFixture(t)

	_, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets"},
		NewOptions{Slug: "add-widgets", User: "../../other"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

// The same person, day and slug twice is refused rather than written alongside: the
// first file is either the change already written or is named too vaguely to tell
// them apart, and both are better fixed than duplicated.
func TestNew_RefusesToOverwrite(t *testing.T) {
	fs, _, opts := newFixture(t)

	first, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets"}, NewOptions{Slug: "add-widgets"})
	require.NoError(t, err)

	_, err = newChange(t, opts,
		changefile.Changefile{Title: "Add widgets again"}, NewOptions{Slug: "add-widgets"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// The first file is left exactly as it was.
	assert.Equal(t, "Add widgets", read(t, fs, first).Title)

	// And nothing was written beside it.
	entries, err := afero.ReadDir(fs, changesFixtureDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// The default slug collides with itself, so a second unnamed changefile in a day is
// refused too — the first one wants naming before another is added.
func TestNew_RefusesASecondFixme(t *testing.T) {
	_, _, opts := newFixture(t)

	_, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets"}, NewOptions{})
	require.NoError(t, err)

	_, err = newChange(t, opts,
		changefile.Changefile{Title: "Fix retries"}, NewOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), changefile.FixmeSlug)
}

// Every name New produces from an explicit slug has to be one ValidateName accepts,
// including the collision suffix. (Without a slug it deliberately produces one that
// does not; see TestNew_DefaultsTheSlugToFixme.)
func TestNew_ProducesValidNames(t *testing.T) {
	_, _, opts := newFixture(t)

	for _, slug := range []string{"add-widgets", "fix-retries"} {
		path, err := newChange(t, opts,
			changefile.Changefile{Title: "Add widgets"},
			NewOptions{Slug: slug})
		require.NoError(t, err)
		assert.Empty(t, changefile.ValidateName(path))
	}
}

// New does not validate what it writes. A new changefile is a starting point and is
// usually invalid anyway, so a malformed field is written as given and left for `hark
// validate` — losing the invocation would be worse than writing a file to edit.
func TestNew_WritesADraftItWouldNotValidate(t *testing.T) {
	fs, _, opts := newFixture(t)

	path, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets", PRUrl: "not-a-url"},
		NewOptions{Slug: "add-widgets", LookupUser: shellUser(testUser)})
	require.NoError(t, err)

	assert.Equal(t, "not-a-url", read(t, fs, path).PRUrl)

	// But it is a problem, and validate is what says so.
	errs := read(t, fs, path).Validate()
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "pr_url")
}

// The date in the filename comes from the local clock, so a change written late in
// the evening is not dated tomorrow.
func TestNew_DateIsLocal(t *testing.T) {
	_, _, opts := newFixture(t)
	opts.Now = func() time.Time { return time.Date(2026, 9, 9, 23, 45, 0, 0, time.Local) }

	path, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets"}, NewOptions{})
	require.NoError(t, err)

	assert.Contains(t, path, "2026-09-09_")
}

// A changefile with nothing to describe the change yet gets a prompt for the author.
func TestNew_SeedsTheBodyWithAPrompt(t *testing.T) {
	fs, _, opts := newFixture(t)

	path, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets"}, NewOptions{})
	require.NoError(t, err)

	assert.Equal(t, bodyTemplate, read(t, fs, path).Body)
}

// The prompt has to be nothing but a comment, whatever it says, so an author who never
// fills it in doesn't publish it. This is what the wording is free to change under.
func TestNew_BodyTemplateIsEntirelyAComment(t *testing.T) {
	assert.Empty(t, strings.TrimSpace(stripHTMLComments(bodyTemplate)))
}

// A body that was supplied is the explanation; the prompt would only be in the way.
func TestNew_LeavesASuppliedBodyAlone(t *testing.T) {
	fs, _, opts := newFixture(t)

	path, err := newChange(t, opts,
		changefile.Changefile{Title: "Add widgets", Body: "Some detail."}, NewOptions{})
	require.NoError(t, err)

	assert.Equal(t, "Some detail.", read(t, fs, path).Body)
}

func TestJiraTags(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   []string
	}{
		{
			name:   "the branch is the ticket",
			branch: "DEVSDK-123",
			want:   []string{"DEVSDK-123"},
		},
		{
			name:   "with the work described after it",
			branch: "DEVSDK-456-add-widgets",
			want:   []string{"DEVSDK-456"},
		},
		{
			name:   "under a user prefix",
			branch: "xavdid/DEVSDK-456-add-widgets",
			want:   []string{"DEVSDK-456"},
		},
		{
			// RUN_DEVSDK is its own board, so the underscore is part of the key rather
			// than a prefix on DEVSDK.
			name:   "a board name containing an underscore",
			branch: "RUN_DEVSDK-2972",
			want:   []string{"RUN_DEVSDK-2972"},
		},
		{
			name:   "that board under a user prefix",
			branch: "xavdid/RUN_DEVSDK-2741",
			want:   []string{"RUN_DEVSDK-2741"},
		},
		{
			name:   "several, deduplicated, in the order they appear",
			branch: "DEVSDK-2-and-DEVSDK-1-and-DEVSDK-1-again",
			want:   []string{"DEVSDK-2", "DEVSDK-1"},
		},
		{
			name:   "nothing to find",
			branch: "xavdid/add-widgets",
			want:   nil,
		},
		{
			name:   "not fooled by a lowercase project or a bare number",
			branch: "fixes-issue-12-and-pull-34",
			want:   nil,
		},
		{
			// The reason the branch is the only source: a generated body is full of
			// enum values that look exactly like ticket references.
			name:   "an API diff is not a source of tickets",
			branch: "* Add `PAYMENT_METHOD-1`; remove CHECKOUT_SESSION-99",
			want:   []string{"PAYMENT_METHOD-1", "CHECKOUT_SESSION-99"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jiraTags(tt.branch))
		})
	}
}

// A body is the nested detail under a change's bullet. Callers that generate one
// rather than typing it — the code generator writes the API diff to a file for the
// pull request description — point at that file.
func TestNew_ReadsTheBodyFromAFile(t *testing.T) {
	fs, _, opts := newFixture(t)

	body := "* Add support for `widgets`\n* Remove support for `gizmos`\n"
	require.NoError(t, afero.WriteFile(fs, "diff.txt", []byte(body), 0o644))

	path, err := newChange(t, opts, changefile.Changefile{Title: "Update generated code"}, NewOptions{
		BodyPath: "diff.txt",
		PRs:      fakeFinder{},
	})
	require.NoError(t, err)

	cf := read(t, fs, path)
	assert.Equal(t, "* Add support for `widgets`\n* Remove support for `gizmos`", cf.Body)
}

// The draft is what the caller asked for explicitly, so it wins — the same way it
// does over the pull request.
func TestNew_ExplicitBodyBeatsTheFile(t *testing.T) {
	fs, _, opts := newFixture(t)
	require.NoError(t, afero.WriteFile(fs, "diff.txt", []byte("from the file"), 0o644))

	draft := changefile.Changefile{Title: "Add widgets", Body: "written by hand"}
	path, err := newChange(t, opts, draft, NewOptions{BodyPath: "diff.txt", PRs: fakeFinder{}})
	require.NoError(t, err)

	assert.Equal(t, "written by hand", read(t, fs, path).Body)
}

// Unlike a missing pull request, a body file that was named and is not there is a
// mistake worth stopping for: the caller meant to include content.
func TestNew_MissingBodyFileIsAnError(t *testing.T) {
	_, _, opts := newFixture(t)

	_, err := newChange(t, opts, changefile.Changefile{Title: "Add widgets"}, NewOptions{
		BodyPath: "nope.txt",
		PRs:      fakeFinder{},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope.txt")
}

// Automation knows who it is, and a runner's shell user says nothing about who
// queued the work, so an explicit one wins.
func TestNew_ExplicitUserBeatsTheShellUser(t *testing.T) {
	_, _, opts := newFixture(t)

	path, err := newChange(t, opts, changefile.Changefile{}, NewOptions{
		Slug:       "update-generated-code",
		User:       "stripe-openapi",
		PRs:        fakeFinder{pr: &PullRequest{Title: "Update generated code"}},
		LookupUser: shellUser("runner"),
	})
	require.NoError(t, err)

	assert.Equal(t, ".hark/changes/2026-09-09_stripe-openapi_update-generated-code.change.md", path)
}

// The combination the code generator uses: every field explicit, no lookup at all,
// so no gh call, no token, and no Jira tags harvested out of an API diff.
func TestNew_FullyExplicitNeedsNoLookup(t *testing.T) {
	fs, _, opts := newFixture(t)
	require.NoError(t, afero.WriteFile(fs, "diff.txt", []byte("* Add support for `widgets`\n"), 0o644))

	draft := changefile.Changefile{
		Title:             "Update generated code",
		PRUrl:             "https://github.com/stripe/stripe-go/pull/2222",
		IsStripeAPIChange: true,
	}
	path, err := newChange(t, opts, draft, NewOptions{
		User:     "stripe-openapi",
		Slug:     "update-generated-code",
		BodyPath: "diff.txt",
		// No PRs finder: a title and a pr_url leave nothing to look up.
	})
	require.NoError(t, err)

	// Automation passes a slug like everything else, since nothing generates one.
	assert.Equal(t, ".hark/changes/2026-09-09_stripe-openapi_update-generated-code.change.md", path)

	cf := read(t, fs, path)
	assert.Equal(t, "Update generated code", cf.Title)
	assert.Equal(t, "https://github.com/stripe/stripe-go/pull/2222", cf.PRUrl)
	assert.True(t, cf.IsStripeAPIChange)
	assert.Empty(t, cf.JiraTicketsClosed)
	assert.Equal(t, "* Add support for `widgets`", cf.Body)
}
