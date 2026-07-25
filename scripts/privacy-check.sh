#!/usr/bin/env bash
set -euo pipefail

repo_root=${1:-.}
cd "$repo_root"

private_markers=(
  'graph''ite'
  'OP''G-[0-9][0-9]*'
)

failed=0
for marker in "${private_markers[@]}"; do
  if git grep -n -i -E -- "$marker"; then
    failed=1
  fi

  if git ls-files | grep -i -E -- "$marker"; then
    failed=1
  fi
done

if (( failed )); then
  echo "Privacy scan failed: forbidden private reference found." >&2
  exit 1
fi
