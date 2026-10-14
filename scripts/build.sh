#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$ROOT/dist/bin"
for arch in amd64 arm64; do
  echo "Building linux/$arch ..."
  (cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$ROOT/dist/bin/quarkdrive-$arch" .)
done
echo 'Go binaries successfully built. Frontend is self-contained, no npm runtime dependency.'
