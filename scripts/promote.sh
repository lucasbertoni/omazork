#!/usr/bin/env bash
# Merge develop into master without carrying coding-agent control files.
#
# master is what the Omarchy marketplace installs, so it must not contain
# CLAUDE.md, .claude/ or docs/agents/. Those live on develop only.
# Flow is one-way: never merge master back into develop (it would delete them).
set -euo pipefail

STRIP=(CLAUDE.md .claude docs/agents)
SRC=${1:-develop}

cd "$(git rev-parse --show-toplevel)"
[[ -z $(git status --porcelain) ]] || { echo "working tree not clean" >&2; exit 1; }

git checkout -q master
# Conflicts on the stripped paths (modify/delete) are expected; resolved below.
git merge --no-ff --no-commit "$SRC" || true
git rm -rq --ignore-unmatch --cached "${STRIP[@]}" 2>/dev/null || true
rm -rf "${STRIP[@]}"

if [[ -n $(git diff --name-only --diff-filter=U) ]]; then
  echo "unresolved conflicts outside the stripped paths:" >&2
  git diff --name-only --diff-filter=U >&2
  exit 1
fi

git commit -q -m "Merge $SRC into master (agent files stripped)"
for p in "${STRIP[@]}"; do
  if git ls-tree -r --name-only HEAD | grep -q "^$p"; then
    echo "ERROR: $p still tracked on master" >&2; exit 1
  fi
done
echo "master is at $(git rev-parse --short HEAD); agent files absent."
