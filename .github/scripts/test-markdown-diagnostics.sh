#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
predicate="$script_dir/check-markdown-diagnostics.jq"
healthy='{"basename_conflicts":[],"asset_basename_conflicts":[],"phantoms":[],"broken_anchors":[]}'

jq -e -f "$predicate" <<< "$healthy" > /dev/null

for field in basename_conflicts asset_basename_conflicts phantoms broken_anchors; do
  diagnostics="$(jq --arg field "$field" '.[$field] = [{}]' <<< "$healthy")"
  if jq -e -f "$predicate" <<< "$diagnostics" > /dev/null; then
    echo "Expected $field problems to fail the diagnostic gate" >&2
    exit 1
  fi
done

echo "Markdown diagnostic gate: all 5 cases passed"
