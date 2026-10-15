> **v0.1.14**：顶部紧凑“下载”按钮、搜索图标按需展开，移除取消选择；文件夹单击进入，目录单页显示；客户端下载请求适配与四秒平均速度统计。安装包使用官方 fnpack，提供 x86_64 / ARM64 两个架构。项目仍为实验性预览，完整真机验收待完成。

> **列表交互更新**：网盘、搜索与分享目录自动读取全部数据，在同一列表中滚动查看；选中文件后，顶部显示下载操作，不再使用底部操作栏或翻页。

> **文件夹交互更新**：个人网盘和分享目录均改为单击进入，复选框仍用于选择下载；键盘回车操作保持可用。

> **打包规范已统一**：官方 fnpack 构建，架构分包、manifest 单一版本源、完整包内审计。见 [打包规范](docs/PACKAGING.md)。历史 `_UNVERIFIED.fpk` 不属于当前正式构建流程；真实 fnOS 安装和账号验收仍待完成。

> **v0.1.7 下载诊断修复**：增加夸克下载请求兼容头、安全的 Cookie 域名限制和任务列表失败原因；仍需 fnOS 与真实夸克账号验收。

> **v0.1.6 文件夹双击操作**：已移除个人网盘与分享下载列表的“操作”列，双击文件夹进入（键盘回车亦可）。保留复选框和批量下载；参见 docs/INTERACTION_v0.1.6.md。

> **v0.1.3 UI 重构**：已按选定参考图升级四个页面、扫码弹窗、任务详情与主题外观。参见 [UI 变更和限制](docs/UI_REDESIGN_v0.1.3.md)。FPK 仍为未实机验证候选包。

# QuarkDrive for fnOS

**QuarkDrive** is an independent, experimental native fnOS application for downloading files from a personal Quark cloud drive and from Quark sharing links to NAS storage. It is **not** an official Quark product and requires a valid Quark account.

> **Status: preview / NOT production verified.** The Quark private interfaces, QR login, sharing, and fnOS packaging need testing with a real account and compatible fnOS device. This repository does not contain Quark credentials.

## v0.1.1 gateway fix

A reported fnOS gateway error, `请求来源不匹配`, was caused by comparing the browser `Origin` with the backend-facing `Host`. Browser-originated requests now require same-origin Fetch Metadata when the internal Host differs; cross-site requests remain blocked, as do missing app request identifiers and non-admin writes. The QR dialog now shows an actionable error if login initiation fails. The fix is covered by HTTP handler tests but **has not yet been verified on the reporting fnOS device**.

See [the v0.1 requirement audit](docs/REQUIREMENTS_AUDIT.md) for implemented and missing functionality.

## Scope

| Capability | Implementation | Live verified |
|---|---|---|
| QR login + account inspection | Implemented with Quark web session flow | No |
| Personal cloud listing/search | Implemented; API pages automatically combined into one scrollable list | No |
| Bulk selection and folder recursion | Implemented | Mock/local only |
| Share URL preview, tree, save to cloud, download | Implemented, sequential cloud saves | No |
| Download into fnOS shared directory | Implemented, app data-share path | No |
| Background queue, pause/resume, progress/speed, retry | Implemented, polling UI | Local tests only |
| HTTP Range download, chunk resume, signed URL re-request | Implemented; per-chunk and whole-file SHA-256 | Local HTTP tests |
| Limited parallelism, global bandwidth cap, free space reserve | Implemented | Partially |
| AMD64 and ARM64 binaries | Compiled | Yes, Linux cross-compilation |
| Native FPK packages | Official fnpack builds with architecture/version/payload audits | Real-device installation pending |
| Persistent application state | Atomic protected JSON with encrypted tokens (not SQLite) | Local tests |
| Frontend | Self-contained HTML/CSS/JS (not Vue 3) | Local browser flows with fixture API data |

Both source kinds use the same download scheduler and Range download engine. No Docker, Redis, Node runtime, Python runtime or external aria2 required.

## Build and development

Requirements: Go 1.23+; bash, tar, gzip, Python 3 for package checks. The supplied browser bundle works offline (no npm install).

```bash
./scripts/bootstrap-fnpack.sh
./scripts/package-fpk.sh
./scripts/verify-fpk.sh dist/QuarkDrive_*.fpk
```

The manifest is the release version source. Binaries and staged frontend receive that version during packaging. Official fnpack is required; builds fail if it is missing and never silently create tar-based installation candidates. Final x86_64/ARM64 FPKs and SHA-256/audit records are stored in `dist/`; intermediates are kept in `build/`. See [the packaging contract](docs/PACKAGING.md). A successful official build does not prove real-device installation or startup.

For local development (NOT for an exposed network port):

```bash
cd backend
QUARKDRIVE_DEV=1 QUARKDRIVE_DATA_DIR=./data QUARKDRIVE_WEB_DIR=../frontend go run .
# Open http://127.0.0.1:18088/
```

Developer mode **bypasses fnOS admin authentication** and binds only `127.0.0.1`. Do not port-forward, proxy, or expose it. In production, the backend only listens on a Unix socket within the installed application directory, uses fnOS gateway identity headers, and requires `X-Trim-Isadmin: true`.

## Usage on fnOS (after official packaging and testing)

1. Build both architecture packages with official `fnpack`, audit them, then install the matching package on an authorized test NAS for device acceptance. Historical tar candidates are not official builds.
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
