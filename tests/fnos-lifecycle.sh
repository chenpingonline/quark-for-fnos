#!/usr/bin/env bash
# Linux-only lifecycle contract test; run in an isolated non-root container.
set -euo pipefail
PACKAGE=${1:?Provide fnpack staging directory}
VERSION=$(awk -F= '$1 == "version" {print $2}' "$PACKAGE/manifest")
TEST_ROOT=$(mktemp -d)
trap 'TRIM_APPDEST="$TEST_ROOT/appdest" TRIM_PKGVAR="$TEST_ROOT/var" "$PACKAGE/cmd/main" stop >/dev/null 2>&1 || true; rm -rf "$TEST_ROOT"' EXIT
mkdir -p "$TEST_ROOT/target" "$TEST_ROOT/var" "$TEST_ROOT/downloads"
cp -a "$PACKAGE/app/." "$TEST_ROOT/target/"
ln -s "$TEST_ROOT/target" "$TEST_ROOT/appdest"
export TRIM_APPDEST="$TEST_ROOT/appdest" TRIM_PKGVAR="$TEST_ROOT/var"
export TRIM_DATA_SHARE_PATHS="$TEST_ROOT/downloads" TRIM_TEMP_LOGFILE="$TEST_ROOT/errors"
export QUARKDRIVE_DEV=1 # Lifecycle must remove this before starting production.
"$PACKAGE/cmd/main" start
"$PACKAGE/cmd/main" status
PID_BEFORE=$(cat "$TRIM_PKGVAR/quarkdrive.pid")
"$PACKAGE/cmd/main" start
[[ $(cat "$TRIM_PKGVAR/quarkdrive.pid") = "$PID_BEFORE" ]]
python3 - "$TRIM_APPDEST/app.sock" "$VERSION" <<'PY'
import socket,json,sys,re

def request(path):
    s=socket.socket(socket.AF_UNIX);s.settimeout(5);s.connect(sys.argv[1])
    s.sendall(('GET /app/quarkdrive'+path+' HTTP/1.1\r\nHost: nas\r\nX-Trim-Userid: 1000\r\nX-Trim-Isadmin: true\r\nConnection: close\r\n\r\n').encode())
    data=b''
    while True:
        part=s.recv(65536)
        if not part:break
        data+=part
    s.close()
    assert data.startswith(b'HTTP/1.1 200'),data[:200]
    return data.split(b'\r\n\r\n',1)

_,body=request('/api/v1/health')
out=json.loads(body);assert out['mode']=='fnos' and out['version']==sys.argv[2],out
_,html=request('/')
assets=re.findall(rb'(?:src|href)="(\./assets/[^" ]+)"',html)
assert assets
for asset in assets:
    header,body=request(asset.decode()[1:])
    assert body and (b'javascript' in header or asset.endswith(b'.css')),header
print('All packaged page scripts and styles served through fnOS Unix-socket route')
print('Packaged Unix-socket health: version '+out['version']+', mode fnos')
PY
printf 'preserve\n' > "$TEST_ROOT/downloads/existing-file"
STATE_BEFORE=$(sha256sum "$TRIM_PKGVAR/data/state.json" | cut -d' ' -f1)
"$PACKAGE/cmd/main" stop
if "$PACKAGE/cmd/main" status; then exit 1; else [[ $? = 3 ]]; fi
"$PACKAGE/cmd/main" start
[[ $(sha256sum "$TRIM_PKGVAR/data/state.json" | cut -d' ' -f1) = "$STATE_BEFORE" ]]
for hook in upgrade_init upgrade_callback uninstall_init uninstall_callback; do "$PACKAGE/cmd/$hook"; done
[[ $(cat "$TEST_ROOT/downloads/existing-file") = preserve ]]
"$PACKAGE/cmd/main" stop
# An unrelated PID must never be signalled.
sleep 60 & FOREIGN=$!
printf '%s\n' "$FOREIGN" > "$TRIM_PKGVAR/quarkdrive.pid"
"$PACKAGE/cmd/main" stop
kill -0 "$FOREIGN"
kill "$FOREIGN"; wait "$FOREIGN" 2>/dev/null || true
unset TRIM_DATA_SHARE_PATHS
if "$PACKAGE/cmd/main" start; then exit 1; fi
grep -q 'data-share paths are unavailable' "$TRIM_TEMP_LOGFILE"
echo 'Symlink path, non-root start/status/stop/restart, version, data retention, foreign PID and missing grants: passed'
