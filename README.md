# 棱镜浏览器 · Prism Browser

面向 Windows 本机多环境管理的桌面底座、独立前端原型及产品开发文档。

**当前已进入首版集中验收与候选打包，正式验收仍为4/21。** 固定档案、独立会话、AppContainer代理保护、FIFO、Cookie、批次、完整备份/预检/恢复、回收、迁移和诊断均有本地实现。本机真实三存储隔离/重开、代理故障、双版本迁移和恢复已有证据；独立远端出口、人工新页面及干净Windows验收尚缺。候选身份、实际检查和逐票关闭条件见[验收报告](docs/verification/V1-final.md)。网页原型仍独立为demo，不进行自动点击。

**正式代理启动已经接入，但每次必须实际核对保护。** 零网络能力AppContainer、同身份桥、同通道前检、准确Job进程树及持久资源日志共同保护；缺失、失败或未知即拒绝，绝不自动直连。首版以Windows隔离服务正常为支持条件；底层服务自身损坏留后续加固。本机受控流量不代替独立远端无旁路验收，当前候选只用于合成数据检查。

[产品需求](docs/PRD.md) · [开发方案](docs/DEVELOPMENT.md) · [内核合同](docs/KERNEL.md) · [需求追踪](docs/TRACEABILITY.md) · [验收记录](docs/ACCEPTANCE.md)

开发入口：[项目开发指引](AGENTS.md) · [开发规范](docs/ENGINEERING.md) · [V1 总规格](docs/SPEC.md) · [开发 Issues 与依赖](docs/ISSUES.md)

持续实施：[Goal 执行规则](docs/GOAL.md) · [当前执行位置](docs/PROGRESS.md)。T01 应用契约与 DemoAdapter 已验收；T02 的 WailsAdapter 使用相同页面连接本地服务，各项能力分票验收，不以模拟成功替代。

![环境工作台](docs/screenshots/environments.png)

## 当前可以做什么

| 页面       | 原型交互                                                                                              |
| ---------- | ----------------------------------------------------------------------------------------------------- |
| 浏览器环境 | 搜索、分组及状态筛选、分页、多选、批量操作；右侧抽屉创建和编辑环境；从配置模板新建；导入示例 Cookie。 |
| 代理管理   | HTTP、HTTPS、SOCKS5 文本导入、逐行预览、编辑、绑定关系、模拟连接检查。                                |
| 内核管理   | 查看固定内核版本、来源说明、环境引用和接入要求；元数据不代表已安装。                                  |
| 备份与恢复 | 保存和导出原型 JSON 快照，校验文件并确认替换当前演示配置。                                            |
| 操作记录   | 查看和导出原型操作结果与错误说明。                                                                    |
| 产品与开发 | 在页面内阅读产品需求、开发方案和内核适配文档。                                                        |

环境配置在创建后保留固定种子；重新生成先改变草稿，保存后才生效。每个环境分别绑定代理和内核。原型用独立记录演示这些关系，真实进程、独立数据目录与网络隔离仍需桌面实现验证。

本软件的目标是无需云账号、鉴权服务或套餐即可使用的本机工作区，不设置产品级实例数量配额。实际数量与并发能力受电脑资源影响；当前原型的表现不能作为真实浏览器容量测试结果。

## 运行独立网页原型

需要 **Node.js 22.12 或更高版本**，以及随 Node.js 安装的 npm。在项目目录执行：

```sh
npm ci
npm run dev
```

打开终端显示的本机地址，通常为 `http://127.0.0.1:5173`。开发服务默认仅监听本机；切换页面使用 `/#/environments`、`/#/proxies`、`/#/kernels`、`/#/backups`、`/#/activity` 和 `/#/guide`。

检查与构建：

```sh
npm run check
npm run build
npm run preview
```

首次执行页面测试先运行 `npx playwright install chromium`。`check` 依次运行领域/应用契约/adapter 测试、源码及测试类型检查、TypeScript/Vite 构建、文档检查和可重复 UI 流程；也可单独 `npm run test:ui`。页面测试自动用独立 5183 端口启动/关闭 Vite，测试报告在 `output/goal/T01/report/`。命令存在不代表已通过，实际结果见 [验收记录](docs/ACCEPTANCE.md)。`preview` 仍不会启动真实内核。

当前用户已要求停止自动化点击。不启动上述点击流程；日常与自动 CI 改用 `npm run check:background`（不包含 UI 点击），保留旧测试供明确要求时使用。CI 的点击步骤只有手动触发且显式勾选 `run_ui_clicks` 才执行。实际桌面验证也不再自动抢焦点或截取屏幕。

2026-10-06 [本机缺口补验](docs/verification/V1-local-acceptance.md)覆盖真实强制停止、Cookie边界、257目录、12真实队列、双环境完整恢复和实际权限失败。目录失败崩溃已修，新`.6`候选/本机无点击安装通过；换行门禁根因仅以Go锁LF修正，远程三job及一次性Server开发预览安装也通过。旧`.4`只保历史，最新来源/哈希和[准确远程范围](docs/verification/V1-local-remote.json)单列；独立远端、人工、精确`.6`的Home新用户/VM/缺WebView2等仍待验。

## 从源码运行 Windows 桌面底座

需要 Windows x64、Go **1.27.1**、Wails CLI **v2.16.0**、Node.js 22.12+ 和已安装的 WebView2 Runtime。Go 可安装到系统或项目忽略的 `.tools/go/`；项目脚本仅修改自己的工具环境，不改系统 PATH。安装固定 CLI：

```powershell
npm ci
$env:GOBIN = Join-Path (Get-Location) '.tools/bin'
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
npm run doctor:desktop
npm run test:desktop
npm run build:windows
./build/bin/prism-browser.exe
```

也可 `npm run dev:desktop`。桌面 production 构建使用单独 Vite `desktop` 模式；桥接失败会显示阻断提示，不退回网页 demo。exe 旁生成 Go 运行时许可通知，分发时须一起保留。构建反射阶段不会开启用户数据库；程序启动先检查 WebView2 与目录边界。安装预览使用 `npm run build:installer`，生成安装器、哈希与真实签名状态于 `build/releases/`；目前 **NotSigned、开发预览、不含浏览器内核**，安装/升级/卸载的实际验收见 [安装说明](docs/INSTALLATION.md) 与 [T03 记录](docs/verification/T03.md)。

桌面默认数据库为 `%LOCALAPPDATA%/PrismBrowser/app.db`，桌面壳的 WebView 数据在同根 `workbench-webview/`，与网页原型 localStorage 分离。保存 ID、显式 seed、精确内核引用、分组及偏好；旧档案的 `kernel-pending` 不自动重绑定。持久批量、完整导出、预检和正式恢复已接入，真实三存储恢复有局部证据；内核诊断不等于全部能力验收。native `.prismbackup` 不携带内核本体，不承诺跨用户/重装/跨机器恢复登录。不要把原型 JSON 导入生产数据库。

桌面内核页选择精确四段发行版本和预期 ZIP SHA-256；官方来源会核对所选 tag 的 Windows x64 ZIP 与官方摘要，本地来源需通过系统文件选择器并明确确认可信。验证通过后登记新的不可替换 ID，被引用构建不能直接移除。148.0.7778.215 是实测候选，不是生产推荐；资产缺失不会自动换版本。既有点击验证入口 `npm run verify:kernel` 保留，但当前不再执行；无点击真实探测入口见 [T04 记录](docs/verification/T04.md)。

真实桌面复核使用 **Node.js 24**，运行 `npm run verify:desktop`：脚本创建专用合成测试根，以 Windows UI Automation 操作实际 exe，正常关窗、重开，再读取实际 SQLite；只清理自己启动的测试进程，不开启调试端口。它会将测试窗口置前，请不要在验证时操作该窗口。截图/脱敏记录在 `output/goal/T02/`，测试数据库在 `.appdata/verification/`，均不提交。不能用该测试推导真实 Chromium 已启动。

## 建议的体验顺序

1. 打开代理管理，填入示例，解析预览后保存，体验模拟检查。
2. 返回环境页，新建环境并选择代理、固定内核及地区设置。
3. 在指纹页签查看固定种子和能力说明，保存环境。
4. 用示例 Cookie 体验校验和导入，再启动、关闭环境，查看活动记录。
5. 创建并导出快照；停止模拟运行环境后，体验导入与恢复。

![指纹配置抽屉](docs/screenshots/fingerprint.png)

![代理管理](docs/screenshots/proxies.png)

## 数据与当前边界

原型将演示配置保存到当前浏览器的 `localStorage`。更换浏览器、端口或地址会进入不同的存储空间；清除站点数据会移除这里的记录。请导出快照留存需要保留的演示配置。

- 填入的代理凭据和 Cookie 是原型数据，可能以明文保存在当前浏览器。请勿输入真实账号 Cookie、真实代理密码或其他敏感资料。
- 原型快照包含环境配置、示例 Cookie、代理及内核元数据；导出排除代理密码。它不包含 Chromium 用户目录，也不能作为真实店铺登录状态的完整备份。
- 恢复会替换环境、代理和内核配置，保留原指纹种子；代理密码清空、检查状态重置。恢复前需停止所有模拟运行环境。
- T01–T04已正式验收，其后本机/服务验证不冒充完整交付。DPAPI不可直接跨Windows用户/机器恢复，HTTP/SOCKS5到代理不加密认证，HTTPS才TLS到代理；HTTP只接Basic。术语见 [GLOSSARY.md](GLOSSARY.md)。通道仅接受内部前检/准确Job与package身份；SOCKS5远端目标解析不含代理host自身DNS，更不能证明全路径保护。Cookie和完整恢复已有真实局部结果，新UI与独立出口仍待验。
- 指纹读值、浏览器功能兼容性和隔离效果需要后续真实运行验收。本项目不以隐蔽性评分或不封号承诺作为完成条件。

## 如何继续开发

内核已指定为 [adryfish/fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium)。本仓库没有随附内核二进制文件。正式接入须固定实际版本、架构、来源和校验值，并按 [内核适配合同](docs/KERNEL.md) 验证支持参数及实际读值。

| 文档                                 | 用途                                                   |
| ------------------------------------ | ------------------------------------------------------ |
| [PRD](docs/PRD.md)                   | 已确认范围、完整桌面目标、页面行为与验收条件。         |
| [DEVELOPMENT](docs/DEVELOPMENT.md)   | 模块划分、数据模型、本地接口、进程生命周期和恢复方案。 |
| [KERNEL](docs/KERNEL.md)             | 固定内核来源、参数映射、能力分层和接入门槛。           |
| [TRACEABILITY](docs/TRACEABILITY.md) | 12 项需求与路由、源码入口、建议验收的对应关系。        |
| [ACCEPTANCE](docs/ACCEPTANCE.md)     | 本次真正执行过的检查、截图证据和待完成事项。           |

核心前端位于 [`src/App.tsx`](src/App.tsx)，演示配置/解析/快照位于 [`src/domain.ts`](src/domain.ts)，本机服务位于 [`internal/workspace/`](internal/workspace/)，原生包与文件边界位于 [`internal/backup/`](internal/backup/)。先按用户授权逐票完成源码，再补暂停的单环境、批次/备份与真实恢复验收；详细阶段门槛见开发方案。

## 许可与来源

本仓库原创代码与文档使用 [MIT License](LICENSE)。第三方依赖保留自己的版权和许可证，详情见 [第三方声明](THIRD_PARTY_NOTICES.md)。MIT 声明不覆盖外部 Chromium 内核及其全部组件。

页面从交互需求重新实现，未复制商业浏览器的发布代码、品牌图片、图标资源、云接口、账号数据或旧资料归档，也未引入 Ant-Browser 源代码。公开仓库仅包含本项目代码、文档、示例与相应验证材料。

[GitHub 仓库](https://github.com/axgiroud312-byte/prism-local-browser)
