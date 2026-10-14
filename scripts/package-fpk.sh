#!/usr/bin/env bash
# Builds staging packages. Uses official fnpack where present, falls back to format-compatible tar container (UNVERIFIED on fnOS).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$ROOT/scripts/build.sh"
mkdir -p "$ROOT/dist/packages"
for arch in amd64 arm64; do
  stage="$(mktemp -d)"
  trap 'rm -rf "$stage"' EXIT
  cp -a "$ROOT/packaging/fnos/." "$stage/"
  sed -i "s/PLATFORM_REPLACE/$(test "$arch" = amd64 && echo x86 || echo arm)/" "$stage/manifest.template"
  mv "$stage/manifest.template" "$stage/manifest"
  mkdir -p "$stage/app/bin" "$stage/app/www"
  cp "$ROOT/dist/bin/quarkdrive-$arch" "$stage/app/bin/quarkdrive"
  cp -a "$ROOT/frontend/." "$stage/app/www/"
  chmod +x "$stage/app/bin/quarkdrive"
  # Make official fnpack authoritative if it is installed.
  if command -v fnpack >/dev/null 2>&1; then
    (cd "$stage" && fnpack build)
    pkg=$(find "$stage" "$(dirname "$stage")" -maxdepth 1 -name '*.fpk' -type f | head -n1)
    test -n "$pkg" || { echo 'fnpack did not create FPK' >&2; exit 1; }
    cp "$pkg" "$ROOT/dist/packages/QuarkDrive_v0.1.7_$arch.fpk"
  else
    # Reverse-engineered fnpack 1.2.x layout: app.tgz with manifest.checksum MD5.
    # This is an install candidate, NOT a certified official FPK build.
    (cd "$stage/app" && tar -czf "$stage/app.tgz" .)
    checksum=$(md5sum "$stage/app.tgz" | awk '{print $1}')
    printf '\nchecksum=%s\n' "$checksum" >> "$stage/manifest"
    rm -rf "$stage/app"
    (cd "$stage" && tar -czf "$ROOT/dist/packages/QuarkDrive_v0.1.7_${arch}_UNVERIFIED.fpk" --exclude='*.DS_Store' .)
  fi
  rm -rf "$stage"
  trap - EXIT
done
(cd "$ROOT/dist/packages" && sha256sum *.fpk > SHA256SUMS)
echo "Packages: $ROOT/dist/packages"
