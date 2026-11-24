#!/usr/bin/env bash
set -euo pipefail

action_script=$(cd "$(dirname "$0")" && pwd)/release-pr.sh
test_root=$(mktemp -d "${RUNNER_TEMP:-$PWD}/hark-release-test.XXXXXXXX")
trap 'rm -r -- "$test_root"' EXIT
export RUNNER_TEMP="$test_root"
export GITHUB_OUTPUT="$test_root/output"
export MODE=update
export PREPARE_COMMAND
mkdir "$test_root/repo"
cd "$test_root/repo"
git init -q
git config user.email test@example.com
git config user.name Test
printf '1.0.0\n' > version
git add version
git commit -qm base
base_commit=$(git rev-parse HEAD)
base_tree=$(git rev-parse 'HEAD^{tree}')

prepare() {
  : > "$GITHUB_OUTPUT"
  bash "$action_script" prepare
}

prepare_empty() {
  printf '%s\n' '{"packages":[],"body":""}' > "$HARK_PREPARE_RESULT"
}
prepare_release() {
  printf '1.0.1\n' > version
  printf '%s\n' "$SOURCE_DATE_EPOCH" > prepared-epoch
  printf '%s\n' '{"packages":[{"name":"example","from":"1.0.0","to":"1.0.1"}],"body":"Release example."}' > "$HARK_PREPARE_RESULT"
}
export -f prepare_empty prepare_release

PREPARE_COMMAND=prepare_empty
prepare
[[ $(sed -n 's/^tree=//p' "$GITHUB_OUTPUT") == "$base_tree" ]]
PREPARE_COMMAND+=$'\nprintf extra > extra'
if prepare; then
  echo 'Prepare accepted a changed tree with no selected packages' >&2
  exit 1
fi
git restore --source="$base_commit" --staged --worktree -- .

PREPARE_COMMAND=prepare_release
prepare
prepared_epoch=$(sed -n 's/^epoch=//p' "$GITHUB_OUTPUT")
prepared_tree=$(sed -n 's/^tree=//p' "$GITHUB_OUTPUT")
export HEAD_SHA
HEAD_SHA=$(GIT_AUTHOR_DATE="@$prepared_epoch +0000" git commit-tree "$prepared_tree" -p "$base_commit" -m 'Version packages')
git restore --source="$base_commit" --staged --worktree -- .

MODE=check
prepare
[[ $(sed -n 's/^tree=//p' "$GITHUB_OUTPUT") == "$prepared_tree" ]]
git restore --source="$base_commit" --staged --worktree -- .
PREPARE_COMMAND+=$'\nprintf extra > extra'
if prepare; then
  echo 'Check accepted an unexpected file' >&2
  exit 1
fi
