package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// run executes a fresh command tree with the given args and returns everything
// it wrote to stdout/stderr. Use it only for commands that never touch disk.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runFs(t, nil, args...)
}

// runFs is run bound to a specific filesystem, for commands that read or write
// files. A nil fs means the real one.
func runFs(t *testing.T, fs afero.Fs, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	root := newRootCmd("1.2.3", fs)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), err
}

func runFsStreams(t *testing.T, fs afero.Fs, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var out, errOut bytes.Buffer
	root := newRootCmd("1.2.3", fs)
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)

	err = root.Execute()
	return out.String(), errOut.String(), err
}

// harkFs is a filesystem laid out the way hark expects, holding a releases file
// and one released changefile.
//
// Every test that runs a command body needs one of these: the operations all read
// the repo, so running them against the real filesystem would read (and write) the
// hark checkout itself.
func harkFs(t *testing.T) afero.Fs {
	t.Helper()

	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, ".hark/releases.json",
		[]byte(`{"metadata":{"language":"go","channel":"ga"},`+
			`"releases":[{"version":"1.0.0","released_on":"2024-01-15"}]}`), 0o644))
	require.NoError(t, afero.WriteFile(fs, ".hark/changes/2024-01-14_xavdid_did-thing.change.md",
		[]byte("---\ntitle: \"Did a thing\"\nreleased_in_version: \"1.0.0\"\n---\n"), 0o644))
	return fs
}

func TestVersionFlag(t *testing.T) {
	out, err := run(t, "--version")
	require.NoError(t, err)
	assert.Contains(t, out, "1.2.3")
}

func TestHelpListsAllCommands(t *testing.T) {
	out, err := run(t, "--help")
	require.NoError(t, err)

	for _, name := range []string{"new", "release", "build", "validate", "inspect"} {
		assert.Contains(t, out, name)
	}
}

func TestInspect(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "first.change.md", []byte("---\ntitle: First\nsemver_level: major\n---\n"), 0o644))
	require.NoError(t, afero.WriteFile(fs, "second.change.md", []byte("---\ntitle: Second\n---\n"), 0o644))

	stdout, stderr, err := runFsStreams(t, fs, "inspect", "--format", "json", "second.change.md", "first.change.md")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	var got []struct {
		Path        string `json:"path"`
		SemverLevel string `json:"semver_level"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, []struct {
		Path        string `json:"path"`
		SemverLevel string `json:"semver_level"`
	}{
		{Path: "second.change.md", SemverLevel: "patch"},
		{Path: "first.change.md", SemverLevel: "major"},
	}, got)
}

func TestInspectRequiresArgumentsAndJSONFormat(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "example.change.md", []byte("---\ntitle: Example\n---\n"), 0o644))

	for _, args := range [][]string{
		{"inspect", "--format", "json"},
		{"inspect", "example.change.md"},
		{"inspect", "--format", "text", "example.change.md"},
	} {
		stdout, _, err := runFsStreams(t, fs, args...)
		require.Error(t, err, "%v should fail", args)
		assert.Empty(t, stdout)
	}
}

func TestInspectFailureWritesNoJSONAndNamesPath(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "valid.change.md", []byte("---\ntitle: Valid\n---\n"), 0o644))

	stdout, stderr, err := runFsStreams(t, fs, "inspect", "--format", "json", "valid.change.md", "missing.change.md")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "missing.change.md")
}

func TestUnknownCommand(t *testing.T) {
	_, err := run(t, "nope")
	require.Error(t, err)
}

// There are deliberately no flags for paths, so that the changelog and the
// changefiles cannot be pointed at different places. An intro is likewise a file
// the captain writes, not a flag.
func TestRejectedFlags(t *testing.T) {
	for _, args := range [][]string{
		{"build", "--dir", "changes"},
		{"build", "--versions-path", "meta/releases.json"},
		{"validate", "--dir", "changes"},
		{"release", "1.1.0", "--intro", "Some prose."},
	} {
		_, err := runFs(t, harkFs(t), args...)
		require.Error(t, err, "%v should not be accepted", args)
	}
}

// An intro is picked up from its file, with nothing on the command line naming it.
func TestReleaseUsesAnIntroFile(t *testing.T) {
	fs := harkFs(t)
	require.NoError(t, afero.WriteFile(fs, ".hark/intros/intro-1.1.0.md",
		[]byte("A big release.\n"), 0o644))

	out, err := runFs(t, fs, "release", "1.1.0")
	require.NoError(t, err)
	assert.Contains(t, out, "using intro .hark/intros/intro-1.1.0.md")

	changelog, err := afero.ReadFile(fs, "CHANGELOG.md")
	require.NoError(t, err)
	assert.Contains(t, string(changelog), "A big release.")
}

func TestBuild(t *testing.T) {
	fs := harkFs(t)

	out, err := runFs(t, fs, "build")
	require.NoError(t, err)
	assert.Contains(t, out, "CHANGELOG.md")

	written, err := afero.ReadFile(fs, "CHANGELOG.md")
	require.NoError(t, err)
	assert.Contains(t, string(written), "Did a thing")
}

// The releases file gives releases their order and dates, so build cannot run
// without one.
func TestBuildRequiresVersionsFile(t *testing.T) {
	_, err := runFs(t, afero.NewMemMapFs(), "build")
	require.Error(t, err)
}

func TestBuildRejectsArgs(t *testing.T) {
	_, err := run(t, "build", "docs/CHANGELOG.md")
	require.Error(t, err)
}

func TestValidate(t *testing.T) {
	out, err := runFs(t, harkFs(t), "validate")
	require.NoError(t, err)
	assert.Contains(t, out, "validated 1 changefiles")
}

// validate checks everything, so there is nothing to name.
func TestValidateRejectsArgs(t *testing.T) {
	_, err := run(t, "validate", "a.change.md")
	require.Error(t, err)
}

func TestValidateReportsAProblem(t *testing.T) {
	fs := harkFs(t)
	require.NoError(t, afero.WriteFile(fs, ".hark/changes/2024-01-15_xavdid_broken.change.md",
		[]byte("---\nsection: \"Added\"\n---\n"), 0o644))

	out, err := runFs(t, fs, "validate")
	require.Error(t, err)
	assert.Contains(t, out, "title is required")
}

func TestRelease(t *testing.T) {
	fs := harkFs(t)
	require.NoError(t, afero.WriteFile(fs, ".hark/changes/2024-02-01_xavdid_pending.change.md",
		[]byte("---\ntitle: \"Not shipped yet\"\n---\n"), 0o644))

	out, err := runFs(t, fs, "release", "1.1.0",
		"--pinned-api-version", "2024-01-01", "--minimum-runtime-version", "3.10")
	require.NoError(t, err)
	assert.Contains(t, out, "released 1.1.0")

	versions, err := afero.ReadFile(fs, ".hark/releases.json")
	require.NoError(t, err)
	assert.Contains(t, string(versions), "2024-01-01")
	assert.Contains(t, string(versions), "3.10")
}

func TestReleaseRequiresVersion(t *testing.T) {
	out, err := run(t, "release")
	require.Error(t, err)

	// A bad invocation should still show the user how to invoke it.
	assert.Contains(t, out, "Usage:")
}

func TestNew(t *testing.T) {
	fs := harkFs(t)

	out, err := runFs(t, fs, "new", "add-widgets",
		"--title", "Add support for widgets", "--pr-url", "https://github.com/stripe/stripe-go/pull/1",
		"--section", "Added", "--semver-level", "major", "--jira-tag", "DEVSDK-1", "--jira-tag", "DEVSDK-2")
	require.NoError(t, err)
	assert.Contains(t, out, "add-widgets.change.md")

	paths, err := afero.Glob(fs, ".hark/changes/*_add-widgets.change.md")
	require.NoError(t, err)
	require.Len(t, paths, 1)

	written, err := afero.ReadFile(fs, paths[0])
	require.NoError(t, err)
	for _, want := range []string{"Add support for widgets", "Added", "semver_level: major", "DEVSDK-1", "DEVSDK-2"} {
		assert.Contains(t, string(written), want)
	}
}

func TestNewWritesTheSemverLevel(t *testing.T) {
	fs := harkFs(t)

	_, err := runFs(t, fs, "new", "add-widgets", "--title", "Add widgets", "--semver-level", "minor")
	require.NoError(t, err)

	paths, err := afero.Glob(fs, ".hark/changes/*_add-widgets.change.md")
	require.NoError(t, err)
	require.Len(t, paths, 1)

	written, err := afero.ReadFile(fs, paths[0])
	require.NoError(t, err)
	assert.Contains(t, string(written), "semver_level: minor")
}

// A level nobody can read fails the command rather than being written for CI to reject
// later, which matters most to the automation that passes it.
func TestNewRejectsAnUnknownSemverLevel(t *testing.T) {
	_, err := run(t, "new", "add-widgets", "--title", "Add widgets", "--semver-level", "breaking")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "major, minor, patch")
}

func TestNewRejectsExtraArgs(t *testing.T) {
	_, err := run(t, "new", "add-widgets", "unexpected")
	require.Error(t, err)
}

// Each call to newRootCmd must produce an independent tree; flag values must not
// leak between them.
func TestRootCmdIsIndependent(t *testing.T) {
	fs := harkFs(t)

	out, err := runFs(t, fs, "release", "1.1.0", "--pinned-api-version", "2024-01-01")
	require.NoError(t, err)
	assert.Contains(t, out, "1.1.0")

	// A second release with no flag must not pick up the first one's value. It
	// inherits the pinned version from 1.1.0, so the giveaway is the flag not
	// being applied to a conflicting entry.
	_, err = runFs(t, fs, "release", "1.2.0")
	require.NoError(t, err)

	versions, err := afero.ReadFile(fs, ".hark/releases.json")
	require.NoError(t, err)
	assert.Contains(t, string(versions), "1.2.0")
}

// A generated body comes from a file; a hand-written one is inline. Asking for
// both means one of them was going to be silently dropped.
func TestNewBodyFile(t *testing.T) {
	fs := harkFs(t)
	require.NoError(t, afero.WriteFile(fs, "diff.txt", []byte("* Add support for `widgets`\n"), 0o644))

	_, err := runFs(t, fs, "new", "update-generated-code",
		"--title", "Update generated code", "--pr-url", "https://github.com/stripe/stripe-go/pull/1",
		"--stripe-api-change", "--body-file", "diff.txt")
	require.NoError(t, err)

	paths, err := afero.Glob(fs, ".hark/changes/*_update-generated-code.change.md")
	require.NoError(t, err)
	require.Len(t, paths, 1)

	written, err := afero.ReadFile(fs, paths[0])
	require.NoError(t, err)
	assert.Contains(t, string(written), "is_stripe_api_change: true")
	assert.Contains(t, string(written), "* Add support for `widgets`")
}

func TestNewRejectsBothBodyFlags(t *testing.T) {
	_, err := runFs(t, harkFs(t), "new", "add-widgets",
		"--title", "Add widgets", "--body", "inline", "--body-file", "diff.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")
}
