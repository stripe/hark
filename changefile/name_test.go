package changefile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	assert.Equal(t, "2026-09-09_xavdid_add-widgets.change.md", Name("2026-09-09", "xavdid", "add-widgets"))
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
			// uniquePath appends a counter, which is a fourth part.
			name:     "a collision suffix",
			path:     "2026-09-09_xavdid_add-widgets_2.change.md",
			wantErrs: 0,
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
			path:     "xavdid_add_widgets.change.md",
			wantErrs: 1,
		},
		{
			name:     "an unparseable date",
			path:     "2026-13-45_xavdid_add-widgets.change.md",
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
	for _, path := range []string{
		"2026-09-09_xavdid_" + FixmeSlug + ".change.md",
		// A second one the same day picks up a collision suffix; still unnamed.
		"2026-09-09_xavdid_" + FixmeSlug + "_2.change.md",
	} {
		errs := ValidateName(path)
		require.Len(t, errs, 1, path)
		assert.Contains(t, errs[0].Error(), FixmeSlug)
	}

	// A real slug that merely starts with the same letters is fine.
	assert.Empty(t, ValidateName("2026-09-09_xavdid_FIXMEup-the-docs.change.md"))
}
