#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 --issues <comma-separated> --tag <version> [--staging-branch staging] [--main-branch main]"
  echo "Examples:"
  echo "  $0 --issues 88,89 --tag v0.1.1"
  echo "  $0 --issues 88,89 --tag v0.2.0 --staging-branch staging --main-branch main"
}

issues=""
tag=""
staging_branch="staging"
main_branch="main"
staging_workflow="staging-ci.yml"
main_workflow="production-deploy.yml"
project_owner="sgarver"
project_number="1"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --issues)
      issues="$2"
      shift 2
      ;;
    --tag)
      tag="$2"
      shift 2
      ;;
    --staging-branch)
      staging_branch="$2"
      shift 2
      ;;
    --main-branch)
      main_branch="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$issues" || -z "$tag" ]]; then
  usage
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Working tree not clean. Commit or stash changes before release."
  exit 1
fi

echo "Release starting for tag $tag"
echo "Issues: $issues"

scripts/wait-ci.sh "$staging_workflow" "$staging_branch" 1200

pr_url=$(gh pr list --base "$main_branch" --head "$staging_branch" --json url -q '.[0].url' || true)
if [[ -z "$pr_url" ]]; then
  pr_url=$(gh pr create --base "$main_branch" --head "$staging_branch" --title "Release $tag" --body "Closes #${issues//,/ #}" )
fi
echo "Using PR: $pr_url"

scripts/wait-ci.sh "$main_workflow" "$main_branch" 1200

gh pr merge "$pr_url" --squash

./deploy-local.sh

echo "Tagging release $tag"
git tag "$tag"
git push origin "$tag"

if ! gh release view "$tag" >/dev/null 2>&1; then
  gh release create "$tag" --title "$tag" --notes "## Summary\n- automated release for $tag\n- closes issues: $issues\n"
fi

echo "Closing issues: $issues"
IFS=',' read -r -a issue_list <<< "$issues"
for id in "${issue_list[@]}"; do
  gh issue close "$id" --repo sgarver/workforce-loss-tracker --comment "Closing: released in $tag."
done

project_id=$(gh project view "$project_number" --owner "$project_owner" --format json -q '.id')
status_field_id=$(gh project field-list "$project_number" --owner "$project_owner" --format json | jq -r '.fields[] | select(.name=="Status") | .id')
done_id=$(gh project field-list "$project_number" --owner "$project_owner" --format json | jq -r '.fields[] | select(.name=="Status") | .options[] | select(.name=="Done") | .id')

items_json=$(gh project item-list "$project_number" --owner "$project_owner" --format json --limit 500)
for id in "${issue_list[@]}"; do
  item_id=$(echo "$items_json" | jq -r --argjson num "$id" '.items[] | select(.content.number == $num) | .id')
  if [[ -n "$item_id" && "$item_id" != "null" ]]; then
    gh project item-edit --id "$item_id" --project-id "$project_id" --field-id "$status_field_id" --single-select-option-id "$done_id"
  fi
done

echo "Release complete."
