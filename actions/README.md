# `hark` actions

These are github actions that the SDKs depend on via their own `changelog.yml` files (plus some internal utilities).

All the externally relevant actions are combined into a `stripe/hark/actions/checks@master` composite for convenience. Pull requests can opt out of the changefile or migration-guide requirements with the `skip-changefile` and `skip-migration-guide` description checkboxes. Consuming workflows must include `edited` in `pull_request.types` for checkbox changes to rerun the checks.
