# Release PRs

Check out the base commit with `persist-credentials: false`, install Hark and any dependencies, then call the action with `prepare-command` and a PR creator `token`. The action maintains one **Version packages** PR on its reserved `head-branch`. Set `expected-creator` to reject an existing PR with a different author. Serialize calls that update or approve that PR. The action leaves a PR alone once auto-merge is enabled.

The preparation command runs on the checked-out base. It writes generated files to the working tree, does not commit, and writes this JSON to `$HARK_PREPARE_RESULT`:

```json
{
  "packages": [{"name": "example", "from": "1.0.0", "to": "1.0.1"}],
  "body": "Release example 1.0.1."
}
```

Return an empty `packages` array and leave the working tree unchanged when no release is due. Set `body` to the complete PR description, including any repository-required template fields. The action sets `SOURCE_DATE_EPOCH` so dates in the generated tree can be reproduced on a later day.

For validation, pass `mode: check` and the full `head-sha`. Fetch both revisions but check out the proposal's trusted base commit to run the preparation command. The action checks that the proposal is exactly one commit on that base and that the generated tree matches its tree. The calling workflow checks PR identity and base freshness before approval. Check mode needs no write token.
