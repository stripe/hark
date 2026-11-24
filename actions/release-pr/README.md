# Release PRs

Check out the base commit with `persist-credentials: false`, install Hark and any dependencies, then call the action with `prepare-command` and a PR creator `token`. The action maintains one **Version packages** PR on its reserved `head-branch`. Set `expected-creator` to reject an existing PR with a different author. Serialize calls that update or approve that PR. The action leaves a PR alone once auto-merge is enabled.

A Hark root opts in with a tracked `.hark/config.json` containing its own `release` object. The action runs `hark changes --unreleased` in each opted-in root. It calls the preparation command once if any root has pending entries, passing two arguments:

1. The path to a JSON file containing an array of `{root, changefiles}` objects for roots with pending entries. `root` is relative to the repository checkout (`.` for the root); `changefiles` are paths relative to that root.
2. The path to an empty skip file. Append one package name from `release.packages[].name` per line to skip it. Leave that package’s release files untouched so its changefiles remain pending. The command owns any rules about packages that must release together. The action rejects unknown names and changes to the Hark ledger or changefiles in a fully skipped root.

Generate the release files in the working tree without committing. The Git tree determines what goes into the PR. If the tree is unchanged, the action closes an existing release proposal or leaves the branch alone when there isn't one. Set `pr-body` for any repository-required PR description text. The action sets `SOURCE_DATE_EPOCH` so the command can reproduce the generated files on a later day.

For validation, pass `mode: check` and the full `head-sha`. Fetch both revisions but check out the proposal's trusted base commit to run the preparation command. The action checks that the proposal is exactly one commit on that base and that the generated tree matches its tree. The calling workflow checks PR identity and base freshness before approval. Check mode needs no write token.
