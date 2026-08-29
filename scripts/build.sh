#!/usr/bin/env bash
# Reproducible release build: same flags here, in CI, and in release.sh, so
# checksums pinned in scripts/engine.version match what CI builds from the tag.
# Usage: build.sh <amd64|arm64> <output-path>
set -euo pipefail
cd "$(dirname "$0")/.."

arch="$1"
out="$2"

CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
  go build -trimpath -buildvcs=false -ldflags="-s -w" -o "$out" ./cmd/omazork
