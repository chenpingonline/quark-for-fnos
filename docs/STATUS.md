# Engineering Status

Date: 2026-10-15.

## Verified in this development environment

- Linux amd64 and arm64 Go cross-compilation with `CGO_ENABLED=0`.
- Local Go unit tests including 206/Content-Range, non-Range fallback, restart checkpoint loss, SHA/secret persistence, wrong 200 response, input path safety, admin gate and paused task cancellation.
- Go race detector passes for all test scenarios (`go test -race`).
- Browser JavaScript parse check passes (`node --check`).
- Referenced UI scripts/styles are tested through the actual Go handler for standalone and fnOS gateway paths. Browser flows with fixture API data cover files, settings, search, task details and pause/resume display; these are not real Quark account tests.
- Slow-stream HTTP regression confirms speed is reported before chunk completion without advancing durable checkpoints; four-second averaging, stalls, concurrent transfer counters, pause/resume and completion are covered.
- Download-link requests use the desktop client profile on drive.quark.cn; request-identity consistency and credential-safe error codes are tested with mocked transport. Real large-file compatibility remains account-dependent and requires device validation.
- Official fnpack 1.2.3 builds with source/payload/architecture/version audits; historical tar candidates remain separate.
- Non-root ARM64 Linux container lifecycle checks passed: Unix socket health/version, symlink installation path, repeated start, stop/restart, data retention, foreign PID safety and missing resource grants. This is not real fnOS acceptance.

- Theme auto mode now uses bundled `@trimjs/web-app` 0.5.0, `micro_app=true`, `getPlatformConfig()` and `os/theme`. Tests cover host priority over browser preference, event/snapshot races, manual override, SDK failure, mobile initialization, BFCache restore and subscription cleanup. Official host theme APIs require fnOS 1.2.0401 / mobile App 1.34.0 or later; real-device SDK/host validation remains pending.

## Implemented, requires external validation

- Quark web QR authentication and cookie lifecycle with live Quark App.
- Account lookup and actual listing/search response compatibility.
- Share link token, file listing, save-as, returned ID, cloud cleanup and refresh.
- Real Quark signed download links, expiration behavior, VIP/account quota behavior.
- fnOS identity headers, gateway socket routing, data-share grant, lifecycle callbacks.
- fnOS GUI install/update/uninstall; real AMD64/ARM64 hardware.

## Outstanding production requirements

1. Successful clean-device install/upgrade testing. Official fnpack builds are now supported; historical `_UNVERIFIED.fpk` artifacts are not official builds.
2. Quark integration tests with an authorized account and share links. No guarantee that private API responses have not changed.
3. Migrate JSON state to SQLite/WAL with schema migration and crash testing at scale.
4. Port self-contained UI to Vue 3 + TypeScript if the original architecture is mandatory. Functional v0.1 UI currently uses plain JavaScript and a polling mechanism instead of WebSocket.
5. Validate permissions, sandbox boundaries, symlink race-hardening, multitenant access, and large-file performance on a real fnOS installation.
6. Expand share-token refresh, reauthorization, cloud temporary-directory reconciliation and quota-aware batched saves.
7. Real-world benchmarking and prolonged fault injection for 100+ tasks, power loss, network outages and partial disk full.

Do not describe this release as a production-certified fnOS app or as guaranteed compatible with current Quark servers.

## v0.1.1 patch

- Fixed the fnOS gateway Origin/Host mismatch for browser requests with `Sec-Fetch-Site: same-origin`; requests from cross-site contexts still fail, and the existing administrator and app header checks remain mandatory.
- QR dialog displays the API failure inline with retry instructions.
- Added `origin_test.go` with proxy, cross-site, missing marker and non-admin cases.
- No official fnpack build or actual Quark/fnOS account validation was performed as part of this patch.
- Full requirement audit: `docs/REQUIREMENTS_AUDIT.md`.

### v0.1.2: UI theme follow-up (2026-10-14)

- Added fnOS theme preference bridge and immediate light/dark rendering.
- Automatically checks same-origin fnOS iframe storage / desktop attributes, falling back to OS appearance when unavailable.
- Watches storage and host DOM updates; supports manual light/dark override via **应用设置**.
- Added dark styles for login modal, all views, forms, tables and task status; QR canvas intentionally remains black on white.
- Added `tests/theme.test.cjs` (7 passing isolated tests). Hardware / real fnOS live theme switching remains **unverified**.

- 下载任务提供“清理记录”：按已完成、失败或两者清理，默认仅移除记录，也可显式选择同时删除 NAS 文件（直接删除，不经过回收站）；进行中、已暂停及仍在收尾的任务跳过。文件删除限定在当前授权目录内，以目录描述符逐级访问，拒绝符号链接、目录和越界路径，保留其他任务引用的文件；失败任务只清理按任务 ID 保存的临时文件，保留下载路径上可能已存在的其他文件。自动测试覆盖文件保留/删除、授权和路径校验、持久化、保存失败与状态变化；界面以样例任务验证取消、方式切换与确认流程，真机验收待完成。

- 下载任务列表支持复选框、当前分类全选及半选，刷新保留选择，切换分类清空选择。选中后在顶部批量开始（暂停任务恢复、失败任务重试）、暂停或清理；已完成任务不参与开始/暂停。批量请求最多 4 个并发，单项失败保留选择并继续其余任务；以样例页面验证混合状态批量操作和禁用状态，真机验收待完成。

- 清理确认框精简为任务数量、默认不勾选的“同时删除 NAS 本机文件”及取消/清理按钮。清理范围取选中任务；未选择时取当前分类内的已完成/失败任务，不再二次选择范围和方式。

- 账号弹窗按需读取夸克 `/member` 接口，显示会员类型及已用/总空间；失败仅显示会员信息暂不可用，不改变登录状态。不推测到期日期，也不把接口未知类型标作普通用户。接口调用最长 5 秒，返回字段限于展示数据；会员页面以样例数据验证，真实账号验收待完成。

- 会员图标复用夸克官方分享页的原始 VIP/SVIP/SVIP+ PNG，并记录来源、等级映射及 SHA-256。EXP_VIP 与 EXP_SVIP 按官方枚举分别显示 VIP/SVIP 徽标，取消未经确认的 EXP_SVIP → 88VIP 推断；普通/未知类型不显示付费徽标。

- 修复删除 NAS 文件时对上级目录额外要求列目录权限的问题：Linux 使用 O_PATH 逐级打开路径，保留不跟随符号链接与 unlinkat 删除权限校验。非 root Linux 容器中已先复现原始 permission denied，再验证允许穿过但不可列出的父目录可正常清理；缺少删除权限时仍拒绝删除。真实 fnOS 目录 ACL 待设备复验。
