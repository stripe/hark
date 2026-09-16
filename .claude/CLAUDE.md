# hark

A Go CLI that validates and compiles markdown changefiles (`.change.md`) into a unified changelog. Used across Stripe SDK repos, both as a standalone CLI and as a Go dependency.

Module: `github.com/stripe/hark`

## Quick Reference

```bash
just test          # run all tests with race detector
just build         # build dev binary to bin/hark
just lint          # run golangci-lint
just format        # gofmt -s -w .
just format-check  # verify formatting (CI mode)
just prepare       # format + lint + test
```

## Project Layout

`changefile/` and `releases/` are the public API — other Go code imports them, so treat
their names and signatures as a compatibility surface. `cmd/` is not: the CLI is not
meant to be embedded, and exports only `Execute`.

- `main.go` — entrypoint, version injection via ldflags
- `cmd/` — cobra commands (thin shells that parse args and delegate)
- `changefile/` — **public**: changefile struct, frontmatter parsing, validation, serialization, recursive/parallel reading, filename generation (`Name`/`ValidateName`)
- `releases/` — **public**: `releases.json` read/write, release comparison (`Compare`), version validity (`IsVersionValid`), date-ordered `Insert`, semver-order `Predecessors`, duplicate detection (`FirstDuplicate`), and release channel (`Channel`) derived from version suffixes
- `internal/changelog/` — high-level operations behind each CLI command (build, validate, release, new)

## Fixed layout

The paths hark uses are hardcoded, not configurable: `.hark/releases.json`,
`.hark/changes/*.change.md`, `.hark/intros/intro-<version>.md`, and `CHANGELOG.md`
at the repo root. Only `Options.Root` moves them, and it has no CLI flag — it
exists so a programmatic caller holding several checkouts can work through them one
at a time. Don't add path flags.

A release's intro is a file, never a field: it exists if and only if
`.hark/intros/intro-<version>.md` does, so nothing can record it inconsistently.
Because it is found purely by name, `validate` is what catches a misnamed one — but
an intro for an unreleased version is valid and ignored until that version is cut,
so intros can be drafted ahead of the release.

Changefile names (`{date}_{user}_{slug}.change.md`, where `{date}` is `YYYY-MM-DD`)
must lead with the date: the changelog orders changes within a release by filename,
oldest first — except that a change marked `is_stripe_api_change` sorts last whatever
it is named. Release dates in `releases.json` use that same format. The `{slug}` is
letters, numbers, and hyphens only (`changefile.ValidateSlug`) — an underscore in it
would read as a fourth segment, and a space makes the file a chore to type.

How big a change is lives in one field, `semver_level`. A changefile that leaves it
off is a patch, so read it through `Changefile.Level()` rather than the field: `major`
is what earns the ⚠️ in the changelog and what the migration-guide check looks for.

## Conventions

- Go version: whatever `go.mod` declares (CI reads `go-version-file: go.mod`)
- Exported identifiers get doc comments; every package has a package comment
- Comments should be **CONCISE** and **SPECIFIC**.
- Avoid stutter in the public API (`releases.ReadFile`, not `releases.ReadVersionsFile`)
- CLI commands only parse args and call into `internal/changelog`; no business logic in `cmd/`
- Build the command tree with a constructor instead of package-level `var`s, so tests get independent trees and flag state can't leak
- Operations take an `Options` struct carrying their dependencies (`afero.Fs`, `io.Writer`, paths) — nothing reaches for globals, `os.Stdout`, or the real filesystem directly
- Every function doing file I/O takes `afero.Fs` as a parameter for testability
- Tests use `afero.NewMemMapFs()` — no temp files, no cleanup
- Use `testify` (assert/require) for test assertions
- Errors are returned, never panicked
- Validation methods return `[]error` to report all issues at once

## Dependencies

| Package                       | Purpose                          |
| ----------------------------- | -------------------------------- |
| `github.com/spf13/cobra`      | CLI framework                    |
| `github.com/goccy/go-yaml`    | YAML deserialization             |
| `github.com/spf13/afero`      | Filesystem abstraction (testing) |
| `github.com/stretchr/testify` | Test assertions                  |
| `golang.org/x/sync/errgroup`  | Parallel file reading            |
