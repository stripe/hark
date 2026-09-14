package changefile

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Changefiles are named `{date}_{user}_{slug}.change.md`. This convention mostly exists to give a good shot at generating a unique filename.
//
// That said, we _do_ sort the eventual changelog items by filename date ascending.
const (
	NameSep = "_"

	// placeholder username when a change cannot be attributed to a GitHub login.
	UnknownUser = "unknown"

	// enforces the shape of the leading segment
	DateFormat = time.DateOnly

	// This is the default slug if one isn't provided
	// It deliberately fails validation in [ValidateName] (so it's replaced with a real value before merge) but it's a fine placeholder
	FixmeSlug = "FIXME"
)

// Build a full filename based on component segments.
func Name(date, user, slug string) string {
	return strings.Join([]string{date, user, slug}, NameSep) + Extension
}

// validates the structure of a filename.
// It reports every problem it finds rather than stopping at the first.
// An empty slice means the changefile is valid.
//
// Extra care is taken to validate the date portion, since we use it to sort bullets in a changefile.
func ValidateName(path string) []error {
	var errs []error

	name := filepath.Base(path)
	if !strings.HasSuffix(name, Extension) {
		errs = append(errs, fmt.Errorf("name does not end in %s", Extension))
		return errs
	}

	base := strings.TrimSuffix(name, Extension)
	parts := strings.Split(base, NameSep)
	if len(parts) < 3 {
		errs = append(errs, fmt.Errorf("filename should follow format {date}%s{user}%s{slug}%s, got %q",
			NameSep, NameSep, Extension, name))
		return errs
	}

	if _, err := time.Parse(DateFormat, parts[0]); err != nil {
		errs = append(errs, fmt.Errorf("name should lead with an ISO date, like %s; got %q", DateFormat, parts[0]))
	}

	if slug := strings.Join(parts[2:], NameSep); slug == FixmeSlug ||
		strings.HasPrefix(slug, FixmeSlug+NameSep) {
		errs = append(errs, fmt.Errorf("changefile named %q still has the %s placeholder; rename it to a slug that represents the change",
			name, FixmeSlug))
	}

	return errs
}
