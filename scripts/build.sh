#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION=$(awk -F= '{gsub(/[[:space:]]/, "", $1); if ($1 == "version") {gsub(/[[:space:]]/, "", $2); print $2}}' "$ROOT/packaging/fnos/manifest")
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "Invalid manifest version" >&2; exit 1; }
mkdir -p "$ROOT/build/bin"
for arch in amd64 arm64; do
  echo "Building linux/$arch ..."
  (cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.appVersion=$VERSION" -o "$ROOT/build/bin/quarkdrive-$arch" .)
done
echo 'Go binaries successfully built. Frontend is self-contained, no npm runtime dependency.'
