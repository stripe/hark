# Changelog

## 1.1.0

- add `hark inspect` command to read changefile frontmatter and output JSON for each supplied file.  this output currently contains the `path` and `semver_level`.

## 1.0.1

_released `2026-09-15`_

- validate that slugs are alphanumeric and/or hyphens
- separate a change's body from its title with a blank line unless the body opens with a list, so prose and code blocks render correctly
- strip HTML comments out of changefile bodies and release intros when rendering a CHANGELOG
- `hark new` seeds empty changefiles with a comment explaining best practices
- `hark release` creates `.hark/migration-guides/v<MAJOR + 1>.md` when cutting a GA release (if it's not there already), ensuring there's always a place to write the migration guide

## 1.0.0

_released `2026-09-14`_

- initial public release!
