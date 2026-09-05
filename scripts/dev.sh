#!/usr/bin/env bash
# Dev loop: check the working tree, rebuild the engine, reload the shell, open
# the console, and tail the wrapper's log. Assumes the README dev setup (repo
# symlinked into ~/.config/omarchy/plugins/omazork, bin/DEV present) so the
# overlay's bootstrap builds from this tree on launch.
#
# Usage: scripts/dev.sh [--quick] [--no-open] [--no-log]
#   --quick    skip go test and validate.sh (go vet + build + qmllint only)
#   --no-open  reload the shell but don't toggle the console
#   --no-log   don't tail journalctl after opening
set -euo pipefail
cd "$(dirname "$0")/.."

quick=0 open=1 log=1
for arg in "$@"; do
  case "$arg" in
    --quick) quick=1 ;;
    --no-open) open=0 ;;
    --no-log) log=0 ;;
    *) echo "usage: scripts/dev.sh [--quick] [--no-open] [--no-log]" >&2; exit 2 ;;
  esac
done

step() { printf '\033[1m==> %s\033[0m\n' "$*"; }

[ -e bin/DEV ] || { step "touch bin/DEV (so bootstrap never clobbers the dev build)"; mkdir -p bin; touch bin/DEV; }

step "go vet"
go vet ./...

if [ "$quick" = 0 ]; then
  step "go test"
  go test ./...
  # The wrapper refuses to start on invalid data, so catch it here rather than
  # in a silent console.
  step "validate data"
  ./scripts/validate.sh
fi

step "go build"
go build -o bin/omazork ./cmd/omazork

if [ -x /usr/lib/qt6/bin/qmllint ]; then
  step "qmllint"
  # The shell's qs.* modules aren't on qmllint's import path, so warnings are
  # noisy and non-fatal here; only hard errors (syntax, duplicate ids) fail.
  if /usr/lib/qt6/bin/qmllint qml/*.qml 2>&1 | grep -E '^Error' ; then
    exit 1
  fi
fi

step "restart shell"
omarchy-restart-shell

if [ "$open" = 1 ]; then
  # Give quickshell a moment to come up before toggling.
  for _ in $(seq 1 20); do pgrep -x quickshell >/dev/null && break; sleep 0.25; done
  sleep 1
  step "open console"
  omarchy-shell shell toggle omazork
fi

if [ "$log" = 1 ]; then
  step "wrapper log (ctrl-c to stop)"
  journalctl --user -f -n 0 --no-pager | grep --line-buffered -i omazork
fi
