#!/usr/bin/env bash
set -euo pipefail

fail() { printf '%s\n' "$*" >&2; exit 1; }
output() { printf '%s=%s\n' "$1" "$2" >> "$GITHUB_OUTPUT"; }
title='Version packages'

case "$1" in
  state)
    case "$MODE" in
      check)
        [[ "$HEAD_SHA" =~ ^[0-9a-f]{40}$ ]] || fail 'Check mode requires a full head SHA'
        ;;
      update)
        [[ -n "$GH_TOKEN" ]] || fail 'Update mode requires a PR creator token'
        [[ "$BASE_BRANCH" != "$HEAD_BRANCH" ]] || fail 'Release branches must be distinct'
        git check-ref-format "refs/heads/$BASE_BRANCH"
        git check-ref-format "refs/heads/$HEAD_BRANCH"
        pulls=$(gh pr list --base "$BASE_BRANCH" --head "$HEAD_BRANCH" --json number,headRefOid,isCrossRepository,autoMergeRequest,title,author)
        jq -e 'length <= 1' <<< "$pulls" >/dev/null || fail 'Multiple open release PRs'
        old_head=$(gh api "repos/$GH_REPO/git/matching-refs/heads/$HEAD_BRANCH" | jq -r --arg ref "refs/heads/$HEAD_BRANCH" '.[] | select(.ref == $ref) | .object.sha')
        if [[ $(jq length <<< "$pulls") == 1 ]]; then
          jq -e --arg title "$title" --arg head "$old_head" --arg login "$EXPECTED_CREATOR" \
            '.[0] | .isCrossRepository == false and .title == $title and .headRefOid == $head and ($login == "" or .author.login == $login)' \
            <<< "$pulls" >/dev/null || fail 'Unexpected PR using the reserved release branch'
          output pull-request-number "$(jq -r '.[0].number' <<< "$pulls")"
          output frozen "$(jq '.[0].autoMergeRequest != null' <<< "$pulls")"
        fi
        output old-head-sha "$old_head"
        ;;
      *) fail "Unknown mode: $MODE" ;;
    esac
    ;;
  prepare)
    [[ -z $(git status --porcelain) ]] || fail 'Preparation requires a clean checkout'
    BASE_SHA=$(git rev-parse HEAD)
    base_tree=$(git rev-parse 'HEAD^{tree}')
    if [[ "$MODE" == check ]]; then
      [[ $(git rev-list --parents -n 1 "$HEAD_SHA") == "$HEAD_SHA $BASE_SHA" ]] || fail 'The proposed commit must have the checked-out base as its only parent'
      export SOURCE_DATE_EPOCH
      SOURCE_DATE_EPOCH=$(git show -s --format=%at "$HEAD_SHA")
    else
      export SOURCE_DATE_EPOCH
      SOURCE_DATE_EPOCH=$(date +%s)
    fi
    export HARK_PREPARE_RESULT
    HARK_PREPARE_RESULT=$(mktemp "$RUNNER_TEMP/hark-prepare.XXXXXXXX")
    bash -euo pipefail -c "$PREPARE_COMMAND"
    [[ $(git rev-parse HEAD) == "$BASE_SHA" ]] || fail 'The prepare command must not commit'
    jq -e 'type == "object" and (.body | type == "string") and (.packages | type == "array" and all(.[]; (.name | type == "string") and (.from | type == "string") and (.to | type == "string")))' "$HARK_PREPARE_RESULT" >/dev/null
    git add -A
    generated_tree=$(git write-tree)
    if jq -e '.packages == []' "$HARK_PREPARE_RESULT" >/dev/null; then
      [[ "$generated_tree" == "$base_tree" && "$MODE" == update ]] || fail 'No packages selected for a changed tree or a release PR'
    else
      [[ "$generated_tree" != "$base_tree" ]] || fail 'Packages selected without any changes'
    fi
    if [[ "$MODE" == check ]]; then
      [[ "$generated_tree" == "$(git rev-parse "$HEAD_SHA^{tree}")" ]] || fail 'Release PR does not match the generated release tree'
    fi
    output base-sha "$BASE_SHA"
    output epoch "$SOURCE_DATE_EPOCH"
    output tree "$generated_tree"
    output result "$HARK_PREPARE_RESULT"
    ;;
  update)
    if [[ "$FROZEN" == true ]]; then
      output pull-request-number "$PR_NUMBER"
      exit 0
    fi
    live_base=$(gh api "repos/$GH_REPO/git/ref/heads/$BASE_BRANCH" --jq '.object.sha')
    [[ "$BASE_SHA" == "$live_base" ]] || fail 'The base branch advanced during preparation'
    if [[ -n "$PR_NUMBER" ]]; then
      current=$(gh pr view "$PR_NUMBER" --json state,headRefOid,autoMergeRequest)
      jq -e --arg head "$OLD_HEAD_SHA" '.state == "OPEN" and .headRefOid == $head and .autoMergeRequest == null' <<< "$current" >/dev/null || fail 'The PR changed during preparation'
    fi
    if jq -e '.packages == []' "$RESULT" >/dev/null; then
      if [[ -n "$PR_NUMBER" ]]; then
        gh pr close "$PR_NUMBER"
      fi
      exit 0
    fi
    if [[ -n "$OLD_HEAD_SHA" ]]; then
      existing=$(gh api "repos/$GH_REPO/git/commits/$OLD_HEAD_SHA" | jq -r '[.tree.sha, (.parents | map(.sha) | join(" "))] | @tsv')
    else
      existing=''
    fi
    if [[ "$existing" != "$TREE"$'\t'"$BASE_SHA" ]]; then
      commit=$(GIT_AUTHOR_DATE="@$EPOCH +0000" GIT_COMMITTER_DATE="@$EPOCH +0000" \
        git -c user.name='Release automation' -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
        commit-tree "$TREE" -p "$BASE_SHA" -m "$title")
      git -c credential.helper= -c 'credential.helper=!gh auth git-credential' push \
        "--force-with-lease=refs/heads/$HEAD_BRANCH:$OLD_HEAD_SHA" \
        "https://github.com/$GH_REPO.git" "$commit:refs/heads/$HEAD_BRANCH"
    fi
    body=$(mktemp "$RUNNER_TEMP/hark-release-body.XXXXXXXX")
    jq -r '.body' "$RESULT" > "$body"
    if [[ -n "$PR_NUMBER" ]]; then
      gh pr edit "$PR_NUMBER" --body-file "$body"
    else
      url=$(gh pr create --base "$BASE_BRANCH" --head "$HEAD_BRANCH" --title "$title" --body-file "$body")
      PR_NUMBER=${url##*/}
    fi
    output pull-request-number "$PR_NUMBER"
    ;;
  *) fail "Unknown step: $1" ;;
esac
