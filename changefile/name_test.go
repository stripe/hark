package changefile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	assert.Equal(t, "2026-09-09_xavdid_add-widgets.change.md", Name("2026-09-09", "xavdid", "add-widgets"))
}

func TestValidateSlug(t *testing.T) {
	for _, slug := range []string{"add-widgets", "addwidgets", "add-2-widgets", "AddWidgets", "x", FixmeSlug} {
		assert.NoError(t, ValidateSlug(slug), slug)
	}

	for _, slug := range []string{"add widgets", "add_widgets", "add.widgets", "add/widgets", "añadir", ""} {
		assert.Error(t, ValidateSlug(slug), slug)
	}
}

func TestValidateDate(t *testing.T) {
	for _, date := range []string{"2026-09-09", "2026-01-01", "1999-12-31"} {
		assert.NoError(t, ValidateDate(date), date)
	}

	for _, date := range []string{
		"2026-13-45",  // no such month or day
		"2026-02-30",  // no such day in that month
		"2026-9-9",    // parseable, but sorts nowhere near the padded dates
		"2026-09-09 ", // a stray space is still a different filename
		"09-09-2026",  // the other conventional order
		"2026-09-09T00:00:00Z",
		"today",
		"",
	} {
		assert.Error(t, ValidateDate(date), date)
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantErrs int
	}{
		{
			name:     "a generated name",
			path:     ".hark/changes/2026-09-09_xavdid_add-widgets.change.md",
			wantErrs: 0,
		},
		{
			name:     "a login containing a hyphen",
			path:     "2026-09-09_anniel-stripe_add-widgets.change.md",
			wantErrs: 0,
		},
		{
			// the slug is everything past the user, so an underscore in it reads as a fourth segment
			name:     "an underscore in the slug",
			path:     "2026-09-09_xavdid_add_widgets.change.md",
			wantErrs: 1,
		},
		{
			// a name with a space in it is a chore to type at every shell that touches it
			name:     "a space in the slug",
			path:     "2026-09-09_xavdid_add widgets.change.md",
			wantErrs: 1,
		},
		{
			name:     "punctuation in the slug",
			path:     "2026-09-09_xavdid_add.widgets!.change.md",
			wantErrs: 1,
		},
		{
			name:     "an empty slug",
			path:     "2026-09-09_xavdid_.change.md",
			wantErrs: 1,
		},
		{
			name:     "wrong extension",
			path:     "2026-09-09_xavdid_add-widgets.md",
			wantErrs: 1,
		},
		{
			name:     "too few parts",
			path:     "add-widgets.change.md",
			wantErrs: 1,
		},
		{
			// The compiled changelog orders changes by filename, so a name without a
			// date sorts to the end of its release rather than where it belongs.
			name:     "no leading date",
			path:     "xavdid_anniel_add-widgets.change.md",
			wantErrs: 1,
		},
		{
			name:     "an unparseable date",
			path:     "2026-13-45_xavdid_add-widgets.change.md",
			wantErrs: 1,
		},
		{
			// it parses, but sorts before every padded date in the same month
			name:     "an unpadded date",
			path:     "2026-9-9_xavdid_add-widgets.change.md",
			wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Len(t, ValidateName(tt.path), tt.wantErrs)
		})
	}
}

// Every name Name produces has to be one ValidateName accepts, or `hark new`
// creates files that `hark validate` rejects for the wrong reason.
func TestValidateName_AcceptsWhatNameProduces(t *testing.T) {
	for _, slug := range []string{"add-widgets", "removed-orders-resource", "x"} {
		name := Name("2026-09-09", UnknownUser, slug)
		assert.Empty(t, ValidateName(name), "name %q from slug %q", name, slug)
	}
}

// The default slug is meant to fail, so that a changefile nobody has named keeps
// failing until someone names it.
func TestValidateName_RejectsTheFixmeSlug(t *testing.T) {
	path := "2026-09-09_xavdid_" + FixmeSlug + ".change.md"
	errs := ValidateName(path)
	require.Len(t, errs, 1, path)
	assert.Contains(t, errs[0].Error(), FixmeSlug)

	// A real slug that merely starts with the same letters is fine.
	assert.Empty(t, ValidateName("2026-09-09_xavdid_FIXMEup-the-docs.change.md"))
}
