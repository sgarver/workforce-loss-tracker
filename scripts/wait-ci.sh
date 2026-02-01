#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <workflow> <branch> [timeout_seconds]"
  echo "Example: $0 staging-ci.yml staging 900"
  exit 1
fi

workflow="$1"
branch="$2"
timeout_seconds="${3:-900}"
start_time=$(date +%s)

echo "Waiting for CI: workflow=$workflow branch=$branch timeout=${timeout_seconds}s"

while true; do
  run_json=$(gh run list --workflow "$workflow" --branch "$branch" --limit 1 --json status,conclusion,url,updatedAt)
  status=$(echo "$run_json" | jq -r '.[0].status')
  conclusion=$(echo "$run_json" | jq -r '.[0].conclusion')
  url=$(echo "$run_json" | jq -r '.[0].url')
  updated=$(echo "$run_json" | jq -r '.[0].updatedAt')

  echo "status=$status conclusion=$conclusion updated=$updated url=$url"

  if [[ "$status" == "completed" ]]; then
    if [[ "$conclusion" == "success" ]]; then
      echo "CI passed"
      exit 0
    fi
    echo "CI failed: $conclusion"
    exit 2
  fi

  now=$(date +%s)
  if (( now - start_time > timeout_seconds )); then
    echo "Timed out waiting for CI"
    exit 3
  fi

  sleep 20
done
