#!/usr/bin/env bash
# Bootstrap the engine binary into bin/omazork. Run by the overlay on every
# launch (#12): idempotent, so a no-op when the binary is already current.
#
# Minimal version for #15 — #16 replaces the build with a checksum-pinned
# release download, keeping go-build as the fallback.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p bin

if command -v go >/dev/null 2>&1; then
  # go's build cache makes this a cheap no-op when nothing changed.
  go build -o bin/omazork ./cmd/omazork
  exit 0
fi

if [ -x bin/omazork ]; then
  exit 0
fi

echo "omazork bootstrap: no Go toolchain and no bin/omazork binary" >&2
echo "install Go, or wait for the release-download bootstrap (#16)" >&2
exit 1
