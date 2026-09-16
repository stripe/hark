package changefile

import (
	"fmt"
	"path/filepath"
	"regexp"
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

// slugs should be alphanumeric and can't contain underscores (since those are our file separator)
var slugRegex = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// Build a full filename based on component segments.
func Name(date, user, slug string) string {
	return strings.Join([]string{date, user, slug}, NameSep) + Extension
}

func ValidateSlug(slug string) error {
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("slug %q may only contain letters, numbers, and hyphens", slug)
	}
	return nil
}

func ValidateDate(date string) error {
	parsed, err := time.Parse(DateFormat, date)
	if err != nil || parsed.Format(DateFormat) != date {
		return fmt.Errorf("date %q should be an ISO date, like %s", date, DateFormat)
	}
	return nil
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

	if err := ValidateDate(parts[0]); err != nil {
		errs = append(errs, fmt.Errorf("name should lead with an ISO date, like %s; got %q", DateFormat, parts[0]))
	}

	// everything past the user is the slug, so an underscore in it shows up here as extra segments
	slug := strings.Join(parts[2:], NameSep)
	if err := ValidateSlug(slug); err != nil {
		errs = append(errs, err)
	}

	if slug == FixmeSlug || strings.HasPrefix(slug, FixmeSlug+NameSep) {
		errs = append(errs, fmt.Errorf("changefile named %q still has the %s placeholder; rename it to a slug that represents the change",
			name, FixmeSlug))
	}

	return errs
}
