# hark

A tool to generate informative `CHANGELOG.md` files for the [Stripe SDKs](https://docs.stripe.com/sdks#server-side-libraries).

> [!NOTE]
> Though `hark` is open source, it's not a traditional OSS project: it doesn't accept external contributions and has no public issue tracker.
>
> While you are welcome to use it and adapt it for your needs, `hark` does **not** aim to be a general-purpose changelog management solution. It's designed specifically to fit the needs of Stripe's SDKs and Docs teams.

## Installation

We already depend on `hark` via `mise`. Run `mise install` once you've completed your devenv setup (go/sdks/devenv).

## Usage

> this section is written with the assumption that you're working in an SDK directory

Each user-facing PR needs a corresponding `.change.md` file. Create one by running `hark new` in your local branch. It will auto-populate as much information as it can (including your PR information, if one has already been created). If any fields need manual correction, `hark new` will warn you. Ensure your changefile is structurally valid before pushing by running `hark validate` (or CI will fail).

### Releasing versions

`hark release VERSION` adds a new entry to `releases.json` and populates information (e.g. pinned api version)

## Writing a great changelog

- No one likes AI prose. Write it yourself and respect your reader's time
- Readers will scan our changelog to determine if there's anything new or interesting they can take advantage of with their Stripe integration.
  - mention new classnames/methods exactly, so users can copy from the changelog and use new stuff in their code without having to think too hard about it
- We want to mention what's new and who might be able to take advantage of it
  - for example: "Add new event handlers. These are especially relevant if you're already handling event notifications and want more compile-time checks for your integration"
- We can take as much space as we need, but be concise and informative. See [go/docs-style-guide](http://go/docs-style-guide#voice-and-style) for help writing technical docs at Stripe
- For bugs, mention who might have been affected
  - for example: "Fix an issue with API key authentication when passing a callable to the `StripeClient` constructor". Users will know if they do that or not

### Writing a great migration guide

- You should explain what changed, but focus on the resulting user impact.
  - ❌ We moved all our event code to a new directory
  - ✅ Import paths have changed. You'll have to adjust anywhere you call `from stripe.something import ...`.
- Before/after code blocks are very useful!

## Architecture

`hark` is designed to manage small files and turn them into one `CHANGELOG.md`. Each file has a strictly enforced structure and there's a lot of validation to ensure internal consistency. Plus, as part of a PR, we'll have a chance to review changelog content before it goes live.

```
stripe-<lang>/
├── .hark/
│   ├── releases.json
│   ├── changes/
│   │   ├── 2026-01-22_xavdid_some-thing.change.md
│   │   ├── 2026-03-22_xavdid_an-upcoming-feature.change.md
│   │   └── 2026-06-22_xavdid_neato.change.md
│   └── intros/
│       └── intro-1.2.3.md
└── CHANGELOG.md
```

- `releases.json` is a manifest of all of the released versions of an SDK, including the date, its pinned API version (if any) and minimum supported language version
- `changefiles` are little markdown fragments w/ metadata describing a user-facing change to an SDK. They have the extension `.change.md`
- `intro-<VERSION>.md` is a bit of prose that goes after a version header but before any changes are listed. It's a good place to make announcements, summarize/highlight features, or anything else!

### Go modules

While `hark` is mostly designed as a CLI, it also has a Go API for internal use

- `changefile` helps load and validates `Changefile`s
- `releases` provides tools for interacting with the `releases.json` file

Everything else lives in the `internal/changelog` package and isn't part of the public API. These hold a lot of functions backing the CLI commands (so we can call them programmatically).

## Prior art

These are all great projects that we took inspiration from, but none quite fit exactly what we needed.

- [Towncrier](https://towncrier.readthedocs.io/en/stable/)
- [Changesets](https://github.com/changesets/changesets)
- [Scriv](https://scriv.readthedocs.io/en/latest/index.html)
- [Reno](https://docs.openstack.org/reno/latest/)
