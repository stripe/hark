#!/usr/bin/env bash
set -euo pipefail

action_script=$(cd "$(dirname "$0")" && pwd)/release-pr.sh
test_root=$(mktemp -d "${RUNNER_TEMP:-$PWD}/hark-release-test.XXXXXXXX")
trap 'rm -r -- "$test_root"' EXIT
export RUNNER_TEMP="$test_root"
export GITHUB_OUTPUT="$test_root/output"
export MODE=update PREPARE_COMMAND
mkdir "$test_root/repo"
cd "$test_root/repo"
git init -q
git config user.email test@example.com
git config user.name Test
mkdir -p .hark packages/binaries packages/syntax/.hark packages/terraform/.hark
printf '%s\n' '{"release":{"packages":[{"name":"core","version":{"read":["cat","version"]}},{"name":"binaries","path":"packages/binaries","version":{"read":["cat","version"]}},{"name":"unchanged","version":{"read":["cat","other version"]}}]}}' > .hark/config.json
printf '%s\n' '{"release":{"packages":[{"name":"syntax","version":{"read":["cat","version"]}}]}}' > packages/syntax/.hark/config.json
printf '%s\n' '{}' > packages/terraform/.hark/config.json
for file in version 'other version' packages/{binaries,syntax}/version; do printf '1.0.0\n' > "$file"; done
git add .
git commit -qm base
base_commit=$(git rev-parse HEAD)
base_tree=$(git rev-parse 'HEAD^{tree}')

hark() {
  [[ "$*" == 'changes --unreleased' ]] || return 1
  case "$PWD" in
    */terraform) return 1 ;;
  esac
  if [[ ${PENDING:-false} == true ]]; then
    printf '%s\n' '[".hark/changes/pending.change.md"]'
  else
    printf '%s\n' '[]'
  fi
}
export -f hark

prepare() {
  : > "$GITHUB_OUTPUT"
  bash "$action_script" prepare
}

prepare_release() {
  jq -e 'map(.root) == [".", "packages/syntax"] and all(.[]; .changefiles == [".hark/changes/pending.change.md"])' "$1" >/dev/null
  [[ ! -s "$2" ]]
  printf 'syntax\n' >> "$2"
  printf '1.0.1\n' > version
  printf '1.0.1\n' > packages/binaries/version
  printf '%s\n' "$SOURCE_DATE_EPOCH" > prepared-epoch
}
prepare_skipped() {
  printf 'core\nsyntax\n' >> "$2"
}
prepare_invalid_skip() {
  printf 'unknown\n' >> "$2"
}
prepare_stamps_skipped() {
  printf 'syntax\n' >> "$2"
  mkdir -p packages/syntax/.hark/changes
  printf 'stamped\n' > packages/syntax/.hark/changes/pending.change.md
}
prepare_bumps_skipped() {
  prepare_release "$@"
  printf '1.0.1\n' > packages/syntax/version
}
export -f prepare_release prepare_skipped prepare_invalid_skip prepare_stamps_skipped prepare_bumps_skipped

PREPARE_COMMAND=false
prepare
[[ $(sed -n 's/^tree=//p' "$GITHUB_OUTPUT") == "$base_tree" ]]

export PENDING=true
PREPARE_COMMAND=prepare_skipped
prepare
[[ $(sed -n 's/^tree=//p' "$GITHUB_OUTPUT") == "$base_tree" ]]

PREPARE_COMMAND=prepare_invalid_skip
if prepare; then
  echo 'Prepare accepted an unknown skipped package' >&2
  exit 1
fi

PREPARE_COMMAND=prepare_stamps_skipped
if prepare; then
  echo 'Prepare accepted a changefile edit in a skipped root' >&2
  exit 1
fi
git restore --source="$base_commit" --staged --worktree -- .

PREPARE_COMMAND=prepare_bumps_skipped
if prepare; then
  echo 'Prepare accepted a version bump for a skipped package' >&2
  exit 1
fi
git restore --source="$base_commit" --staged --worktree -- .

PREPARE_COMMAND=prepare_release
prepare
prepared_epoch=$(sed -n 's/^epoch=//p' "$GITHUB_OUTPUT")
prepared_tree=$(sed -n 's/^tree=//p' "$GITHUB_OUTPUT")
summary_path=$(sed -n 's/^summary-path=//p' "$GITHUB_OUTPUT")
diff -u - "$summary_path" <<'SUMMARY'
## Packages to release

- <code>binaries</code> at <code>1.0.1</code>
- <code>core</code> at <code>1.0.1</code>

## Skipped packages

- <code>syntax</code>
SUMMARY
export HEAD_SHA
HEAD_SHA=$(GIT_AUTHOR_DATE="@$prepared_epoch +0000" git commit-tree "$prepared_tree" -p "$base_commit" -m 'Version packages')
git restore --source="$base_commit" --staged --worktree -- .

MODE=check
prepare
[[ $(sed -n 's/^tree=//p' "$GITHUB_OUTPUT") == "$prepared_tree" ]]
git restore --source="$base_commit" --staged --worktree -- .
prepare_extra() {
  prepare_release "$@"
  printf extra > extra
}
export -f prepare_extra
PREPARE_COMMAND=prepare_extra
if prepare; then
  echo 'Check accepted an unexpected file' >&2
  exit 1
fi

gh() {
  case "$1 $2" in
    'api repos/test/repo/git/ref/heads/master') printf '%s\n' "$BASE_SHA" ;;
    'api '*'/git/commits/'*) jq -n --arg tree "$TREE" --arg base "$BASE_SHA" '{tree: {sha: $tree}, parents: [{sha: $base}]}' ;;
    'pr view') jq -n --arg head "$OLD_HEAD_SHA" --rawfile body "$RUNNER_TEMP/pr-body" '{state:"OPEN", headRefOid:$head, autoMergeRequest:null, body:$body}' ;;
    'pr create'|'pr edit')
      cp -- "${@: -1}" "$RUNNER_TEMP/pr-body"
      printf '%s\n' "$2" >> "$RUNNER_TEMP/pr-writes"
      printf 'https://github.com/test/repo/pull/42\n'
      ;;
    *) return 1 ;;
  esac
}
export -f gh
export GH_REPO=test/repo BASE_BRANCH=master HEAD_BRANCH=release/version-packages PR_BODY='Initial instructions'
export BASE_SHA="$base_commit" TREE="$prepared_tree" PROPOSAL_BASE_TREE="$base_tree" SUMMARY_PATH="$summary_path"
export OLD_HEAD_SHA="$HEAD_SHA" PR_NUMBER='' FROZEN=false
bash "$action_script" update
grep -Fx 'Initial instructions' "$RUNNER_TEMP/pr-body"
grep -Fx -- '- <code>syntax</code>' "$RUNNER_TEMP/pr-body"

PR_NUMBER=42
bash "$action_script" update
[[ $(cat "$RUNNER_TEMP/pr-writes") == create ]]

printf '\nReviewer notes\n' >> "$RUNNER_TEMP/pr-body"
SUMMARY_PATH="$RUNNER_TEMP/refreshed-summary"
sed 's/- <code>syntax<\/code>/None./' "$summary_path" > "$SUMMARY_PATH"
bash "$action_script" update
diff -u - "$RUNNER_TEMP/pr-body" <<'BODY'
Initial instructions

<!-- hark-release-summary:start -->
## Packages to release

- <code>binaries</code> at <code>1.0.1</code>
- <code>core</code> at <code>1.0.1</code>

## Skipped packages

None.
<!-- hark-release-summary:end -->

Reviewer notes
BODY
[[ $(cat "$RUNNER_TEMP/pr-writes") == $'create\nedit' ]]
