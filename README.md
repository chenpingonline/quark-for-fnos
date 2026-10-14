> **v0.1.7 下载诊断修复**：增加夸克下载请求兼容头、安全的 Cookie 域名限制和任务列表失败原因；仍需 fnOS 与真实夸克账号验收。

> **v0.1.6 文件夹双击操作**：已移除个人网盘与分享下载列表的“操作”列，双击文件夹进入（键盘回车亦可）。保留复选框和批量下载；参见 docs/INTERACTION_v0.1.6.md。

> **v0.1.3 UI 重构**：已按选定参考图升级四个页面、扫码弹窗、任务详情与主题外观。参见 [UI 变更和限制](docs/UI_REDESIGN_v0.1.3.md)。FPK 仍为未实机验证候选包。

# QuarkDrive for fnOS

**QuarkDrive** is an independent, experimental native fnOS application for downloading files from a personal Quark cloud drive and from Quark sharing links to NAS storage. It is **not** an official Quark product and requires a valid Quark account.

> **Status: v0.1.2 preview / NOT production verified.** The Quark private interfaces, QR login, sharing, and fnOS packaging need testing with a real account and compatible fnOS device. This repository does not contain Quark credentials.

## v0.1.1 gateway fix

A reported fnOS gateway error, `请求来源不匹配`, was caused by comparing the browser `Origin` with the backend-facing `Host`. Browser-originated requests now require same-origin Fetch Metadata when the internal Host differs; cross-site requests remain blocked, as do missing app request identifiers and non-admin writes. The QR dialog now shows an actionable error if login initiation fails. The fix is covered by HTTP handler tests but **has not yet been verified on the reporting fnOS device**.

See [the v0.1 requirement audit](docs/REQUIREMENTS_AUDIT.md) for implemented and missing functionality.

## Scope

| Capability | Implementation | Live verified |
|---|---|---|
| QR login + account inspection | Implemented with Quark web session flow | No |
| Personal cloud listing/search | Implemented, paginated | No |
| Bulk selection and folder recursion | Implemented | Mock/local only |
| Share URL preview, tree, save to cloud, download | Implemented, sequential cloud saves | No |
| Download into fnOS shared directory | Implemented, app data-share path | No |
| Background queue, pause/resume, progress/speed, retry | Implemented, polling UI | Local tests only |
| HTTP Range download, chunk resume, signed URL re-request | Implemented; per-chunk and whole-file SHA-256 | Local HTTP tests |
| Limited parallelism, global bandwidth cap, free space reserve | Implemented | Partially |
| AMD64 and ARM64 binaries | Compiled | Yes, Linux cross-compilation |
| Native FPK candidate packages | Tar-compatible candidates, **not built by fnpack** | **NO: do not treat as certified** |
| Persistent application state | Atomic protected JSON with encrypted tokens (not SQLite) | Local tests |
| Frontend | Self-contained HTML/CSS/JS (not Vue 3) | HTTP responses only |

Both source kinds use the same download scheduler and Range download engine. No Docker, Redis, Node runtime, Python runtime or external aria2 required.

## Build and development

Requirements: Go 1.23+; bash, tar, gzip, Python 3 for package checks. The supplied browser bundle works offline (no npm install).

```bash
cd backend
GOFLAGS= go test -race ./...
cd ..
./scripts/build.sh
./scripts/package-fpk.sh
./scripts/verify-fpk.sh dist/packages/*_UNVERIFIED.fpk
```

If the official `fnpack` is installed, the build script invokes it and produces `QuarkDrive_v0.1.2_amd64.fpk` and `..._arm64.fpk`. Without it, the script emits explicitly named `*_UNVERIFIED.fpk` **research candidates**, not official install-validated builds. The release package is not validated unless a real device actually installs/starts it.

For local development (NOT for an exposed network port):

```bash
cd backend
QUARKDRIVE_DEV=1 QUARKDRIVE_DATA_DIR=./data QUARKDRIVE_WEB_DIR=../frontend go run .
# Open http://127.0.0.1:18088/
```

Developer mode **bypasses fnOS admin authentication** and binds only `127.0.0.1`. Do not port-forward, proxy, or expose it. In production, the backend only listens on a Unix socket within the installed application directory, uses fnOS gateway identity headers, and requires `X-Trim-Isadmin: true`.

## Usage on fnOS (after official packaging and testing)

1. Build two target-architecture releases with an official `fnpack`, or have an authorized tester install a **candidate** on a disposable test NAS (never install unverified packages on important storage).
2. Ensure the application is configured with an available fnOS **data-share** resource, and has correct filesystem permissions. The application will **not** accept arbitrary writable absolute paths.
3. Open **QuarkDrive** from the fnOS application launcher with an administrator account.
4. Scan the QR code with the Quark application, then visit **我的网盘** or **分享下载**.
5. Choose files/folders, create a download, and view the task status under **下载任务**.
6. Configure concurrency, per-file connections, rate cap, retries and reserved storage under **应用设置**.

For a production rollout, follow [manual acceptance tests](docs/manual-acceptance.md). Use `cmd/main start|stop|status` only within the fnOS packaging lifecycle where `TRIM_APPDEST` and `TRIM_PKGVAR` are available.

## Operational notes

- Data is stored in `$TRIM_PKGVAR/data`; the default download location is a `data-share` managed by fnOS. State survives service restarts. Never delete the app data directory when updating.
- Private credentials and share tokens are AES-256-GCM encrypted at rest with a local file key. Root access to NAS still defeats this protection.
- Tasks use file-specific IDs. Downloaded `*.part` files are in `.quarkdrive-part/` beneath the authorized directory; valid checkpoints can be resumed after service restart.
- A share download temporarily saves the selected item in `QuarkDriveTemp` on the user's cloud drive. The application only deletes its own saved file ID after successful download if cleanup is enabled. A failed or interrupted transfer retains its cloud copy.
- Limits imposed by Quark (VIP, bandwidth, space, anti-abuse, token expiry) still apply. This client does not attempt to bypass them.
- Settings for *simultaneous task count* take effect after backend restart; active tasks preserve their existing scheduling slots.
- Storage is currently atomic JSON, not the SQLite storage planned in the original architecture. The web UI is vanilla JS, not Vue 3. This was necessary to build offline. Both are explicit gaps before production-ready v0.1.

## Security and limitations

See [security notes](docs/security.md) and [implementation status](docs/STATUS.md). No public APIs from Quark are guaranteed; these are unofficial interoperability interfaces and may break. Please comply with Quark's service terms.

## Project map

```text
backend/                Go stdlib HTTP API, Quark adapter, persistent task engine and tests
frontend/               Self-contained web app, local QR generator, CSS
packaging/fnos/          manifest, data-share permissions, lifecycle scripts, UI config
scripts/                 Linux cross-build, candidate/official packaging and verification
.github/workflows/       Go tests, Linux binary builds
/docs/                   API, design status, manual test plan, security
```

## License

The original code is distributed under MIT (see `LICENSE`). Vendored QR library copyright/license is preserved in `frontend/vendor/LICENSES.txt`. This license does not give permission to violate service terms or reuse third-party brands.

## v0.1.2 theme synchronization

The theme selector in **应用设置 → 界面主题** offers **跟随飞牛系统** (default), **浅色模式**, or **深色模式**. The override is stored only in the current browser's `localStorage` and is not an application-wide server setting. In the fnOS same-origin iframe, the UI reads fnOS's `fnos-theme-mode` or available desktop theme attributes and listens for updates. When host settings cannot be accessed (e.g. cross-origin standalone page), it falls back to the browser's `prefers-color-scheme` preference. This sync path is covered by isolated frontend tests, **not yet verified on real fnOS clients**.

For offline verification, run `node --test tests/theme.test.cjs`. QR code canvas stays white in dark mode to ensure reliable scanning.
