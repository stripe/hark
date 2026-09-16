# Changelog

# Unreleased

_released `2026-09-15`_

- add `--date` flag to `hark new` (optional, defaults to today)
- validate that slugs are alphanumeric and/or hyphens
- separate a change's body from its title with a blank line unless the body opens with a list, so prose and code bodies render as their own block
- strip HTML comments out of changefile bodies and release intros when building a changelog
- seed `hark new` changefiles with a commented prompt for the body, unless a body was supplied

## 1.0.0

_released `2026-09-14`_

- initial public release!
