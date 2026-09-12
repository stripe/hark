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
//	│   └── intros/
//	│       └── intro-1.2.3.md
//	└── CHANGELOG.md
package changelog

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"

	"github.com/stripe/hark/changefile"
)

const (
	HarkDir      = ".hark"
	ChangesDir   = "changes"
	IntrosDir    = "intros"
	ReleasesName = "releases.json"
	Filename     = "CHANGELOG.md"

	// where to look for our filesystem structure. Repo root for the CLI, but a subfolder when we're working internal to Stripe
	DefaultRoot = "."
)

type Options struct {
	Fs   afero.Fs
	Out  io.Writer
	Now  func() time.Time
	Root string
	// used to cap the changefile parsing concurrency. `0` means one worker for each CPU.
	Workers int
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

// given a filename, get the version it represents (if any)
func IntroVersion(name string) (string, bool) {
	version, ok := strings.CutPrefix(name, introPrefix)
	if !ok {
		return "", false
	}

	version, ok = strings.CutSuffix(version, ".md")
	if !ok || version == "" {
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
