#!/usr/bin/env bash
# Validate the committed duration data exactly as CI does
# (docs/action-waits.md §7): regenerate every table from data/extract/ and
# fail on drift, run the validator over table + overlay + shared files, replay
# the walkthrough fixtures against the layered result and fail on any breached
# calibration gate, and fail if a committed calibration report is stale.
set -euo pipefail
cd "$(dirname "$0")/.."
go run ./cmd/actiongen -check "$@"
