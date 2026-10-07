# 棱镜浏览器 · Windows 首版候选安装说明

适用于 **0.3.0-preview.4 · v1-candidate**。保留预览版本格式以兼容既有同用户升级；工作台标题和发布清单明确标为“首版候选”，**不是正式发布**。包含本机环境、正式代理保护、FIFO、Cookie、完整备份恢复、回收、迁移与诊断实现。独立远端出口、人工完整流程和干净Windows验收仍待完成，只用合成数据检查。实际包、哈希与结果见[首版报告](verification/V1-final.md)。

旧preview.1/.2只交付本机配置，不能用它们验证新增功能。WebView2只承载工作台；fingerprint-chromium由用户另行安装，候选包不捆绑内核。

## 安装与启动

1. 使用 Windows 10/11 x64，安装微软 **WebView2 Evergreen Runtime**（94.0.992.31 或更新版本）：<https://developer.microsoft.com/microsoft-edge/webview2/>。
2. 核对旁边`prism-browser-0.3.0-preview.4-SHA256SUMS.txt`及`prism-browser-0.3.0-preview.4-release.json`，再运行`prism-browser-0.3.0-preview.4-windows-amd64-setup.exe`。PowerShell用`Get-FileHash <安装包> -Algorithm SHA256`计算，必须逐字一致；确认清单channel是`v1-candidate`且源码`sourceDirty=false`。
3. 当前构建**未签名**，Windows 可能显示 SmartScreen/未知发布者提示。不要关闭系统安全功能；仅在确认来源和哈希后自行决定是否运行。
4. 安装仅作用于当前Windows用户，无需管理员。为保持升级入口，桌面/开始菜单快捷方式仍叫“棱镜浏览器 · 开发预览”；打开后的窗口必须显示“首版候选 0.3.0-preview.4”。候选不应覆盖日常环境，请优先使用新Windows测试用户或VM。

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

实际状态以[首版报告](verification/V1-final.md)为准；旧[T03验收](verification/T03.md)不替代本版。每版程序目录包含LICENSE、THIRD_PARTY_NOTICES、GO/FRONTEND-THIRD-PARTY-NOTICES、NSIS-LICENSE、INSTALLATION和USER_GUIDE。内核许可按精确版本另记，不再分发Chromium。

正式保护使用Windows现有AppContainer/Job/ACL，不安装额外服务、不停服、不关闭Chromium沙箱。逐会话资源缺失、失败或未知时拒绝代理启动，管理员身份/代理独立检查均不是启动许可。首版要求Windows隔离服务正常；服务自身损坏保护为后续加固，不作已有保证。

Cookie使用指定会话私有pipe，不开公网调试端口。空白启动不会恢复原标签或自动写Cookie；绑定代理仍走正式保护，未绑定才允许明确确认直连。真实写后读回及双环境隔离有本机证据，分区等其他组合保持待验。

## 可重复构建与验证

固定 Node 24、`go.mod` 中 Go 版本、Wails v2.16.0；项目局部工具和锁文件按 [首页](../README.md) 设置。执行：

```powershell
npm ci
npm run check:background
npm run test:desktop
npm run build:installer -- -PreviewRevision 3 -Candidate
npm run build:installer -- -PreviewRevision 4 -Candidate
npm run verify:installer -- --no-clicks --candidate --first-revision=3 --second-revision=4
```

`setup:nsis` 自动取得官方 NSIS 3.13 便携 ZIP，先核验固定 SHA-256，再运行编译器。`build:installer` 同时构建 Windows production 程序、窄维护 helper、许可、安装器与真实签名/哈希清单；生成物在忽略的 `build/releases/`。Wails 反射绑定生成不会开启默认数据库。

**验证优先使用新Windows用户或VM。** `--no-clicks`禁止安装/应用自动点击：静默NSIS、实际快捷方式启动、只读UIA确认native页面与合成记录、正常关窗；带nonce且先只读确认空库的原生API夹具创建编辑，之后核对升级/卸载保留/重装身份。仅明确删除自己合成工作区；已有默认数据、安装、注册或任一同名快捷方式时拒绝。本机新产品目录并不等于干净机器。记录在`output/goal/V1-final/install-no-clicks/`。

不带`--no-clicks`的旧验证路径仍使用自动点击，当前用户约束下**不得运行**；`--silent-wizard`也不等于无点击。人工补验安装/卸载选择页及产品流程见报告；GitHub登录恢复后再安排干净Windows runner。测试机运行时准备脚本不进入产品。
