// Package `changelog` is responsible for the high-level operations behind hark's CLI commands:
// creating changefiles, validating them, compiling them into a CHANGELOG.md, and cutting a release.
//
// It's intentionally not very configurable- we expect the SDKs to conform to a specific shape:
//
//	stripe-<lang>/
//	├── .hark/
//	│   ├── releases.json
//	│   ├── changes/
//	│   │   ├── 2026-01-22_xavdid_some-thing.md
//	│   │   ├── 2026-03-22_xavdid_an-upcoming-feature.md
//	│   │   └── 2026-06-22_xavdid_neato.md
//	│   ├── intros/
//	│   │   └── intro-1.2.3.md
//	│   └── migration-guides/
//	│       └── v2.md
//	└── CHANGELOG.md
package changelog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"

	"github.com/stripe/hark/changefile"
	"github.com/stripe/hark/releases"
)

const (
	HarkDir            = ".hark"
	ChangesDir         = "changes"
	IntrosDir          = "intros"
	ReleasesName       = "releases.json"
	Filename           = "CHANGELOG.md"
	MigrationGuidesDir = "migration-guides"

	// where to look for our filesystem structure. Repo root for the CLI, but a subfolder when we're working internal to Stripe
	DefaultRoot = "."
)

type Options struct {
	changefile.ReadOptions

	Fs   afero.Fs
	Out  io.Writer
	Now  func() time.Time
	Root string
}

func (o Options) withDefaults() Options {
	if o.Fs == nil {
		o.Fs = afero.NewOsFs()
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Root == "" {
		o.Root = DefaultRoot
	}
	return o
}

func (o Options) changesDir() string {
	return filepath.Join(o.Root, HarkDir, ChangesDir)
}

const introPrefix = "intro-"

// given a version, get the intro filename
func IntroName(version string) string {
	return introPrefix + version + ".md"
}

// given a filename, get the version it represents (if any). The version portion has to pass [releases.IsVersionValid] since it'll never be found by a version otherwise.
func IntroVersion(name string) (string, bool) {
	version, ok := strings.CutPrefix(name, introPrefix)
	if !ok {
		return "", false
	}

	version, ok = strings.CutSuffix(version, ".md")
	if !ok || !releases.IsVersionValid(version) {
		return "", false
	}
	return version, true
}

// introsDir is the directory holding this repo's release introductions.
func (o Options) introsDir() string {
	return filepath.Join(o.Root, HarkDir, IntrosDir)
}

// introPath is where a release's introduction lives, whether or not it is there.
func (o Options) introPath(version string) string {
	return filepath.Join(o.introsDir(), IntroName(version))
}

// MigrationGuideName is the file holding the upgrade instructions for a major version.
// One guide covers a whole major, so it is named for the major alone rather than for the
// release that introduced it.
func MigrationGuideName(major int) string {
	return fmt.Sprintf("v%d.md", major)
}

// migrationGuidesDir is the directory holding this repo's migration guides.
func (o Options) migrationGuidesDir() string {
	return filepath.Join(o.Root, HarkDir, MigrationGuidesDir)
}

// migrationGuidePath is where a major version's migration guide lives (though the path may be empty)
func (o Options) migrationGuidePath(major int) string {
	return filepath.Join(o.migrationGuidesDir(), MigrationGuideName(major))
}

// releasesPath is this repo's releases file.
func (o Options) releasesPath() string {
	return filepath.Join(o.Root, HarkDir, ReleasesName)
}

// changelogPath is this repo's compiled changelog.
func (o Options) changelogPath() string {
	return filepath.Join(o.Root, Filename)
}

// gets the current local date, formatted correctly
func (o Options) today() string {
	return o.Now().Format(changefile.DateFormat)
}
