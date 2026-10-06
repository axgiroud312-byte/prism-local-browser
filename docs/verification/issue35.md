[执行范围](../GOAL.md) · [冻结参考](../UI_REFERENCE.md) · [公共责任](../UI_CONTRACT.md) · [合成截图索引](screenshots/issue35/README.md)

# #35 代理、内核与迁移模块交付记录

日期：2026-10-06。关联 [#35](https://github.com/axgiroud312-byte/prism-local-browser/issues/35)，分支 `codex/issue35-proxy-kernel`。

**结果：页面/窗口模块、既有服务接入和限定检查已交付；正式 App 接入及 #37 跨页终验仍待 MAIN 完成。不是全部商业页面逐像素通过，也不是新增真实桌面验收。**

后续正式App接入已完成：旧demo代理/内核页及重复导入/内核窗口已移除，两模式环境表单复用唯一importer；准确环境范围、迁移单入口、任务及旧内核回归见 [#37整组记录](issue37.md)。下文为本票原组件交付事实；100张harness图不重标成最新App。

- 初始实现/定向用例：`1b94597`。
- 失败提示、密度与受理回执修复：`1b62a51`。
- MAIN 合入：`9c56a1f` 包含 `590a993`；交付前 `c096f9b` 再合入 `3b59df7`。最终检查与截图的源码为 `c096f9b4d01685871de4fc3ece04721d4ce8ea26`；后续证据提交不改变受测源代码。
- 未独立修改 App、全局/公共样式、适配层、应用契约、后端、旧测试或 shared 文档。`NativeRestoreExecution` 保持 #36 所有权及原 props。

## 1 已交付行为与需求映射

| 需求 | 交付模块 | 可检查行为与边界 |
| --- | --- | --- |
| PRX-001 | [ProxyManagementPage](../../src/components/ProxyManagementPage.tsx)、[NativeProxyManager](../../src/components/NativeProxyManager.tsx)、[DemoProxyManager](../../src/components/DemoProxyManager.tsx) | 重做 tab/工具条、筛选、10条分页、行操作、检测详情与使用情况；不是旧页套标题。native 检查呈现服务阶段，演示明确标模拟；历史成功不表示永久保护。 |
| PRX-001 / UX-001 | [ProxyImportWindow](../../src/components/ProxyImportWindow.tsx)、[proxy-import-session](../../src/components/proxy-import-session.ts) | 唯一单个/文本/UTF-8文件导入及环境返回窗口。native 用原 parse/commit/discard；demo 用原解析器与注入服务的 compatibility 保存。不新建 adapter。 |
| PRX-001 | [ProxyEditWindow](../../src/components/ProxyEditWindow.tsx)、[ProxyUsageWindow](../../src/components/ProxyUsageWindow.tsx)、[native-proxy-actions](../../src/components/native-proxy-actions.ts) | keep/replace/clear；旧认证不回填；精确节点/修订/未知请求核实。引用删除受保护；使用列表不把已读环境页当全量。批量分配通过 MAIN 回调传准确所选环境 ID。 |
| CORE-001 / FP-002 | [KernelManagementPage](../../src/components/KernelManagementPage.tsx)、[NativeKernelManager](../../src/components/NativeKernelManager.tsx)、[DemoKernelManager](../../src/components/DemoKernelManager.tsx)、[KernelDetailsWindow](../../src/components/KernelDetailsWindow.tsx) | 服务精确构建、默认/可用/不可用/准备入口、校验、受保护删除、任务和能力详情；安装版本与摘要初始为空。演示元数据不冒充安装；默认变化不重绑定旧环境。 |
| CORE-001 / UX-001 | [kernel-task-owner](../../src/components/kernel-task-owner.ts)、[NativeMigrationManager](../../src/components/NativeMigrationManager.tsx) | 内核受理/未知/取消/失败重试/待保存/历史任务与迟到响应保护；迁移选择、差异、确认、进度、取消、记录及升级前恢复入口保留。真实准备/试用/切换/恢复未在本票执行。 |
| UX-001 | [ProxyKernelModal](../../src/components/ProxyKernelModal.tsx)、[本票样式](../../src/components/proxy-kernel35.css) | 复用公共 ReferenceModalFrame；self 模式提供 portal/inert/Tab/Escape/滚动锁/焦点返回，parent 模式只呈现 frame。上层工作区阻断窗口优先，不 inert React root。 |

### 唯一导入会话

- `getProxyImportSession(application)` 以既有 ApplicationService 为 WeakMap key，内存持有输入、文件名、安全预览、所选行、错误与原请求；关闭、hide、卸载或 SPA 换页不是清除。不会把 native 输入写入 localStorage 或日志；页面重新加载不保证保留。
- 原始输入默认遮罩；显式显示只用于核对当前输入，关闭后恢复遮罩。预览仅显示认证是否设置，不显示用户名/密码。
- Clear 是明确动作，忙/未知时不能清除。坏 UTF-8/超大文件不覆盖先前输入；2MiB 是单文件读入边界，不是代理数量配额。
- 未知提交冻结原 `previewId/requestId/selectedRows`；再次核实只重放同一请求。明确失败保留预览并允许修正；改输入或选行才产生新请求。
- 成功只删除原始行号中已经提交的行，消费预览与请求；未选/坏行继续保留，不能重复导入已提交行。关闭环境编辑不会回滚已保存代理。
- `onImported(ids)` 只交付给仍挂载且打开的窗口，不把迟到成功绑定到已关闭的环境草稿。MAIN 继续持有名称/分组/内核/seed/其他草稿。

### 迁移与长任务保护

- 迁移查找页没有网络策略，必须对准确环境调用 `Environment.Preview(kind: edit)`，读取并丢弃该预览，再调用迁移预览；环境 ID/expectedRevision 必须一致。跨页环境不从当前 workspace 猜 direct；读取失败或修订变化不能确认试用。
- 取消终态移除仍在进行的控制，并重新读取工作区；固定身份、构建、代理绑定与数据引用保持原值。升级前恢复继续复用 #36 的 NativeRestoreExecution。
- `operationIsTerminal` / `mergeOperation` 保留待保存和终态保护；关闭不取消。旧任务回执不能覆盖后续任务。查看另一历史内核任务会清除前一任务的 retry request，不会重试错构建。
- 复审新增回归：任务已明确受理后，刷新 rejection 不得抹掉回执或误标未知提交；显示“已受理；工作区刷新未确认”，保留原 operation，不重复安装。

## 2 实际限定检查

只用已有依赖、Playwright 和隔离 Vite `http://127.0.0.1:5195`。`$proofRoot` 代表仓库外本轮支持目录；三个配置为 1 worker、5195、各自 outputDir，不改仓库全局测试配置。

```powershell
node node_modules/@playwright/test/cli.js test --config "$proofRoot/issue35-playwright.config.ts" proxy-kernel.spec.ts native-boundary.spec.ts --grep "native proxy|unknown proxy|kernel prepare|demo exported|file import|unknown native edit|selecting an older|workspace fault|broken desktop bridge"
node node_modules/@playwright/test/cli.js test --config "$proofRoot/issue35-support.config.ts"
node node_modules/@playwright/test/cli.js test --config "$proofRoot/issue35-kernel.config.ts"
node node_modules/typescript/bin/tsc --noEmit -p tsconfig.json
node node_modules/typescript/bin/tsc --noEmit -p tsconfig.tests.json
```

| 最终结果（合入 c096f9b 后） | 范围 |
| --- | --- |
| **12/12，14.6秒** | 本票11项 [proxy-kernel.spec.ts](../../tests/ui/proxy-kernel.spec.ts) + 未改的 broken desktop bridge 用例。native manager 部分通过现有 App，独立 demo/嵌套往返通过本票 export harness；不是最终 App 全链验收。 |
| **4/4，4.1秒** | 外部迁移支持用例：准确当前/跨页策略、读失败不能猜 direct、取消保原 ID/seed/build/binding/data reference。仅预览和预置合成任务取消；没有 Migration.Prepare/Action 或安装。 |
| **2/2，4.9秒** | 外部旧内核用例副本保留原 bridges 与终态/迟到取消/精确构建/引用保护/未验证字段/窄屏断言；只适配 Prepare→填原合成版本/摘要→Install、关闭任务窗、打开能力详情。仓库旧测试未改，MAIN 仍需同步定位。 |
| 两份 `tsc --noEmit` 通过 | 源码与测试类型检查；未调用 tsc -b、Vite build 或打包命令。 |

覆盖：导入 hide/route/unmount 保留、Clear 与取消不同、未知原请求、成功不重放；文件失败保持原输入、Tab/Escape/inert/焦点；认证处理与 exactRevision、代理阶段/持久终态/绑定不变；内核空版本/可信 ZIP/待保存/重试归属/迟到取消；高优先级工作区阻断及恢复。

中途失败如实保留：工作区焦点检查在 MAIN autofocus 修复合入前失败；后续合成恢复 race 改为在准确恢复按钮 click 时释放测试读故障，不改生产保护。复审刷新 rejection 回归先 **1/1失败**（受理后任务窗未打开），修复后单项 **1/1通过，1.9秒**，再完成最终12/12。早期截图脚本曾用支持 fixture 不接受的任意输入，改为其公开合成 INPUTS 后重拍；不放宽生产契约。

`tdd` / `code-review` skill 不可用，采用可观察定向回归及手工代码/截图复审。未增加依赖；没有安装、生产构建/打包、全套 npm run check、Go/Wails/UIA、真实内核/代理/网络/目录操作、push、PR 合并或 issue 关闭。#37 负责整轮共享检查。

## 3 合成截图与同尺寸核对

最终 **50个“模式×状态”组合 × 两视口 = 100张 viewport PNG**（native68/demo32），另有9张从1280实现图裁出的局部图。每张 DPR1、100%、zh-CN、Asia/Shanghai、viewport-only；动画禁用、caret隐藏、外联请求阻断。页面错误 **0**、document 横向溢出 **0**。

- [全部状态/映射/局部图](screenshots/issue35/README.md)；[逐图几何与来源 commit](screenshots/issue35/manifest.json)；[取证参数](screenshots/issue35/capture.json)；[局部图来源矩形](screenshots/issue35/crops.json)。
- [尺寸/SHA-256及证据核对](screenshots/issue35/integrity.json)：100张原尺寸实现图、9张局部图及50组双视口配对通过；本票两份Markdown本地链接可解析，未发现私人绝对路径。核对脚本首版把HTTP URL误识别为盘符，收紧到独立盘符后通过，没有改动证据内容以绕过检查。
- [代理预览 1440](screenshots/issue35/1440x900/native-proxy-import-preview.png) / [1280](screenshots/issue35/1280x800/native-proxy-import-preview.png)。
- [失败固定 footer](screenshots/issue35/crops/1280-native-proxy-save-error-dialogFooter.png) / [未知固定 footer](screenshots/issue35/crops/1280-native-proxy-import-unknown-dialogFooter.png) / [坏文件保持原文件](screenshots/issue35/crops/1280-native-proxy-import-file-error-dialog.png)。
- [六行使用情况](screenshots/issue35/crops/1280-demo-proxy-bound-dialog.png) / [受理后刷新失败](screenshots/issue35/crops/1280-native-kernel-accepted-refresh-error-dialog.png)。

截图来自 [独立导出 harness](../../tests/ui/fixtures/proxy-kernel-harness.jsx)，复用 #33 样式但简化 shell/环境草稿，不是最终 App。普通 native 用 [本票 bridge](../../tests/ui/fixtures/proxy-kernel.ts)，保存失败/迁移预览/预置取消用仓库外合成支持 scenarios。所有地址/凭据/目录引用/版本/摘要均为合成或示例；截图“真实诊断已核验”仅是服务状态渲染，`synthetic-not-probed` 和水印说明非真实观测。

原 PNG/归档/比较拼图留在仓库外。本票已实际查看两视口原参考、25张私有比较页及关键全尺寸/局部图；最终新增受理刷新失败状态另行核对。无同名内核/迁移原画面，明确使用容器映射；不把捕获数量当视觉通过数量。

| 对象 | 最终实测（1440 / 1280） | 对照/适配结论 |
| --- | --- | --- |
| 页 tabs / 面板 | tabs29；面板 x210/y89、宽1220 / 1060 | 冻结密集管理页结构，删购买/云分类；harness shell 不是完整导航验收。 |
| 列表 | 表头 y140/h40；首行 y190/h48、pitch58；宽1200 / 1040 | 与参考主几何近齐；native列按服务安全字段重排，不复制商业列；长 ID/动作可换行。 |
| 文本空/两行预览 | 1050×552 / 1050×610；x195 / 115 | 左输入/右说明/下预览及固定footer对齐；新增遮罩、原行号、明确所选、坏行/重复，不仿原解析器静默丢行。 |
| 单个添加 | 宽620，未解析高552，解析行增加高度 | 删除商业模式/检测源/URL/UDP/共享账号等无服务字段后缩短；保留真实5字段、解析与确认，不造空行。 |
| 代理编辑 | 620×614；native替换认证高726，受视口max-height限制 | keep/replace/clear替代旧认证回填；下划线输入和固定操作；表单高差为安全/能力适配。 |
| 文件导入 | 500×360；失败footer增高，正文仍滚动 | UTF-8文本替代商业Excel/模板/1000条上限。坏文件不覆盖之前文件；失败始终在footer可见。 |
| 使用/迁移选择 | 1040×592 | demo6行及分页可见；native只有服务返回的引用，不虚构6个成员或默认全选；迁移用真实单环境/修订/策略差异。 |
| 内核准备 | 620×420（官方）/620×530（可信ZIP） | 映射620表单；移除旧大卡片与硬编码版本，精确摘要/信任说明保留。 |
| 短确认/任务 | 宽400；标题40、footer固定，内容自适应 | 原结果/危险确认容器映射；失败、待保存、取消、已受理刷新未确认分别显示，不假造服务成功。 |
| 能力 drawer | 宽660、x780 / 620、top40、bottom8 | 映射environment-edit容器；只读能力/来源/证据，不造原环境表单或指纹任意编辑。 |

已在取证中修复：旧卡片布局、过高空准备/添加窗、使用列表第6行不可见、错误行颜色被table规则覆盖、保存失败/未知反馈落在正文折叠以下。错误footer会压缩可滚正文，部分预览需要滚动；不遮住主操作，也不宣称与原静默错误toast像素相同。

## 4 裁剪、缺参考与剩余责任

1. **无服务裁剪**：购买/海内外云代理/供应商账号、商业接口、Excel模板、刷新URL、UDP和共享账号、原检测源/任意指纹编辑、云账号组织/套餐/实例配额。未删除已经实现但尚未桌面验收的本机入口。
2. **安全差异**：凭据不回显，认证明确处理；重复地址不是相同身份；坏行保留；取消/hide不抹未知请求；明确环境范围；历史成功不解锁下次启动，失败不直连；引用删除和持久终态保护继续由服务核对。
3. **无同名直接原图**：内核列表/安装/可信ZIP/实时任务/失败/重试/能力详情、迁移准备/试用/切换/恢复、保存失败/未知回执/真实取消。它们使用公开映射，不声称找到了原同名页面。
4. **MAIN 必做**：替换完整旧 demo代理/内核route分支；移除代理/内核旧外层toolbar；移除旧App代理导入/edit/kernel dialog及提交footer；两模式环境表单只挂一个ProxyImportWindow，传同一个application/session并避免双dialog/双focus trap；连准确所选环境的批量分配；按界面工作流适配旧kernel测试。
5. **仍待验**：真实环境连续表单往返（本票只检查简化草稿）、最终App层级/全部导航、MAIN整合后的两视口、完整迁移Prepare/Action/恢复业务UI、真实进程/目录/网络/重启持久化、人工桌面与独立远端。模块可交接不等于issue整体验收完成。

MAIN 的精确挂载/props及shared追踪与验收文案已放仓库外 `notes/issue35-integration.md` / `notes/issue35-result.md`，不并发编辑shared文档。不推送、不新建/合并PR、不关闭issue；旧候选exe未更新，正式历史 **4/21、T05–T21 / #6–#22 OPEN** 保持不变。
