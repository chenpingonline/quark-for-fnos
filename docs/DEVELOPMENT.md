# 开发指南

[返回项目首页](../README.md) · [打包规范](PACKAGING.md) · [工程状态](STATUS.md)

## 技术与依赖


前端为自包含 **HTML / CSS / JavaScript**，后端使用 **Go**，通过 HTTP API 通信。任务界面每秒轮询；应用状态使用原子写入的 JSON，目前未使用 Vue、SQLite 或 WebSocket。

构建需要 Go 1.23+、Node.js（检查前端）、Python 3、bash 与官方 fnpack；这些开发工具不需要安装到运行应用的 NAS 中。

以下构建命令在仓库根目录执行。

```bash
# 获取官方打包工具
./scripts/bootstrap-fnpack.sh

# 测试、交叉编译、官方打包与审计
./scripts/package-fpk.sh

# 校验当前源码对应的安装包
./scripts/verify-fpk.sh dist/QuarkDrive_*.fpk
```

`packaging/fnos/manifest` 是唯一发布版本源。脚本把版本注入 Go 程序、包内前端和 FPK 文件名；生成新交付物前递增补丁版本，不覆盖已交付包。工具保存在 `.tools/`，中间产物位于 `build/`，FPK、SHA-256 和本地审计记录位于 `dist/`。历史 `_UNVERIFIED.fpk` 不属于官方构建流程。详见 [打包规范](PACKAGING.md)。

### 本地预览

```bash
cd backend
QUARKDRIVE_DEV=1 QUARKDRIVE_DATA_DIR=./data QUARKDRIVE_WEB_DIR=../frontend go run .
# 浏览器访问 http://127.0.0.1:18088/
```

开发模式绕过飞牛管理员认证，仅监听本机回环地址，不能通过端口转发或代理暴露到外部。正式运行使用 `/app/quarkdrive` 统一网关与 Unix Socket，并校验飞牛管理员身份及应用请求标记。

### 目录结构

```text
backend/           Go API、夸克适配、下载引擎、持久化与测试
frontend/          页面、样式、图标和本地二维码组件
packaging/fnos/    应用清单、权限、共享目录、生命周期和桌面入口
scripts/          构建、官方 fnpack 打包与校验
tests/            前端、打包和 Linux 生命周期测试
docs/             界面截图、API、安全、工程状态与验收文档
```

