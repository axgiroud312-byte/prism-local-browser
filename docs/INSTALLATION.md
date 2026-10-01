# 棱镜浏览器 · Windows 开发预览

本包是 **0.3.0 开发预览**，仅交付本机桌面工作台和 SQLite 环境配置。可新建、编辑和重开档案；**尚无真实 fingerprint-chromium、代理、Cookie 或完整备份能力**。WebView2 只是桌面页面宿主，不是产品浏览器内核。

## 安装与启动

1. 使用 Windows 10/11 x64，安装微软 **WebView2 Evergreen Runtime**（94.0.992.31 或更新版本）：<https://developer.microsoft.com/microsoft-edge/webview2/>。
2. 核对安装包旁的 SHA256SUMS 和 release.json，再双击 `prism-browser-0.3.0-preview.1-windows-amd64-setup.exe`。
3. 当前构建**未签名**，Windows 可能显示 SmartScreen/未知发布者提示。不要关闭系统安全功能；仅在确认来源和哈希后自行决定是否运行。
4. 安装仅作用于当前 Windows 用户，不需要管理员权限。通过桌面或开始菜单“棱镜浏览器 · 开发预览”打开。

缺失/过旧 WebView2 时，安装或启动明确阻断并给出官方链接；不会静默下载、重置数据或改用网页示例。已有 WebView2 时可离线安装和使用本机配置能力。

## 程序与数据

- 程序：`%LOCALAPPDATA%/Programs/PrismBrowserPreview/versions/<预览版本>/`。
- 用户数据：`%LOCALAPPDATA%/PrismBrowser/app.db`；工作台 WebView 在同根 `workbench-webview/`。
- `PrismBrowser.maintenance.lock` 是同用户安装/运行独占锁，不是档案数据；进程退出自动释放。
- 不从原型 localStorage 或原型 JSON 导入真实配置，不用真实账号/代理/Cookie 测试本预览。

## 升级与卸载

升级前正常关闭工作台，运行更新版本安装包。安装新增独立版本目录，保留旧程序和用户数据；拒绝自动降级。升级后从原快捷方式读取同一个工作区，不换 ID 或 seed。

在 Windows“已安装的应用”中卸载，或运行程序目录的 `uninstall.exe`。**默认保留全部用户数据**，重装后继续使用同一工作区。只有主动勾选“删除此 Windows 用户的全部棱镜数据”并再次确认才永久删除；这不删除 WebView2 或其他软件数据。数据/程序目录含链接或重解析点时拒绝操作，不沿链接删除外部文件。卸载不递归清空未知程序目录文件。

卸载遇文件占用或权限错误会准确失败：保留卸载器和注册供解除占用后重试，不先清除重试入口。数据删除可能已完成部分文件，不报告全部成功，也不会默默改成保留策略。

自动化安装：安装包 `/S`；自动化卸载：`uninstall.exe /S` 默认仍保留数据。永久删除必须显式 `/S /REMOVE-DATA=CONFIRMED`，请勿对真实工作区误用。

实际构建与安装验收状态以 [T03 记录](verification/T03.md) 为准；本预览不是 T21 的完整产品验收包。许可文件跟随每个版本：本项目 LICENSE、THIRD_PARTY_NOTICES、GO-THIRD-PARTY-NOTICES 与 NSIS-LICENSE。fingerprint-chromium 的来源和许可由其对应版本单独记录，当前没有内核分发。

后续T11隔离组件允许安装时请求管理员授权、日常界面尽量普通用户运行；这是已确认的未来安装合同，组件尚未实现/打包或安装，不改变上述旧预览的免管理员范围。当前源码真实代理启动仍被安全门禁阻止，不能把安装旧预览或管理员授权当作全路径保护就绪。[D012](DECISIONS.md#d012--安全边界缺失先阻止代理启动并允许隔离组件管理员安装2026-10-01)。

T12 Cookie控制仅新增源码，使用指定会话匿名pipe、不需要额外安装服务或调试端口；尚未构建新的桌面/安装器，不包含在旧T03预览中。它不解除T11代理启动门禁，也不能用旧安装包验收Cookie读回或新增UI。[当前记录](verification/T12.md)。

## 可重复构建与验证

固定 Node 24、`go.mod` 中 Go 版本、Wails v2.16.0；项目局部工具和锁文件按 [首页](../README.md) 设置。执行：

```powershell
npm ci
npm run test:desktop
npm run build:installer
npm run build:installer -- -PreviewRevision 2
npm run verify:installer
```

`setup:nsis` 自动取得官方 NSIS 3.13 便携 ZIP，先核验固定 SHA-256，再运行编译器。`build:installer` 同时构建 Windows production 程序、窄维护 helper、许可、安装器与真实签名/哈希清单；生成物在忽略的 `build/releases/`。Wails 反射绑定生成不会开启默认数据库。

**验证请用没有棱镜数据的新 Windows 用户或 VM。** 验证脚本检测到已有默认数据或安装立即拒绝，不把改环境变量当用户隔离，也不会替你删除现有档案。它使用两个明确不同预览修订，实际安装后 UIA 创建编辑和两次正常重开，核对 SQLite；升级后、保留卸载后、重装后核对同一 ID/seed/偏好；最后仅对脚本自己的合成工作区明确删除。记录在 `output/goal/T03/`。

GitHub 全新 Windows runner 上执行同一闭环（`--silent-wizard` 仅略过安装向导点击，实际安装和应用 UI 操作不省略），依赖缺失时在临时测试主机安装微软签名运行时。该测试主机准备脚本不进入产品安装器。无屏幕的 runner 可禁用截图，但不能禁用 UI/数据库断言；本机 Windows 11 另验证用户可见的数据保留选择页。
