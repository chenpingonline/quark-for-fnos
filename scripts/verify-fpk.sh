#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ $# -gt 0 ]] || { echo 'Usage: verify-fpk.sh package.fpk ...' >&2; exit 2; }
for fpk in "$@"; do python3 "$ROOT/scripts/audit-fpk.py" "$fpk"; done
