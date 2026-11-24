#!/usr/bin/env bash
set -euo pipefail

fail() { printf '%s\n' "$*" >&2; exit 1; }
output() { printf '%s=%s\n' "$1" "$2" >> "$GITHUB_OUTPUT"; }
title='Version packages'

read_version() {
  local package=$1 name directory arg version
  local -a args=()
  name=$(jq -r '.name' <<< "$package")
  directory=$(jq -r '.path' <<< "$package")
  while IFS= read -r -d '' arg; do args+=("$arg"); done < <(jq -jr '.read[] | ., "\u0000"' <<< "$package")
  version=$(cd -- "$directory" && HARK_RELEASE_PACKAGE="$name" "${args[@]}") || fail "Could not read version for $name"
  [[ -n "$version" && "$version" != *[[:cntrl:]]* ]] || fail "Expected one version for $name"
  printf '%s' "$version"
}

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
    pending=$(mktemp "$RUNNER_TEMP/hark-pending.XXXXXXXX")
    skipped=$(mktemp "$RUNNER_TEMP/hark-skipped.XXXXXXXX")
    configs=$(mktemp "$RUNNER_TEMP/hark-configs.XXXXXXXX")
    git ls-files -z -- '.hark/config.json' ':(glob)**/.hark/config.json' > "$configs"
    roots='[]'
    packages='[]'
    while IFS= read -r -d '' config; do
      root=$(dirname "$(dirname "$config")")
      [[ ! -L "$config" ]] || fail "Release configuration must be a regular file: $config"
      release=$(jq -c '
        if type != "object" then error("expected config object")
        elif has("release") | not then null
        elif (.release | type) != "object" then error("release must be an object")
        else .release end
      ' "$config")
      [[ "$release" != null ]] || continue
      packages=$(jq -ce --arg root "$root" --argjson release "$release" '
        . + ($release | if has("packages") then .packages else [] end
          | if type != "array" then error("release.packages must be an array") else . end
          | map(if (.name | type) == "string" and (.name | length) > 0
                then {root: $root, name, path: (.path // $root), read: .version.read}
                else error("invalid package name") end))
      ' <<< "$packages")
      changes=$(cd -- "$root" && hark changes --unreleased)
      jq -e 'type == "array" and all(.[]; type == "string")' <<< "$changes" >/dev/null
      if jq -e 'length > 0' <<< "$changes" >/dev/null; then
        roots=$(jq -ce --arg root "$root" --argjson changes "$changes" '. + [{root: $root, changefiles: $changes}]' <<< "$roots")
      fi
    done < "$configs"
    printf '%s\n' "$roots" > "$pending"
    if [[ "$roots" != '[]' ]]; then
      before=$(mktemp "$RUNNER_TEMP/hark-versions.XXXXXXXX")
      if [[ "$MODE" == update ]]; then
        jq -e 'all(.[]; .read | type == "array" and length > 0 and .[0] != "" and all(.[]; type == "string" and (contains("\u0000") | not)))' \
          <<< "$packages" >/dev/null || fail 'Each release package must configure version.read as a command array'
        while IFS= read -r package; do
          version=$(read_version "$package")
          jq -c --arg version "$version" '. + {version: $version}' <<< "$package" >> "$before"
        done < <(jq -c '.[]' <<< "$packages")
      fi
      bash -euo pipefail -c "$PREPARE_COMMAND"' "$@"' hark-prepare "$pending" "$skipped"
    fi
    [[ $(git rev-parse HEAD) == "$BASE_SHA" ]] || fail 'The prepare command must not commit'
    git add -A
    generated_tree=$(git write-tree)
    skipped_names='[]'
    while IFS= read -r name || [[ -n "$name" ]]; do
      jq -e --arg name "$name" 'any(.[]; .name == $name)' <<< "$packages" >/dev/null || fail "Unknown skipped package: $name"
      skipped_names=$(jq -ce --arg name "$name" '. + [$name]' <<< "$skipped_names")
      printf 'Skipping release of %s\n' "$name"
    done < "$skipped"
    skipped_roots=$(mktemp "$RUNNER_TEMP/hark-skipped-roots.XXXXXXXX")
    jq -jr --argjson skipped "$skipped_names" '
      group_by(.root)[] | select(all(.[]; .name as $name | $skipped | index($name) != null))
      | .[0].root, "\u0000"
    ' <<< "$packages" > "$skipped_roots"
    while IFS= read -r -d '' root; do
      git --literal-pathspecs diff --cached --quiet -- "$root/.hark/changes" "$root/.hark/releases.json" \
        || fail "Preparation changed Hark release records for skipped root: $root"
    done < "$skipped_roots"
    if [[ "$MODE" == check ]]; then
      [[ "$generated_tree" != "$base_tree" ]] || fail 'No release changes were generated'
      [[ "$generated_tree" == "$(git rev-parse "$HEAD_SHA^{tree}")" ]] || fail 'Release PR does not match the generated release tree'
    elif [[ "$generated_tree" != "$base_tree" ]]; then
      releases=$(mktemp "$RUNNER_TEMP/hark-releases.XXXXXXXX")
      while IFS= read -r package; do
        version=$(read_version "$package")
        jq -ce --arg version "$version" --argjson skipped "$skipped_names" '
          .name as $name | ($skipped | index($name) != null) as $skip
          | if $skip and .version != $version then error("Preparation bumped skipped package: " + $name)
            else {name, version: $version, skipped: $skip, changed: (.version != $version)} end
        ' <<< "$package" >> "$releases"
      done < "$before"
      summary=$(mktemp "$RUNNER_TEMP/hark-summary.XXXXXXXX")
      jq -rs '
        def code: "<code>" + @html + "</code>";
        def bullets: if length == 0 then "None." else join("\n") end;
        sort_by(.name)
        | ["## Packages to release", "", (map(select(.changed) | "- \(.name | code) at \(.version | code)") | bullets),
           "", "## Skipped packages", "", (map(select(.skipped) | "- \(.name | code)") | bullets)] | join("\n")
      ' "$releases" > "$summary"
      output summary-path "$summary"
    fi
    output base-sha "$BASE_SHA"
    output epoch "$SOURCE_DATE_EPOCH"
    output tree "$generated_tree"
    output base-tree "$base_tree"
    ;;
  update)
    if [[ "$FROZEN" == true ]]; then
      output pull-request-number "$PR_NUMBER"
      exit 0
    fi
    live_base=$(gh api "repos/$GH_REPO/git/ref/heads/$BASE_BRANCH" --jq '.object.sha')
    [[ "$BASE_SHA" == "$live_base" ]] || fail 'The base branch advanced during preparation'
    current='{}'
    if [[ -n "$PR_NUMBER" ]]; then
      current=$(gh pr view "$PR_NUMBER" --json state,headRefOid,autoMergeRequest,body)
      jq -e --arg head "$OLD_HEAD_SHA" '.state == "OPEN" and .headRefOid == $head and .autoMergeRequest == null' <<< "$current" >/dev/null || fail 'The PR changed during preparation'
    fi
    if [[ "$TREE" == "$PROPOSAL_BASE_TREE" ]]; then
      if [[ -n "$PR_NUMBER" ]]; then
        gh pr close "$PR_NUMBER"
      fi
      exit 0
    fi
    body=$(mktemp "$RUNNER_TEMP/hark-release-body.XXXXXXXX")
    jq -jr --rawfile summary "$SUMMARY_PATH" --arg initial "$PR_BODY" '
      "<!-- hark-release-summary:start -->" as $start | "<!-- hark-release-summary:end -->" as $end
      | ($start + "\n" + $summary + $end) as $section
      | (.body // $initial) as $body
      | if ($body | contains($start) or contains($end)) then
          ($body | split($start)) as $parts
          | if ($parts | length) != 2 or ($parts[0] | contains($end)) then error("Invalid Hark summary markers")
            else ($parts[1] | split($end)) as $tail
              | if ($tail | length) != 2 then error("Invalid Hark summary markers")
                else $parts[0] + $section + $tail[1] end end
        else $body + (if $body == "" then "" else "\n\n" end) + $section + "\n" end
    ' <<< "$current" > "$body"
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
    if [[ -z "$PR_NUMBER" ]]; then
      url=$(gh pr create --base "$BASE_BRANCH" --head "$HEAD_BRANCH" --title "$title" --body-file "$body")
      PR_NUMBER=${url##*/}
    elif ! jq -e --rawfile body "$body" '.body == $body' <<< "$current" >/dev/null; then
      gh pr edit "$PR_NUMBER" --body-file "$body"
    fi
    output pull-request-number "$PR_NUMBER"
    ;;
  *) fail "Unknown step: $1" ;;
esac
