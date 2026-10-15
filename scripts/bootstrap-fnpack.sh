#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION=1.2.3
case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) host=darwin-arm64;;
  Darwin/x86_64) host=darwin-amd64;;
  Linux/aarch64|Linux/arm64) host=linux-arm64;;
  Linux/x86_64) host=linux-amd64;;
  *) echo 'Unsupported fnpack build host' >&2; exit 1;;
esac
URL="https://static2.fnnas.com/fnpack/fnpack-${VERSION}-${host}"
mkdir -p "$ROOT/.tools"
TEMP=$(mktemp "$ROOT/.tools/fnpack-download.XXXXXX")
trap 'rm -f "$TEMP"' EXIT
curl --fail --location --silent --show-error --proto '=https' --max-time 120 "$URL" -o "$TEMP"
chmod 755 "$TEMP"
"$TEMP" --help | head -n 3
mv "$TEMP" "$ROOT/.tools/fnpack"
python3 - "$ROOT/.tools/fnpack" "$URL" "$VERSION" <<'PY'
import hashlib, json, sys
from pathlib import Path
p=Path(sys.argv[1])
p.with_name('fnpack-source.json').write_text(json.dumps({'url':sys.argv[2],'version':sys.argv[3],'sha256':hashlib.sha256(p.read_bytes()).hexdigest()},indent=2)+'\n')
PY
