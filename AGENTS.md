# QuarkDrive fnOS packaging

- Keep `packaging/fnos/manifest` as the only release version source. Inject that version into Linux binaries, staged frontend and FPK names; never maintain independent release labels in source.
- Before a new requested installable delivery, increment the manifest patch version unless the user specifies one. Do not overwrite a previously delivered version. Failed builds before an FPK exists may retry the same version.
- Use official fnpack; absence or failure is a build error, never a reason to silently create a tar-based FPK.
- Use native architecture-specific packages: x86_64 (`GOARCH=amd64`, `platform=x86`) and ARM64 (`GOARCH=arm64`, `platform=arm`). Do not label a single-architecture binary `platform=all`.
- Cache build tools in `.tools/`, binaries and staging in `build/`, final FPKs and audit/SHA-256 records in `dist/`. These directories are ignored.
- Preserve `appname=quarkdrive`, gateway prefix, package user, data-share names and persistent state paths. Lifecycle hooks must not delete downloads or account data during upgrade.
- Run `scripts/check.sh`, official packaging and `scripts/audit-fpk.py` before delivery. Verify source version, UI version, binary, ELF architecture, JSON contracts, executable hooks, package payload hash and SHA-256.
- Record official-tool build separately from real-device installation/upgrade/start/stop validation. Never claim fnOS acceptance solely from archive or container checks.
- Packaging does not authorize Git commits, pushes, publication or installing on a NAS. Preserve unrelated worktree changes.
