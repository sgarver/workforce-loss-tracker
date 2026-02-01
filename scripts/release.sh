#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 --issues <comma-separated> --tag <version> [--source-branch <branch>] [--staging-branch staging] [--main-branch main]"
  echo "Examples:"
  echo "  $0 --issues 88,89 --tag v0.1.1"
  echo "  $0 --issues 112 --tag v0.1.1 --source-branch feature/release-docs"
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
source_branch=""

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
    --source-branch)
      source_branch="$2"
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

if [[ -z "$source_branch" ]]; then
  source_branch=$(git rev-parse --abbrev-ref HEAD)
fi

echo "Release starting for tag $tag"
echo "Issues: $issues"
echo "Source branch: $source_branch"

echo "Ready to promote? Type 'approve' to merge $source_branch -> $staging_branch."
read -r approval
if [[ "$approval" != "approve" ]]; then
  echo "Approval not granted. Aborting release."
  exit 1
fi

git fetch origin
git checkout "$staging_branch"
git merge --ff-only "origin/$staging_branch" || true
git merge --ff-only "$source_branch"
git push origin "$staging_branch"

scripts/wait-ci.sh "$staging_workflow" "$staging_branch" 1200

git checkout "$main_branch"
git merge --ff-only "origin/$main_branch" || true

git merge --ff-only "origin/$staging_branch"
git push origin "$main_branch"

scripts/wait-ci.sh "$main_workflow" "$main_branch" 1200

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
