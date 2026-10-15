# fnOS packaging contract

`packaging/fnos/manifest` is the sole release version source. Source UI displays “开发预览”; packaging injects the manifest version into its About panel and the Go health endpoint. Increment the manifest patch version before a new installable delivery; retry a failed build at the same version only if it has not produced a delivery artifact.

## Build

```bash
./scripts/bootstrap-fnpack.sh  # Official fnpack 1.2.3 for this development host
./scripts/package-fpk.sh      # Checks, cross-build, official fnpack, audit, SHA-256
./scripts/verify-fpk.sh dist/QuarkDrive_*.fpk
```

Builds require Go 1.23+, Node.js for frontend checks, Python 3 for metadata/audit, and official fnpack. Runtime packages require none of Node, Python, Docker or external aria2. An explicit `FNPACK=/absolute/path/to/fnpack` may select a separately installed official tool. The bootstrap records its official HTTPS source and locally calculated SHA-256, not a vendor-published signature.

- `.tools/`: development-only fnpack cache and download provenance.
- `build/bin/`: Linux amd64/arm64 binaries.
- `build/staging/<arch>/`: complete fnpack input and intermediate output.
- `build/fnpack-<version>-<arch>.log`: build evidence.
- `dist/QuarkDrive_<version>_x86_64.fpk` and `..._arm64.fpk`: final packages.
- `dist/audit-<version>-<arch>.json`, `dist/SHA256SUMS`: local validation evidence.

Packaging refuses to overwrite final files for an already built version. Older `dist/packages/*_UNVERIFIED.fpk` files are historical research candidates; they are not part of the official build pipeline. The verifier checks packages against the current source and binaries, so historical releases need their matching source checkout to reproduce the audit.

## fnOS host contract

The app retains `appname=quarkdrive`, package user/group `quarkdrive`, data-share `quarkdrive/downloads`, gateway prefix `/app/quarkdrive`, and `${TRIM_APPDEST}/app.sock`. It declares `os_min_version=1.1.3100` as the documented unified-gateway prerequisite; this is not a claim that every eligible OS build has been tested.

The lifecycle uses `TRIM_APPDEST`, `TRIM_PKGVAR` and `TRIM_DATA_SHARE_PATHS`. No development authentication bypass is inherited in production. It checks executable identity through resolved `/proc/<pid>/exe`, waits for the Unix socket, and only sends stop signals while the process still belongs to the app. Downloads and encrypted state under `${TRIM_PKGVAR}/data` are retained by install/upgrade/uninstall hooks; package identity and storage grants must stay stable.

## Required verification

Automated checks cover tests/race/vet, JavaScript syntax/theme checks, shell syntax, metadata/entry/permissions/resource contracts, icon sizes, executable bits, payload checksum, Linux ELF architecture, version marker, byte-for-byte current frontend, and final SHA-256. The audit records source commit plus dirty-worktree status and tool/binary/frontend digests; a dirty build is not represented as a clean committed source release.

Before production acceptance, install on real x86_64/ARM64 fnOS devices and verify launch, gateway admin identity, storage grants, QR login, actual downloads, stop/restart, upgrade with existing data, and uninstall retention. See [manual acceptance](manual-acceptance.md). No build or Docker test substitutes for those checks.

Official references: [fnpack](https://developer.fnnas.com/docs/cli/fnpack), [manifest](https://developer.fnnas.com/docs/core-concepts/manifest), [gateway](https://developer.fnnas.com/docs/core-concepts/gateway-registration), [resources](https://developer.fnnas.com/docs/core-concepts/resource).

## Linux lifecycle regression check

`tests/fnos-lifecycle.sh /absolute/path/to/build/staging/arm64` runs against a built stage on Linux. Run as a non-root user in an isolated test environment: it creates its own temporary app/data directories and tests the real binary/socket, symlink installation path, repeated start, stop/restart, data retention, unrelated PID safety and missing-share error. The current local ARM64 container check passed; real fnOS and native x86_64 lifecycle acceptance remain separate.
