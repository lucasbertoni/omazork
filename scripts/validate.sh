#!/usr/bin/env bash
# Validate the committed duration data exactly as CI does
# (docs/action-waits.md §7): regenerate every table from data/extract/ and
# fail on drift, then run the validator over table + overlay + shared files.
set -euo pipefail
cd "$(dirname "$0")/.."
go run ./cmd/actiongen -check "$@"
