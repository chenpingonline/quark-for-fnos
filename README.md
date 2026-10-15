<div align="center">

<img src="packaging/fnos/ICON_256.PNG" alt="QuarkDrive" width="112" />

# QuarkDrive for fnOS

**运行在飞牛 fnOS 上的原生夸克网盘下载器**

扫码连接夸克账号，浏览个人网盘与分享文件，将文件直接下载到 NAS。

[![Release](https://img.shields.io/github/v/release/chenpingonline/quark-for-fnos?include_prereleases&display_name=tag)](https://github.com/chenpingonline/quark-for-fnos/releases)
[![Downloads](https://img.shields.io/github/downloads/chenpingonline/quark-for-fnos/total)](https://github.com/chenpingonline/quark-for-fnos/releases)
![fnOS](https://img.shields.io/badge/fnOS-x86__64%20%7C%20ARM64-2ea44f)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[下载安装包](https://github.com/chenpingonline/quark-for-fnos/releases) · [安装与使用](#安装与使用) · [界面预览](#界面预览) · [问题反馈](https://github.com/chenpingonline/quark-for-fnos/issues) · [打包规范](docs/PACKAGING.md)

</div>

![QuarkDrive 个人网盘：文件列表、批量选择和紧凑下载工具栏](docs/images/drive-light.jpg)

> 页面截图来自当前应用的本地预览，使用示例账号、文件和任务数据；截图中的速度不代表实际下载性能。

## 项目简介

QuarkDrive 是面向 **飞牛 fnOS** 的独立第三方夸克网盘下载工具。通过飞牛桌面打开应用，扫码登录后即可浏览个人网盘、解析分享链接、选择文件或文件夹，并交给 NAS 后台下载。

- **原生运行**：提供 x86_64、ARM64 两种 FPK，运行不需要 Docker、Node.js、Python、Redis 或外部 aria2。
- **后台下载**：关闭页面后下载任务仍继续，可查看进度、速度、失败原因并暂停或继续任务。
- **简洁界面**：文件夹单击进入，目录文件在同一列表滚动查看；选中文件后显示下载按钮，搜索框按需展开。

> [!NOTE]
> 当前为实验性预发布版本。项目非夸克官方应用，需要有效夸克账号；私有接口、账号权益及服务端策略可能影响可用性。构建和本地测试通过不等于完整飞牛真机验收，具体范围见 [实现与验证状态](docs/STATUS.md)。

## 功能

| 模块 | 功能 |
| --- | --- |
| 账号管理 | 夸克 App 扫码登录、账号状态检查、退出登录，本地凭据加密保存 |
| 我的网盘 | 文件夹单击进入、文件搜索、名称/时间/大小排序、批量选择及文件夹递归下载 |
| 分享下载 | 解析分享链接和提取码、浏览目录、选择文件并下载到 NAS |
| 文件列表 | 自动读取目录的全部 API 页，合并到同一个滚动列表；目录内容变化或加载异常时明确提示 |
| 下载任务 | 查看全部、进行中、已完成、失败和暂停任务，支持暂停、继续、失败重试及任务详情 |
| 速度统计 | 持续统计传输字节，显示最近四秒平均速度；任务每秒刷新，下载中的零速度显示为 `0 B/s` |
| 分片与续传 | HTTP Range 并发下载、已完成分片断点记录、分片与完整文件 SHA-256 校验、下载地址重新获取 |
| 下载设置 | 同时任务数、单文件连接数、分片大小、全局限速、重试次数和磁盘空间预留 |
| 存储路径 | 选择飞牛已授权给应用的共享下载目录 |
| 界面主题 | 浅色、深色和自动模式，适配不同窗口宽度 |

## 界面预览

以下均为示例数据，不包含真实账号凭据或私人文件。点击图片可查看完整尺寸。

### 下载任务 · 浅色模式

查看任务进度、速度、状态与失败原因，在列表或任务详情中暂停、继续和重试。

![QuarkDrive 浅色下载任务页面](docs/images/tasks-light.jpg)

<details>
<summary><strong>分享下载与应用设置</strong></summary>

### 分享下载

输入分享链接与提取码后浏览文件，选择需要下载的内容。

![QuarkDrive 分享下载页面](docs/images/share-light.jpg)

### 应用设置

顶部标签切换设置分类，下载参数采用左侧名称、右侧输入框的紧凑布局。

![QuarkDrive 下载设置页面](docs/images/settings-light.jpg)

</details>

## 支持平台与下载

从 [GitHub Releases](https://github.com/chenpingonline/quark-for-fnos/releases) 下载适合 NAS CPU 架构的安装包。预发布版本也在此页面列出。

| NAS 架构 | 安装包文件名 |
| --- | --- |
| Intel / AMD 64 位，x86_64 / amd64 | `QuarkDrive_<版本>_x86_64.fpk` |
| ARM 64 位，ARM64 / aarch64 | `QuarkDrive_<版本>_arm64.fpk` |

- 包声明的最低系统版本为 **fnOS 1.1.3100**，仍需在具体设备与系统版本上验证兼容性。
- 使用飞牛管理员账号安装和访问，NAS 需要能够连接夸克相关服务。
- 两种架构独立打包，不提供单架构二进制伪装的 `all` 通用包。

## 安装与使用

1. 在 [Releases](https://github.com/chenpingonline/quark-for-fnos/releases) 下载匹配架构的 FPK。
2. 打开飞牛 **应用中心 → 手动安装**，选择安装包，按系统提示完成安装与共享目录授权。
3. 从飞牛桌面打开 **QuarkDrive**，使用夸克 App 扫码登录。
4. 在 **我的网盘** 中单击进入文件夹，勾选需要的文件或文件夹，点击顶部 **下载** 并确认 NAS 下载目录。
5. 下载分享文件时，进入 **分享下载**，粘贴链接、填写提取码并解析，然后选择内容下载。
6. 在 **下载任务** 查看进度、暂停/继续任务或检查失败原因；下载参数和存储路径可在 **应用设置** 中调整。

关闭页面不会停止 NAS 后台下载。修改“同时下载任务数”后，需要重启应用后台服务才能按新并发数调度。

### 升级与数据

在应用中心安装更高版本的对应架构 FPK，使用升级流程保留现有应用数据和下载目录。升级前建议备份重要数据；完整设备升级验收仍待完成。

- 应用状态位于 `$TRIM_PKGVAR/data`，账号凭据使用 AES-256-GCM 与本地密钥加密保存；拥有 NAS root 权限仍可读取本机数据与密钥。
- 下载写入飞牛授权的 `data-share` 目录，不接受任意未授权绝对路径。
- 未完成下载及有效断点记录位于下载目录下的 `.quarkdrive-part/`，不要在续传前删除。
- 分片只有在写盘、同步并校验后才记为可续传；速度统计与这些断点记录分开。
- 分享下载会先把文件转存到账号的 `QuarkDriveTemp` 临时目录；启用清理后，仅在下载成功后删除本应用转存的对应文件。失败或中断时保留云端副本。
- 不支持 Range 的来源会降级为单连接下载；当前该降级模式中断后从头开始。

## 常见问题

### 下载失败或 HTTP 400

先查看任务列表的失败原因和任务详情。应用已适配桌面客户端下载地址请求，并保持直链获取与实际下载的客户端标识一致；这不保证所有账号、文件和服务端策略都可用。反馈时请提供应用版本、任务状态和错误码，不要提供 Cookie、扫码凭据或带签名的下载直链。

### 为什么速度和进度不完全同步？

速度按持续传输的数据计算最近四秒平均值；进度与断点记录按已经完成并校验的分片更新，因此分片尚未完成时，进度可能暂时不变，但仍显示下载速度。持续无数据时，平均速度会逐步降至 `0 B/s`；暂停或完成后显示 `—`。

### 是否支持突破账号权益或服务端限制？

本应用只提供已登录账号的文件浏览和下载能力，不提供账号、会员权益或下载线路。账号权益、限速、空间、风控和下载地址有效期仍受夸克服务约束。

### 目前验证到了哪一步？

Go 测试、race/vet、前端语法及主题/列表测试、模拟 API 浏览器流程、官方 fnpack 构建与包内校验均已覆盖。ARM64 隔离 Linux 容器验证了非 root 启停、Unix Socket、资源路由和数据保留。真实 fnOS 安装升级、宿主主题切换、账号接口与长期大文件下载仍需持续验证，见 [工程状态](docs/STATUS.md) 和 [手动验收清单](docs/manual-acceptance.md)。

## 开发与打包

前端为自包含 **HTML / CSS / JavaScript**，后端使用 **Go**，通过 HTTP API 通信。任务界面每秒轮询；应用状态使用原子写入的 JSON，目前未使用 Vue、SQLite 或 WebSocket。

构建需要 Go 1.23+、Node.js（检查前端）、Python 3、bash 与官方 fnpack；这些开发工具不需要安装到运行应用的 NAS 中。

```bash
# 获取官方打包工具
./scripts/bootstrap-fnpack.sh

# 测试、交叉编译、官方打包与审计
./scripts/package-fpk.sh

# 校验当前源码对应的安装包
./scripts/verify-fpk.sh dist/QuarkDrive_*.fpk
```

`packaging/fnos/manifest` 是唯一发布版本源。脚本把版本注入 Go 程序、包内前端和 FPK 文件名；生成新交付物前递增补丁版本，不覆盖已交付包。工具保存在 `.tools/`，中间产物位于 `build/`，FPK、SHA-256 和本地审计记录位于 `dist/`。历史 `_UNVERIFIED.fpk` 不属于官方构建流程。详见 [打包规范](docs/PACKAGING.md)。

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
scripts/           构建、官方 fnpack 打包与校验
tests/            前端、打包和 Linux 生命周期测试
docs/             界面截图、API、安全、工程状态与验收文档
```

## 历史变更与开发文档

- **v0.1.14**：紧凑下载工具栏与按需搜索，整合单页文件列表、客户端请求适配、四秒平均速度和官方分架构打包。
- **v0.1.13**：速度统计与断点记录分开，下载中零速度明确显示，任务每秒刷新。
- **v0.1.12**：下载地址和实际下载统一桌面客户端标识。
- **v0.1.11**：修复单页目录模块缺少后端静态资源路由的问题。
- **v0.1.7**：增加下载兼容请求头、Cookie 域名限制和失败原因诊断。
- **v0.1.6**：历史版本使用双击进入文件夹，现已改为单击。
- **v0.1.3**：四个页面、扫码弹窗、任务详情与主题重构；参见 [UI 变更记录](docs/UI_REDESIGN_v0.1.3.md)。
- **v0.1.2**：自动、浅色、深色主题设置，以及独立浏览器兜底；同源宿主主题路径仍需真实客户端验证。
- **v0.1.1**：修复统一网关后端 Host 与浏览器 Origin 不一致的问题，同时保留跨站、管理员及应用标记检查；登录失败在扫码弹窗内提示。

更多说明：[HTTP API](docs/API.md) · [安全说明](docs/security.md) · [实现与验证状态](docs/STATUS.md) · [需求审计](docs/REQUIREMENTS_AUDIT.md)。SQLite/WAL、规模化故障测试、完整宿主接入及分享令牌生命周期等后续事项在工程状态中维护。

## 许可证

项目原创代码采用 [MIT License](LICENSE)。本地二维码组件的版权和许可说明保存在 [第三方许可](frontend/vendor/LICENSES.txt)。QuarkDrive 与夸克官方无隶属关系，请遵守服务条款并仅处理有权访问的文件。
