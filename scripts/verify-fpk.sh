#!/usr/bin/env bash
set -euo pipefail
for fpk in "$@"; do
  test -f "$fpk" || exit 1
  echo "Checking $fpk"
  work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
  tar -xzf "$fpk" -C "$work"
  for f in manifest app.tgz ICON.PNG ICON_256.PNG config/resource config/privilege cmd/main; do test -f "$work/$f" || { echo "Missing $f" >&2; exit 1; }; done
  md5=$(md5sum "$work/app.tgz"|cut -d' ' -f1)
  grep -q "^checksum=$md5$" "$work/manifest" || { echo 'app.tgz checksum mismatch' >&2; exit 1; }
  (cd "$work" && tar -tzf app.tgz | grep -F 'bin/quarkdrive' >/dev/null) || { echo 'binary missing' >&2; exit 1; }
  python -m json.tool "$work/config/resource" >/dev/null
  python -m json.tool "$work/config/privilege" >/dev/null
  echo "Container and checksum structure valid (does not prove fnOS installation)."
  rm -rf "$work"; trap - EXIT
done
