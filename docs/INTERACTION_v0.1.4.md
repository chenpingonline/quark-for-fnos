# QuarkDrive v0.1.4：文件夹交互调整

- 我的网盘、分享下载两个文件列表移除冗余的“操作 / 打开”列。
- 文件夹双击所在行（图标、名称或行内空白处）进入子目录；也可聚焦文件夹行后按 Enter 打开。
- 单击文件夹行不跳转，勾选框独立用于多选和下载。
- 通过面包屑、返回上一级进入父目录；原有批量下载、分页、搜索及任务 API 不变。
- 类型、大小、修改时间字段保持原样，文件名称列使用腾出的宽度，并继续安全截断超长名称。

## 本地测试

- `python tests/ui_smoke.py --screenshots`：验证双击、单击、键盘 Enter、返回、分享目录双击、复选框、任务和主题流程。测试使用 Mock API。
- `cd backend && go test ./... && go test -race ./... && go vet ./...`：后端回归测试。
- `bash scripts/package-fpk.sh`、`bash scripts/verify-fpk.sh dist/packages/QuarkDrive_v0.1.4_*_UNVERIFIED.fpk`：本地构建与包结构校验。

## 限制

生成的 FPK 在当前环境未经过官方 fnpack 或真实 fnOS 安装/升级测试。账号、分享接口及真实下载能力未在此次 UI 调整中做实测。
