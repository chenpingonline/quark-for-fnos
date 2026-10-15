#!/usr/bin/env python3
"""Validate source contracts and actual FPK payloads without extracting untrusted paths."""
import argparse
import hashlib
import io
import json
import re
import struct
import subprocess
import tarfile
from pathlib import Path, PurePosixPath

ROOT = Path(__file__).resolve().parents[1]
PACK = ROOT / 'packaging/fnos'
HOOKS = ['main', 'install_init', 'install_callback', 'upgrade_init', 'upgrade_callback',
         'uninstall_init', 'uninstall_callback', 'config_init', 'config_callback']

def digest(data):
    return hashlib.sha256(data).hexdigest()

def manifest(data):
    values = {}
    for line in data.decode().splitlines():
        if not line.strip() or line.lstrip().startswith('#'):
            continue
        key, value = line.split('=', 1)
        key = key.strip()
        assert key not in values, f'Duplicate manifest key: {key}'
        values[key] = value.strip()
    return values

def contracts(m, privilege, resource, ui):
    assert m['appname'] == 'quarkdrive'
    assert re.fullmatch(r'\d+\.\d+\.\d+', m['version'])
    assert m.get('micro_app') == 'true', 'Official frontend SDK requires micro_app=true'
    assert m['platform'] in ['x86', 'arm'], 'Native binaries need architecture-specific packages'
    assert m['os_min_version'] == '1.1.3100', 'Unified gateway minimum OS contract missing'
    assert m['source'] == 'thirdparty' and m['ctl_stop'] == 'true' and m['checkport'] == 'false'
    assert privilege['defaults']['run-as'] == 'package'
    assert privilege['username'] == privilege['groupname'] == 'quarkdrive'
    assert [x['name'] for x in resource['data-share']['shares']] == ['quarkdrive/downloads']
    entry = ui['.url'][m['desktop_applaunchname']]
    assert m['desktop_uidir'] == 'ui' and m['desktop_applaunchname'] == 'quarkdrive.main'
    assert entry['gatewayPrefix'] == '/app/quarkdrive' and entry['gatewaySocket'] == 'app.sock'
    assert entry['url'].rstrip('/') == '/app/quarkdrive' and entry['allUsers'] is False

def png(data, size):
    assert data[:8] == b'\x89PNG\r\n\x1a\n'
    assert struct.unpack('>II', data[16:24]) == (size, size), f'Expected {size}px PNG'

def source():
    m = manifest((PACK / 'manifest').read_bytes())
    contracts(m, json.loads((PACK / 'config/privilege').read_text()),
              json.loads((PACK / 'config/resource').read_text()),
              json.loads((PACK / 'app/ui/config').read_text()))
    for name in HOOKS:
        p = PACK / 'cmd' / name
        assert p.is_file() and p.stat().st_mode & 0o111, f'Executable hook missing: {name}'
    assert (PACK / 'wizard').is_dir()
    assert '版本 开发预览' in (ROOT / 'frontend/index.html').read_text()
    png((PACK / 'ICON.PNG').read_bytes(), 256)
    png((PACK / 'ICON_256.PNG').read_bytes(), 256)
    for name in ['icon_64.png', 'icon_256.png']:
        png((PACK / 'app/ui/images' / name).read_bytes(), 256)
    print('Source manifest, privilege, resource, gateway, hooks and icons: passed')
    return m

def members(archive):
    result = {}
    for member in archive.getmembers():
        path = PurePosixPath(member.name)
        assert not path.is_absolute() and '..' not in path.parts, 'Unsafe archive path'
        name = path.as_posix().removeprefix('./')
        assert not member.issym() and not member.islnk(), f'Unexpected archive link: {name}'
        assert name not in result, f'Duplicate archive member: {name}'
        assert not any(p == '.DS_Store' or p.startswith('._') for p in path.parts)
        result[name] = member
    return result

def read(archive, entries, name):
    member = entries[name]
    assert member.isfile() and member.size < 64 * 1024 * 1024, f'Invalid file: {name}'
    return archive.extractfile(member).read()

def package(path, tool=None):
    expected = source()
    with tarfile.open(path, 'r:gz') as outer:
        entries = members(outer)
        m = manifest(read(outer, entries, 'manifest'))
        assert m['version'] == expected['version'], 'Package and source versions differ'
        payload = read(outer, entries, 'app.tgz')
        assert hashlib.md5(payload).hexdigest() == m['checksum'], 'Payload checksum mismatch'
        privilege = json.loads(read(outer, entries, 'config/privilege'))
        resource = json.loads(read(outer, entries, 'config/resource'))
        assert read(outer, entries, 'LICENSE') == (ROOT / 'LICENSE').read_bytes()
        png(read(outer, entries, 'ICON.PNG'), 256)
        png(read(outer, entries, 'ICON_256.PNG'), 256)
        for name in HOOKS:
            member = entries['cmd/' + name]
            assert member.mode & 0o111
            assert read(outer, entries, 'cmd/' + name) == (PACK / 'cmd' / name).read_bytes()
    with tarfile.open(fileobj=io.BytesIO(payload), mode='r:gz') as app:
        entries = members(app)
        ui = json.loads(read(app, entries, 'ui/config'))
        contracts(m, privilege, resource, ui)
        binary = read(app, entries, 'bin/quarkdrive')
        machine = struct.unpack('<H', binary[18:20])[0]
        arch = 'amd64' if m['platform'] == 'x86' else 'arm64'
        assert binary[:6] == b'\x7fELF\x02\x01' and machine == (62 if arch == 'amd64' else 183)
        assert entries['bin/quarkdrive'].mode & 0o111
        assert binary == (ROOT / 'build/bin' / ('quarkdrive-' + arch)).read_bytes()
        assert m['version'].encode() in binary, 'Injected binary version marker missing'
        web = {}
        for p in (ROOT / 'frontend').rglob('*'):
            if not p.is_file():
                continue
            relative = p.relative_to(ROOT / 'frontend').as_posix()
            data = p.read_bytes()
            if relative == 'index.html':
                data = data.replace('版本 开发预览'.encode(), ('版本 v' + m['version']).encode())
            assert read(app, entries, 'www/' + relative) == data, f'Stale frontend: {relative}'
            web[relative] = digest(data)
        for size in [64, 256]:
            png(read(app, entries, f'ui/images/icon_{size}.png'), 256)
    result = {'version': m['version'], 'architecture': arch, 'appname': m['appname'],
              'os_min_version': m['os_min_version'], 'sha256': digest(Path(path).read_bytes()),
              'binary_sha256': digest(binary), 'web_sha256': web,
              'source_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
              'source_dirty': bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True)),
              'checks': 'passed', 'real_fnos_installation': 'not tested'}
    if tool:
        result['fnpack'] = {'path': str(Path(tool).resolve()), 'sha256': digest(Path(tool).read_bytes()),
                            'version_output': subprocess.check_output([tool, '--help'], text=True).splitlines()[:3]}
        provenance = Path(tool).with_name('fnpack-source.json')
        if provenance.is_file():
            record = json.loads(provenance.read_text())
            assert record['sha256'] == result['fnpack']['sha256'], 'fnpack cache differs from download record'
            result['fnpack']['download_source'] = record
    return result

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('fpk', nargs='?')
    parser.add_argument('--source', action='store_true')
    parser.add_argument('--fnpack')
    parser.add_argument('--report')
    args = parser.parse_args()
    if args.source:
        source()
    elif args.fpk:
        result = package(args.fpk, args.fnpack)
        if args.report:
            Path(args.report).write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        parser.error('Specify --source or an FPK')
