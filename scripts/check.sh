#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
(cd "$ROOT/backend" && go test -race ./... && go vet ./...)
node --check "$ROOT/frontend/assets/app.js"
node --check "$ROOT/frontend/assets/file-pages.js"
node --check "$ROOT/frontend/assets/qr.js"
node --test "$ROOT/tests/theme.test.cjs" "$ROOT/tests/file-pages.test.cjs"
for script in "$ROOT"/packaging/fnos/cmd/*; do sh -n "$script"; done
for script in "$ROOT"/scripts/*.sh; do bash -n "$script"; done
python3 "$ROOT/scripts/audit-fpk.py" --source
python3 "$ROOT/tests/packaging_test.py"
bash -n "$ROOT/tests/fnos-lifecycle.sh"
