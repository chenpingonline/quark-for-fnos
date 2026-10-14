# QuarkDrive v0.1 需求实现审计（基于源码静态检查）

审计日期：2026-10-14  
对照：`QuarkDrive_fnOS_v0.1_Architecture.md` 与用户确认的八组 v0.1 功能。  
代码版本：本报告随 `v0.1.1-preview` 网关问题修复源码提供。  

**总体结论：未完成全部需求。** 已有个人/分享两种入口及下载内核的实验性实现；不满足原始技术栈、数据持久化、网关正式验收和部分可靠性要求。静态存在不等于已在真实夸克账户上成功。

| 需求 | 代码中已具备 | 仍缺少/验证不足 | 判定 |
|---|---|---|---|
| 扫码登录、账号信息、登录状态检查 | `backend/quark.go` 中二维码会话创建/查询、账号昵称和 UID、加密 Cookie | v0.1.0 因 Origin/Host 比对被阻止；v0.1.1 修复尚未在 NAS 确认；网盘容量展示未实现；二维码/账户接口真实兼容性待验证 | 部分实现 |
| 文件浏览、搜索、批量选择 | `backend/quark.go` 中分页查询、搜索；`frontend/assets/app.js` 跨页勾选；`backend/main.go` 递归处理目录 | 夸克真实数据格式未核对；原始规格中的类型过滤、虚拟表格未实现 | 基本代码存在，未实测 |
| 分享解析、文件树、转存、下载 | `backend/quark.go` 的分享令牌、分页文件、转存及删除；与统一下载器相连 | 没有容量感知的分批转存、临时转存清理队列和重启后远端遗留目录对账；真实接口未验证 | 部分实现 |
| NAS 授权目录、层级保留 | `packaging/fnos/config/resource` data-share；`backend/main.go` 限制选择已知目录；相对路径保护 | 不支持通过 fnOS 开放授权 API 交互选目录；创建任务时无法独立选择目标目录；符号链接 TOCTOU 风险尚需设备安全测试 | 部分实现 |
| 暂停、恢复、进度、速度、自动重试 | `backend/download.go` 任务队列、暂停/继续、按秒速度、重试；UI 轮询 | 无 WebSocket；UI 无任务删除/详情；某些状态只置为 FAILED；设置并发数后正在运行的调度信号量不动态调整 | 部分实现 |
| 可靠断点续传、刷新直链、完整性校验 | Range 校验、分片 SHA-256、重启 checkpoint 复核、文件整体 SHA-256、短链重取 | 无云端可信哈希时无法强确认内容一致；某些过期 URL 在探测阶段失败且不自动刷新；未实测断电/长时间任务/真实 CDN | 部分实现 |
| 并发数、连接数、限速、磁盘预留 | `Settings` 字段与输入校验、带宽时序、磁盘余量计算 | 缺少独立全局 HTTP 请求连接数上限；并发更改需重建调度器/重启；磁盘中途不足、并行空间预算及 429 Retry-After 未完整处理 | 部分实现 |
| x86_64/ARM64 FPK、启停、升级保留 | `scripts/build.sh` 交叉编译；`packaging/fnos/cmd/main` 启停；数据保存目录 | 先前候选 FPK 非官方 `fnpack` 构建；未分别验证安装/升级、卸载保留数据；升级脚本空实现，无数据库版本迁移 | 部分实现，未正式验收 |

## 与既定工程架构不一致

- 前端是 **原生 HTML/CSS/JavaScript**，未使用 Vue 3、TypeScript、Vite、Pinia、Naive UI。
- 后端使用 Go 标准 `net/http`，没有 Gin 模块化分层。
- 状态持久化为原子写入 `state.json`（凭据加密），**不是 SQLite/WAL/事务/迁移**。
- 前端每约 2.5 秒拉取任务，**不是 WebSocket**。
- 单 Go 包拆分为 `main.go` / `model.go` / `quark.go` / `download.go`，并非原方案的 `internal/{auth,download,...}` 层。

## v0.1.1 缺陷修复与安全边界

原 `main.go` 对所有写请求直接比较 `Origin.Host == r.Host`。飞牛网关将浏览器请求转发到 Unix Socket 时，`Host` 可能改变，导致二维码 POST 被拒绝并显示“请求来源不匹配”。修复行为：

1. 浏览器提供 `Sec-Fetch-Site: same-origin` 且后端通过生产 Unix Socket 网关运行时，允许 Origin 与内部 Host 不同；开发模式仍要求严格 Host 匹配。
2. `cross-site`、`same-site`、`none` 一律拒绝；伪造/非法 Origin 拒绝；仍要求 `X-Trim-Isadmin` + `X-Trim-Userid` + `X-QuarkDrive-Request: 1`。
3. 如果浏览器/网关未传递 Fetch Metadata、同时后端收到被重写的 Host，仍将保守拒绝；不能为兼容性无条件放宽校验。需要在设备上记录**非敏感** `Origin`、`Host`、`Sec-Fetch-Site` 头以判断后续兼容策略，勿记录 Cookie/Token。
4. 修复只验证本地 Go HTTP 测试，不代表实际夸克接口成功扫码，也不证明完整 fnOS FPK 兼容。

## 优先级与下一步

- **P0（先恢复可用）**：在测试 fnOS 安装更新版本后确认二维码 POST 不再报 403；观察夸克二维码响应并完成真实登录。
- **P0（完成实际闭环）**：测试个人文件、分享转存、长文件下载；确认 App 用户的实际 NAS 写入权限。
- **P1（满足文档承诺）**：SQLite/WAL 和迁移、Vue 3/TS/Vite、WebSocket、官方 fnpack 以及双架构设备安装/升级。
- **P1（可靠性）**：远端身份稳定性、临时文件对账、容量感知分批转存、拒绝覆盖与恢复演练、动态并发和 Retry-After。

本报告为**静态源码审计 + 本地测试**。截图只确认某设备能启动前端，并确认 v0.1.0 存在请求来源错误；无法从截图证明其他功能可用。
