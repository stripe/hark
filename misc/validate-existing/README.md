# validate-existing

Runs the locally built `hark validate` against all 21 SDK branches. Useful for regression testing while working on a change.

```bash
just validate-existing
```

This runs a [potent] plan that checks out each branch for each SDK, validates, then moves on. It puts you back on your original branch when it's done. It's also idempotent, so you can iterate locally to fix anything you find.

## Why bother

`hark validate`'s rules are a contract with a decade's worth of existing files. There's no fixture that can emulate that, so the best way to guard against regression is to actually just test against them all.

Once a version of `hark` is released, all SDKs immediately start using it in CI. So an accidental regression means a lot of red CI.

[potent]: https://github.com/xavdid/potent
