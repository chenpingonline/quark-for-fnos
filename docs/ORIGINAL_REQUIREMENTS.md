# QuarkDrive for fnOS — v0.1 架构与开发实施规格

状态：技术设计草案（待 PoC 和 fnOS 实机验证）  
日期：2026-10-14  
目标：飞牛 fnOS 原生 FPK，支持个人夸克网盘和夸克分享文件下载至 NAS。

## 1. 决策与非目标

- 技术：Vue 3 + TypeScript + Vite + Naive UI + Pinia；Go + Gin；纯 Go SQLite 驱动；单进程常驻服务。
- fnOS：Native FPK，不依赖 Docker、Redis、Python 或外部 aria2；统一网关通过 `/app/quarkdrive` 将请求代理到 `${TRIM_APPDEST}/app.sock`。
- 下载入口：个人网盘文件（无需转存）；分享文件（必要时先转存到应用自有临时目录）；二者统一由任务调度器与 Range 下载器处理。
- v0.1 权限范围：一个夸克账号；仅 fnOS 管理员使用；只开放读取云盘、分享转存和应用明确拥有的临时云盘文件清理功能。
- 非目标：跨账号隔离、云端删除/移动/重命名、自动双向同步、绕过第三方服务限制、Docker 镜像、UC/阿里/百度云盘。
- 兼容目标：优先满足支持统一网关与开放文件授权 API 的 fnOS 版本。开放 API 文档说明其能力要求 fnOS >=1.2.0401、对应 App >=1.34.0；最终 `os_min_version` 以实机验证为准。

## 2. 分层与责任边界

```text
Vue SPA (Drive / Share / Tasks / Settings)
  ↓ REST + WebSocket
fnOS Gateway (NAS session, Unix socket, /app/quarkdrive)
  ↓
Go API (admin middleware, request validation, origin/CSRF)
  ├─ QuarkAdapter [认证、文件目录、搜索、分享解析/转存、获取下载源]
  ├─ TransferOrchestrator [分享转存批次、云端临时目录归属、容量、清理]
  ├─ TaskScheduler [持久化状态、调度、公平性、全局限流与重试]
  ├─ DownloadEngine [Range 探测、分片、限速、文件发布与校验]
  ├─ FnOSStorage [授权目录、ACL 检查、磁盘空间、路径安全]
  ├─ SQLiteRepository [事务、迁移、WAL、恢复]
  └─ EventHub [WebSocket 事件]
```

核心约束：下载引擎不知道夸克 API；它通过 `SourceResolver` 获取一个短期有效的 DownloadSource。在夸克临时地址失效时调用 Resolver 刷新。分享转存逻辑不直接执行 HTTP 下载。

## 3. 双来源统一抽象

```go
type SourceKind string
const (
    SourcePersonal SourceKind = "personal"
    SourceShare    SourceKind = "share"
)

type RemoteFileRef struct {
    Kind SourceKind
    FileID string        // 账号云盘 file ID 或转存后的 file ID
    ShareID string       // 仅分享任务需要
    ShareFileID string   // 分享目录中的原文件 ID
    RelativePath string // 只允许校验后的相对路径
    ExpectedSize int64
}

type DownloadSource struct {
    URL string           // 只能保存在运行态，不允许发送到浏览器或日志
    Headers map[string]string
    ExpiresAt time.Time
    ExpectedSize int64
    ETag string
    LastModified string
    ProviderFingerprint string
}

type SourceResolver interface {
    Resolve(ctx context.Context, ref RemoteFileRef) (DownloadSource, error)
    Refresh(ctx context.Context, ref RemoteFileRef) (DownloadSource, error)
}
```

安全要求：源 URL、凭据、Authorization/Cookie Headers 不进入任务列表 API、日志、SQLite 明文记录。分享链接首先离线解析，仅允许已知的夸克分享域名与 URL 形态；服务端取回的直链和所有重定向地址必须进行 HTTPS、公网目标、DNS 重绑定/SSRF 等验证。

## 4. 个人网盘下载时序

1. 管理员打开应用并扫码登录，后端获取登录态并加密落盘。
2. `GET /files` 分页列目录，`GET /files/search` 全盘搜索。
3. 用户选择文件/文件夹以及已授权 NAS 目录，`POST /tasks` 创建任务。
4. 后端异步递归展开目录，校验目标路径/容量，写入 `download_files`。
5. 对每个文件获取短期下载地址、探测 Range 支持、执行分片下载。
6. 校验后原子发布最终文件，更新任务进度并推送事件。
7. 无云端转存，因此不会创建或删除夸克网盘文件。

## 5. 分享链接下载时序

1. 用户提交分享 URL 和提取码；后端解析分享 ID、验证有效性，产生临时预览 ID。
2. 前端通过预览 ID 分页浏览分享文件树并批量选择；禁止客户端伪造任意文件 ID。
3. 创建任务并固定目标授权目录/冲突策略。后端按照夸克账户剩余可用容量选择可转存批次。
4. 在应用专属临时云盘目录中转存并持久化记录转存文件 ID 与任务归属，等待服务端转存完成。
5. 由同一下载引擎下载；直链失效可通过保留的转存文件 ID 重新取链。
6. 文件完成且校验通过后，清理**仅由本应用创建并确认归属**的云端临时文件。清理失败进入待清理队列，不回滚本地已完成任务。
7. 对于单个文件大于可用转存空间、分享过期、服务端限制或认证失效，进入可解释的 BLOCKED/FAILED 状态；绝不保证全部分享都可转存。

约束：临时转存文件不能在取到链接后就删除，否则破坏直链刷新和可靠续传。为节约网盘空间，可在**该文件**完全下载并验证后及时清理，而不是等待整个任务组结束。

## 6. 调度与状态机

任务状态：`CREATED -> ENUMERATING -> QUEUED -> RESOLVING -> DOWNLOADING -> VERIFYING -> COMPLETED`。

分支：`PAUSING -> PAUSED`、`RETRY_WAIT`、`BLOCKED_AUTH`、`BLOCKED_SPACE`、`BLOCKED_PERMISSION`、`BLOCKED_REMOTE`、`FAILED`、`CANCELLED`。

- 仅通过服务层完成合法状态转移，数据库以事务校验版本号（乐观锁）；重复请求必须幂等。
- 暂停：先停止分片派发与当前请求，等待数据刷盘并提交 checkpoint，然后返回成功；用户主动暂停不自动恢复。
- 重启：`DOWNLOADING/RESOLVING/VERIFYING` 恢复为待核验队列，逐文件验证源身份和本地 checkpoint，恢复尚未确认的分片。
- 错误分类：网络/5xx 指数退避；429 尊重 Retry-After；401/403 先检查登录态再尝试刷新；404、分享过期、源对象变化提示不可自动恢复；空间/权限进入 BLOCKED。
- 并发默认：同时任务数 3，单文件连接数 4，HTTP 请求总上限 12，分片 8MiB；独立设置全局带宽限制和文件级并发。
- 节流：WebSocket 最大每任务 1 次/秒；断线后重新从 REST 拉全量快照。

## 7. 断点续传可靠性规则

- 分片以闭区间 `[start,end]` 表示，HTTP 响应必须为 `206` 且 Content-Range 匹配；返回 `200` 不能写入分片偏移。
- 对不支持可靠 Range 的服务器降级串行下载；不支持恢复时明确提示需从头下载。
- 完整写入分片后调用必要的持久化机制，成功后才将该分片记为 COMPLETE；保证数据库的 COMPLETE 不会领先于持久化数据。
- 重启时遇到含糊的进行中分片，丢弃/重校验该片而不是直接信任之前写入的字节数。
- 临时文件存放在目标目录下的 `.quarkdrive-part/` 中，避免跨文件系统发布；完成后 `fsync`、文件长度及可用远端校验值检查，再执行不覆盖已有文件的原子 rename；冲突处理在提交点再检查一次。
- 断点续传必须验证远程对象 ID、大小，以及可用的 ETag/Last-Modified/其他源指纹。无可靠对象身份或哈希时不能承诺强一致的内容校验；源变化或无法确认身份时保守重下。
- 限速与并发只是客户端上限，不保证突破夸克账号权益、服务端限速或 CDN 限制。

## 8. 数据库草案（SQLite）

核心表：

- `accounts(id, provider, display_name, state, encrypted_credentials, updated_at)`
- `download_tasks(id, kind, state, target_location_id, target_subdir, conflict_policy, total_bytes, complete_bytes, error_code, attempt, created_at, updated_at, state_version)`
- `download_files(id, task_id, remote_ref_json, relative_path, expected_size, object_fingerprint, temp_path, state, completed_bytes)`
- `download_chunks(id, file_id, start_byte, end_byte, state, digest, updated_at)`
- `share_transfers(id, task_id, share_id, preview_ref, temp_folder_id, transferred_file_id, ownership_marker, state, cleanup_state)`
- `storage_locations(id, path, authorization_ref, last_checked_at, is_default)`
- `app_settings(key, value_json, updated_at)`
- `schema_migrations(version, checksum, applied_at)`

索引：`download_tasks(state, created_at)`、`download_files(task_id, state)`、`download_chunks(file_id, state)`、`share_transfers(cleanup_state)`。对 `task_id + 规范化相对路径` 做唯一性保护，冲突策略交给业务层处理。

WAL、foreign_keys=ON、busy_timeout 配置，关键状态用事务。客户端下载任务需提供 `Idempotency-Key`，避免重复点击创建重复任务。会话密钥文件权限 0600，密钥与密文分离存放；SQLite 备份应包含完整的事务一致性快照。

## 9. API 契约草案

前缀统一：`/app/quarkdrive/api/v1`；WebSocket `/app/quarkdrive/ws`。

| 方法 | 资源 | 说明 |
|---|---|---|
| POST | `/auth/qr-sessions` | 创建扫码会话，返回临时 ID、二维码图像/数据、过期时间 |
| GET | `/auth/qr-sessions/{id}` | WAITING/SCANNED/SUCCESS/EXPIRED/FAILED |
| GET | `/auth/account` | 当前账号（不含 Cookie） |
| DELETE | `/auth/account` | 退出并安全处理关联任务 |
| GET | `/files?parent_id=&cursor=` | 分页文件浏览 |
| GET | `/files/search?q=&cursor=` | 关键词搜索 |
| POST | `/shares/previews` | 解析分享，返回短期 preview ID |
| GET | `/shares/previews/{id}/files` | 分页展示分享目录 |
| POST | `/tasks` | 创建个人/分享任务，携带 `source_kind`、受服务器校验的选择与 `target_location_id` |
| GET | `/tasks` | 列表筛选分页 |
| GET | `/tasks/{id}` | 详情与文件明细 |
| POST | `/tasks/{id}/pause` | 幂等暂停 |
| POST | `/tasks/{id}/resume` | 幂等恢复 |
| POST | `/tasks/{id}/retry` | 重试可恢复失败 |
| DELETE | `/tasks/{id}` | 删除任务记录（默认不删除已下载文件） |
| GET/PUT | `/settings` | 参数读取与更新 |
| GET | `/storage/locations` | 列出已授权目录 |
| POST | `/storage/locations/validate` | 验证已授权的目录标识，不接受随意绝对路径 |
| GET | `/healthz` | 仅经网关返回有限运行状态 |

写接口仅管理员可用，校验 `Origin` 和 CSRF，同一网关 Cookie 不能代替 CSRF 防护。Unix Socket 文件对其他本机进程严格限制权限；来自任意 TCP 入口的伪造网关 Header 不受信任。错误响应采用 `{code,message,request_id,details}`，敏感信息脱敏。

## 10. 前端页面结构

- **我的网盘**：顶部账号状态、路径面包屑、搜索、目录表格、批量选择、目标目录选择、创建下载。
- **分享下载**：输入分享地址+提取码、解析状态、目录树、选择项和大小、目标目录、转存/空间提示。
- **下载任务**：全部/下载中/已完成/失败，进度速度、连接数、耗时、暂停/恢复/重试/清理记录，任务内文件细节。
- **设置**：扫码账号、默认授权位置、并发连接数、全局限速、空间预留、同名策略、清理策略、诊断日志。
- UI 原则：紧凑、响应快；不要大面积无意义卡片；目录表格支持虚拟滚动；所有用户可见错误明确解释可执行的修复操作。

## 11. FPK 打包契约

项目结构：

```text
quarkdrive/
  frontend/
  backend/
    cmd/quarkdrive/
    internal/{api,auth,quark,transfer,scheduler,download,storage,repository,security}/
    migrations/
  packaging/fnos/
    manifest.template
    config/{privilege,resource}
    app/ui/{config,images/}
    cmd/{main,install_init,install_callback,upgrade_init,upgrade_callback,uninstall_init,uninstall_callback,config_init,config_callback}
    wizard/
    ICON.PNG
    ICON_256.PNG
  scripts/{build.sh,package-fpk.sh,verify-fpk.sh}
  tests/{quark,download,fnos}/
  docs/
  .github/workflows/
```

`app/ui/config` 入口示例（需实机验证）：

```json
{".url":{"quarkdrive.main":{"title":"QuarkDrive","icon":"images/icon_{0}.png","type":"iframe","protocol":"","gatewayPrefix":"/app/quarkdrive","gatewaySocket":"app.sock","url":"/app/quarkdrive","allUsers":false}}}
```

`config/privilege`：

```json
{"defaults":{"run-as":"package"},"username":"quarkdrive","groupname":"quarkdrive"}
```

`manifest` 中：`appname=quarkdrive`、`version=0.1.0`、`source=thirdparty`、`platform=x86`/`platform=arm`、`desktop_uidir=ui`、`desktop_applaunchname=quarkdrive.main`、`ctl_stop=true`、`checkport=false`、`disable_authorization_path=false`。`os_min_version` 只写经过验证的最小版本；不要把示例直接当最终包。

- 构建：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ...` 和 `GOARCH=arm64`（使用纯 Go SQLite 驱动）；前端 `npm ci && npm run build`；复制程序到对应 FPK `app/bin`、静态资源到 `app/www`。
- 应用 `cmd/main` 实现 start/stop/status；使用 PID 与实际进程验证，避免 `pkill -f` 误杀；优雅停止分片，及时持久化 checkpoint。
- `${TRIM_APPDEST}`：可替换程序、前端、Socket；`${TRIM_PKGETC}`：配置和密钥；`${TRIM_PKGVAR}`：SQLite、迁移信息、日志；用户文件存在授权数据目录。
- 默认下载目录可以通过 `config/resource` 声明单个应用共享目录，或通过 fnOS 的授权目录功能首次选择；任意下载位置仅接受平台授予权限的路径。
- 打包使用官方 `fnpack build --directory <staging>`，产出两个架构专用 `.fpk`，校验架构、包内容、可执行位和 SHA256，随后在真实 AMD64 与 ARM64 设备安装。
- `upgrade_init/upgrade_callback` 完成版本兼容检查、事务一致性备份、可回滚迁移；卸载不默认删除 NAS 用户下载文件。

## 12. 分阶段开发（任务拆分与退出条件）

### P0 — 平台与夸克接口 PoC（先决条件）

- fnOS 最小 Native FPK：网关访问、管理员身份、Unix Socket、启停、文件夹授权、升级保留数据。
- 夸克登录：二维码生成/轮询/扫码确认/凭据验证。
- 个人网盘：分页目录、搜索、单文件取链下载。
- 分享：分享链接 + 提取码、文件列表、指定文件转存、取链、清理标记。
- Range：206、Content-Range、过期直链重新获取、实际文件长度核对。
- 输出 PoC 记录、真实响应样例（脱敏）、失败模式清单。**若扫码、转存或授权 API 任一失败，不进入完整开发，先改适配策略。**

### P1 — 工程骨架

前后端工程、数据库迁移、统一日志和错误码、模拟 QuarkAdapter、静态页面与网关打通、CI 测试。可安装空壳 FPK。

### P2 — 个人网盘闭环

扫码认证、目录/搜索、创建任务、单文件/目录树下载、任务持久化。验收：关闭网页不中止后台任务。

### P3 — 分享下载闭环

解析预览、分批转存、云端临时文件归属、统一下载器接入、成功后安全清理。验收：转存与本地完成的状态保持一致，失败不误删。

### P4 — 可靠性与安全

Range 并发、速率控制、崩溃恢复、动态刷新直链、磁盘不足、路径穿越、同名冲突、SSRF、鉴权与重放测试。验收：长时间运行、进程崩溃、断网与服务端异常均有可解释恢复路径。

### P5 — 双架构发包

FPK 生命周期、授权管理、升级保留、自动构建、文档、两个架构实机验证。产物：`QuarkDrive_v0.1.0_amd64.fpk`、`QuarkDrive_v0.1.0_arm64.fpk`、SHA256、Release notes。

## 13. 必须通过的测试矩阵

1. 扫码成功、二维码过期、Cookie 失效、退出登录、重新扫码。
2. 中文/Unicode/空文件/同名/深层目录；分页与部分勾选。
3. 分享提取码、失效分享、分享限额、容量不足、单个超大文件。
4. Range 206 正常、200 忽略 Range、416、不匹配 Content-Range、301/302 CDN 重定向。
5. 限流 429、502/503、网络超时、直链过期、文件源身份变化。
6. 按下暂停、关闭浏览器、停止应用、kill -9、NAS 重启后恢复。
7. 下载过程中撤销目录授权、磁盘拔出、空间预留触发。
8. 文件路径穿越、符号链接逃逸、HTTP 重定向到内网、伪造请求 Header。
9. 安装、停止、重启、卸载、应用升级、Schema 升级后的数据保留。
10. AMD64 和 ARM64 独立实机验证；不能只靠交叉编译推断可用性。

## 14. 风险、前提与验收口径

- 夸克第三方接口非稳定公开 SDK：已有社区实现只能证明技术路线值得验证，不能代替 2026-10-14 当日真实账号 PoC。
- 夸克的账号权益、服务端并发限制及条款会影响实际下载结果；不设计绕过机制。
- 没有服务端哈希/对象指纹时，不能向用户承诺强完整性验证；必须报告校验等级。
- Linux 文件路径与 fnOS 文件共享 ACL 并非等同于可写权限；每次任务创建和最终文件提交前均验证授权。
- v0.1 发布完成的定义不是“能打包”，而是双入口下载成功、恢复测试通过、两架构安装验证通过且不能误删用户文件。

## 15. 参考资料

- fnOS 开发者站：https://developer.fnnas.com/docs/guide
- fnOS 官方文档镜像（含统一网关、manifest、权限、生命周期）：https://github.com/ckcoding/fnnas-docs/blob/main/ALL_DOCS.md
- QuarkPan（Python 客户端，参考扫码及网盘 API）：https://github.com/lich0821/QuarkPan
- gopeed-extension-quark（参考分享转存、分批与直链刷新）：https://github.com/iGwkang/gopeed-extension-quark

此文档是设计规格和待验证工作清单；尚未完成代码实现或 NAS 实机测试。
