#!/usr/bin/env bash
# Bootstrap the engine binary into bin/omazork. Run by the overlay on every
# launch (#12): idempotent — a checksum match against scripts/engine.version
# is a no-op, a mismatch after a `git pull` refetches or rebuilds.
#
# Order: pinned release download, then go-build fallback (#16). Set
# OMAZORK_DEV=1 (or touch bin/DEV) to always build from source and skip the
# download — otherwise a dev build gets clobbered by the release binary on
# the next launch.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p bin

dev_mode=0
if [ "${OMAZORK_DEV:-0}" = "1" ] || [ -e bin/DEV ]; then
  dev_mode=1
fi

go_build() {
  command -v go >/dev/null 2>&1 || return 1
  # go's build cache makes this a cheap no-op when nothing changed.
  go build -o bin/omazork ./cmd/omazork
}

if [ "$dev_mode" = 1 ]; then
  if go_build; then exit 0; fi
  echo "omazork bootstrap: OMAZORK_DEV set but no Go toolchain" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64) arch=arm64 ;;
  *)
    echo "omazork bootstrap: unsupported arch $(uname -m)" >&2
    exit 1
    ;;
esac

version=""
want_sha=""
if [ -f scripts/engine.version ]; then
  version="$(sed -n 's/^version=//p' scripts/engine.version)"
  want_sha="$(sed -n "s/^sha256_${arch}=//p" scripts/engine.version)"
fi

sha_of() { sha256sum "$1" | cut -d' ' -f1; }

# Already current?
if [ -n "$want_sha" ] && [ -x bin/omazork ] && [ "$(sha_of bin/omazork)" = "$want_sha" ]; then
  exit 0
fi

# Pinned release download (curl for a public repo, gh while it's private).
if [ -n "$want_sha" ] && [ -n "$version" ]; then
  asset="omazork-linux-${arch}"
  url="https://github.com/lucasbertoni/omazork/releases/download/${version}/${asset}"
  tmp="$(mktemp bin/.download.XXXXXX)"
  trap 'rm -f "$tmp"' EXIT

  fetched=0
  if command -v curl >/dev/null 2>&1 &&
     curl -fsSL --max-time 60 -o "$tmp" "$url" 2>/dev/null; then
    fetched=1
  elif command -v gh >/dev/null 2>&1 &&
       gh release download "$version" --repo lucasbertoni/omazork \
         --pattern "$asset" --output "$tmp" --clobber 2>/dev/null; then
    fetched=1
  fi

  if [ "$fetched" = 1 ]; then
    if [ "$(sha_of "$tmp")" = "$want_sha" ]; then
      chmod +x "$tmp"
      mv "$tmp" bin/omazork
      trap - EXIT
      exit 0
    fi
    echo "omazork bootstrap: checksum mismatch on downloaded $asset, falling back" >&2
  fi
  rm -f "$tmp"
  trap - EXIT
fi

# Fallback: build from source.
if go_build; then exit 0; fi

if [ -x bin/omazork ]; then
  # Keep whatever we have rather than bricking the console.
  echo "omazork bootstrap: download failed and no Go toolchain; keeping existing binary" >&2
  exit 0
fi

echo "omazork bootstrap: could not download the engine release and no Go toolchain found" >&2
echo "install Go (https://go.dev/dl/) or authenticate gh, then retry" >&2
exit 1
