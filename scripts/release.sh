#!/usr/bin/env bash
# Cut a release: bump manifest.json, pin reproducible-build checksums in
# scripts/engine.version, commit, tag. Pushing the tag triggers the release
# workflow, which rebuilds with the same flags, verifies the pinned checksums,
# and attaches the binaries to a GitHub Release.
# Usage: release.sh v0.2.0
set -euo pipefail
cd "$(dirname "$0")/.."

version="${1:?usage: release.sh vX.Y.Z}"
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "release.sh: version must look like vX.Y.Z" >&2; exit 1 ;;
esac

[ -z "$(git status --porcelain)" ] || { echo "release.sh: working tree not clean" >&2; exit 1; }

# Reproducibility hinges on the Go toolchain version; go.mod pins it for CI.
want_go="go$(sed -n 's/^go //p' go.mod)"
have_go="$(go version | awk '{print $3}')"
[ "$have_go" = "$want_go" ] || { echo "release.sh: need $want_go, have $have_go" >&2; exit 1; }

mkdir -p dist
scripts/build.sh amd64 dist/omazork-linux-amd64
scripts/build.sh arm64 dist/omazork-linux-arm64

{
  echo "version=$version"
  echo "sha256_amd64=$(sha256sum dist/omazork-linux-amd64 | cut -d' ' -f1)"
  echo "sha256_arm64=$(sha256sum dist/omazork-linux-arm64 | cut -d' ' -f1)"
} > scripts/engine.version

# Bump manifest.json version (strip the leading v).
plain="${version#v}"
sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"$plain\"/" manifest.json

git add manifest.json scripts/engine.version
git commit -m "Release $version"
git tag "$version"

echo "Now: git push origin master $version"
