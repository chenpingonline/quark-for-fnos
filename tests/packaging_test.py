"""Regressions for fnOS metadata and unsafe/ambiguous archive rejection."""
import importlib.util
import io
import tarfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location('fpk_audit', Path(__file__).resolve().parents[1] / 'scripts/audit-fpk.py')
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

class PackagingTests(unittest.TestCase):
    def test_duplicate_manifest_version_rejected(self):
        with self.assertRaises(AssertionError):
            audit.manifest(b'version=1.0.0\nversion=2.0.0\n')

    def test_unsafe_or_duplicate_members_rejected(self):
        for names in [['../escape'], ['/absolute'], ['manifest', './manifest']]:
            with self.subTest(names=names):
                data = io.BytesIO()
                with tarfile.open(fileobj=data, mode='w') as out:
                    for name in names:
                        item = tarfile.TarInfo(name)
                        out.addfile(item, io.BytesIO())
                data.seek(0)
                with tarfile.open(fileobj=data) as archive, self.assertRaises(AssertionError):
                    audit.members(archive)

    def test_symlink_rejected(self):
        data = io.BytesIO()
        with tarfile.open(fileobj=data, mode='w') as out:
            item = tarfile.TarInfo('app.tgz')
            item.type = tarfile.SYMTYPE
            item.linkname = '/outside'
            out.addfile(item)
        data.seek(0)
        with tarfile.open(fileobj=data) as archive, self.assertRaises(AssertionError):
            audit.members(archive)

    def test_icon_size_rejected(self):
        with self.assertRaises(AssertionError):
            audit.png((audit.PACK / 'ICON.PNG').read_bytes(), 256)

if __name__ == '__main__':
    unittest.main()
