#!/usr/bin/env bash
# Regenerate data/extract/<game>.json from the pinned historicalsource ZIL
# (docs/action-waits.md §5.1). Sources are fetched at the exact commits the
# research write-up quotes, so extraction is reproducible byte-for-byte.
# Usage: extract.sh [zork1|zork2|zork3 ...]  (default: all three)
set -euo pipefail
cd "$(dirname "$0")/.."

pin() {
  case "$1" in
    zork1) echo 97b7b3d68c075dd9af7da499c3e9690ada3471fd ;;
    zork2) echo 3da9661098809788a99cef00f00c865c6c204f96 ;;
    zork3) echo 3ec9ed412b5f3cafe65d83c727d07db1fe4a86a8 ;;
    *) echo "extract.sh: unknown game $1" >&2; exit 2 ;;
  esac
}

fetch() {
  local game="$1" sha dir
  sha="$(pin "$game")"
  dir=".cache/zil/$game"
  if [ -d "$dir/.git" ] && [ "$(git -C "$dir" rev-parse HEAD)" = "$sha" ]; then
    return 0
  fi
  rm -rf "$dir"
  mkdir -p "$dir"
  git -C "$dir" init -q
  git -C "$dir" remote add origin "https://github.com/historicalsource/$game"
  git -C "$dir" fetch -q --depth 1 origin "$sha"
  git -C "$dir" checkout -q "$sha"
}

games=("$@")
[ "${#games[@]}" -eq 0 ] && games=(zork1 zork2 zork3)
for game in "${games[@]}"; do
  fetch "$game"
  go run ./cmd/zilextract -game "$game" -src ".cache/zil/$game"
done
