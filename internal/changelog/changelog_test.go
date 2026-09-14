package changelog

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/stripe/hark/changefile"
)

func TestWithDefaults(t *testing.T) {
	got := Options{}.withDefaults()

	assert.NotNil(t, got.Fs)
	assert.NotNil(t, got.Out)
	assert.NotNil(t, got.Now)
	assert.Equal(t, DefaultRoot, got.Root)
}

func TestWithDefaults_KeepsProvidedValues(t *testing.T) {
	fs := afero.NewMemMapFs()
	var out bytes.Buffer

	got := Options{
		Fs:          fs,
		Out:         &out,
		Root:        "checkouts/stripe-go",
		ReadOptions: changefile.ReadOptions{Workers: 3},
	}.withDefaults()

	assert.Same(t, fs, got.Fs)
	assert.Same(t, &out, got.Out)
	assert.Equal(t, "checkouts/stripe-go", got.Root)
	assert.Equal(t, 3, got.Workers)
}

// The layout is fixed, so the only thing that moves it is the repo root. A caller
// holding several checkouts relies on this to work through them one at a time.
func TestOptionsPaths(t *testing.T) {
	tests := []struct {
		name          string
		root          string
		changesDir    string
		releasesPath  string
		changelogPath string
	}{
		{
			name:          "the current directory",
			root:          "",
			changesDir:    ".hark/changes",
			releasesPath:  ".hark/releases.json",
			changelogPath: "CHANGELOG.md",
		},
		{
			name:          "another checkout",
			root:          "checkouts/stripe-go",
			changesDir:    "checkouts/stripe-go/.hark/changes",
			releasesPath:  "checkouts/stripe-go/.hark/releases.json",
			changelogPath: "checkouts/stripe-go/CHANGELOG.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{Root: tt.root}.withDefaults()

			assert.Equal(t, tt.changesDir, opts.changesDir())
			assert.Equal(t, tt.releasesPath, opts.releasesPath())
			assert.Equal(t, tt.changelogPath, opts.changelogPath())
		})
	}
}

// An intro is located purely by its name, so the two directions have to agree:
// whatever IntroName writes, IntroVersion has to read back.
func TestIntroName(t *testing.T) {
	assert.Equal(t, "intro-1.2.3.md", IntroName("1.2.3"))
	assert.Equal(t, ".hark/intros/intro-1.2.3.md", Options{}.withDefaults().introPath("1.2.3"))

	for _, version := range []string{"1.2.3", "10.0.0", "1.3.0-beta.1", "86.4.1"} {
		got, ok := IntroVersion(IntroName(version))
		assert.True(t, ok, "IntroVersion should read back %q", version)
		assert.Equal(t, version, got)
	}
}

func TestIntroVersion_RejectsNamesThatWouldNeverBeFound(t *testing.T) {
	for _, name := range []string{
		"1.2.3.md",       // no prefix
		"intro-1.2.3",    // no extension
		"intro-.md",      // no version
		"intro.md",       // neither
		"README.md",      // something else entirely
		"Intro-1.2.3.md", // wrong case

		// named after something no release could be called
		"intro-1.md",
		"intro-1.2.md",
		"intro-1.2.3.4.md",
		"intro-next.md",
		"intro-1.0.0-rc.1.md",
	} {
		_, ok := IntroVersion(name)
		assert.False(t, ok, "%q should not be accepted", name)
	}
}

// Dates come from the local clock rather than UTC, so a release cut late in the
// evening is dated the day the person cutting it would write.
func TestOptionsToday_IsLocal(t *testing.T) {
	lateEvening := time.Date(2026, 9, 9, 23, 30, 0, 0, time.Local)
	opts := Options{Now: func() time.Time { return lateEvening }}.withDefaults()

	assert.Equal(t, "2026-09-09", opts.today())
}
