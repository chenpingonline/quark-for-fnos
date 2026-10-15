#!/usr/bin/env bash
# Release builds require official fnpack. Never silently create a tar-based FPK.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION=$(awk -F= '{gsub(/[[:space:]]/, "", $1); if ($1 == "version") {gsub(/[[:space:]]/, "", $2); print $2}}' "$ROOT/packaging/fnos/manifest")
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid manifest version' >&2; exit 1; }
FNPACK_BIN="${FNPACK:-$ROOT/.tools/fnpack}"
if [[ -z "${FNPACK:-}" && ! -x "$FNPACK_BIN" ]]; then
  FNPACK_BIN=$(command -v fnpack || true)
fi
[[ -n "$FNPACK_BIN" && -x "$FNPACK_BIN" ]] || { echo 'Official fnpack required: run scripts/bootstrap-fnpack.sh' >&2; exit 1; }
for label in x86_64 arm64; do
  [[ ! -e "$ROOT/dist/QuarkDrive_${VERSION}_${label}.fpk" ]] || { echo 'Version already packaged. Increment manifest version before a new delivery.' >&2; exit 1; }
done
export COPYFILE_DISABLE=1 LC_ALL=C
"$ROOT/scripts/check.sh"
"$ROOT/scripts/build.sh"
mkdir -p "$ROOT/build/staging" "$ROOT/dist"
for arch in amd64 arm64; do
  stage="$ROOT/build/staging/$arch"
  rm -rf "$stage"
  mkdir -p "$stage"
  cp -a "$ROOT/packaging/fnos/." "$stage/"
  cp "$ROOT/LICENSE" "$stage/LICENSE"
  platform=x86; label=x86_64
  if [[ "$arch" = arm64 ]]; then platform=arm; label=arm64; fi
  mkdir -p "$stage/app/bin" "$stage/app/www"
  cp "$ROOT/build/bin/quarkdrive-$arch" "$stage/app/bin/quarkdrive"
  cp -a "$ROOT/frontend/." "$stage/app/www/"
  python3 - "$stage" "$VERSION" "$platform" <<'PY'
import re,sys
from pathlib import Path
stage=Path(sys.argv[1]); p=stage/'manifest'
p.write_text(re.sub(r'^platform\s*=.*$', 'platform='+sys.argv[3], p.read_text(), flags=re.M))
p=stage/'app/www/index.html';source=p.read_text()
assert '版本 开发预览' in source, 'Frontend version marker missing'
p.write_text(source.replace('版本 开发预览','版本 v'+sys.argv[2]))
PY
  chmod 755 "$stage/app/bin/quarkdrive" "$stage"/cmd/*
  (cd "$stage" && "$FNPACK_BIN" build) 2>&1 | tee "$ROOT/build/fnpack-$VERSION-$arch.log"
  [[ -f "$stage/quarkdrive.fpk" ]] || { echo 'fnpack output missing' >&2; exit 1; }
  python3 "$ROOT/scripts/audit-fpk.py" "$stage/quarkdrive.fpk" --fnpack "$FNPACK_BIN" --report "$ROOT/dist/audit-$VERSION-$label.json"
  cp "$stage/quarkdrive.fpk" "$ROOT/dist/QuarkDrive_${VERSION}_${label}.fpk"
done
python3 - "$ROOT/dist" <<'PY'
import hashlib, sys
from pathlib import Path
folder=Path(sys.argv[1]); lines=[]
for p in sorted(folder.glob('*.fpk')):
    lines.append(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n')
(folder/'SHA256SUMS').write_text(''.join(lines))
PY
echo "Official fnpack builds: $ROOT/dist"
