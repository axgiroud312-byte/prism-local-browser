# 桌面可用性收口（2026-10-07）

用户取消1:1要求；商业参考缺失不再阻塞桌面交付。旧缺参考、FAIL、不支持及各检查的来源全部保留，不改写成1:1验收通过。本轮由 `codex/issue32-37-ui-integration` 的 `8688906` 继续，已核实 PR #31 精确 `e170099bc25aa15ecb9a8456d782394a2139ec20` 为祖先，PR #38 仍依赖 #31，不自动合并。

## 本轮交付结论

**已交付新程序与必要修复；整体桌面可用性收口尚未完成。** 真实桌面中，150 内核在迁移试用和普通打开均发生 `chrome.dll / 80000003` 崩溃。原档案与数据已保住，但未查明上游断言/栈，不能将失败诊断修复称为内核修复。148 环境和本轮指定的 Clash 代理环境有真实成功读回。没有用 Vite、模拟 bridge 或旧 exe 补齐实际桌面缺口。

日期 2026-10-07，Windows 本机。全部操作在自有隔离 root 内，实际 Wails root 带 `synthetic-only` marker；没有使用真实用户 profile、Cookie、凭据或业务数据，没有停止 Clash 或其他程序。正常退出时自有浏览器为 0、恢复预检暂存与 staging 为 0；3 个网络会话全部 closed，18 项资源全部 released。正式 native-workspace 的合成数据与迁移失败副本留在 ignored 本机目录；两个可重建内核发布诊断 payload 已按清单清理，范围见[独立记录](desktop-kernel-publication-environment.md)。

公开报告仅列合成标识与脱敏结果。必要证据集中在 ignored `output/goal/desktop-closeout/`；同一份记录中，受控故障、服务自动化、模拟 bridge 与新 Wails 桌面操作分别注明。没有恢复商业参考或继续图库审计。

## 直接运行程序

| 属性 | 实际值 |
| --- | --- |
| 入口 | 本机 `build/bin/prism-browser.exe` |
| 应用版本 / 类型 | `0.3.0-preview.8` / Windows amd64 production Wails 开发预览 |
| 构建源码 | `e349eeadaa92a83ce60d97ec2ef5b309fdf68d92`；包含前一修复 `be5b1768c96ffc6f448a695f6e430dc8c828402b` |
| SHA-256 / 大小 | `d10c562bd732f0e3546440ebc0b331ec175bf0851f46c3887709d3a9125c8f4c` / 21,003,776 bytes |
| 实际构建 | `npm run build:windows -- -PreviewRevision 8`；Wails 生产构建 35.467 秒通过，Go / 前端许可通知已生成 |
| 实际窗口 | 两个本轮测试实例均显示 `0.3.0-preview.8`，native 模式、真实工作区，没有注入 bridge |
| 已保留上一程序 | `0.3.0-preview.7`、源码 `be5b176`、SHA `6acdcc1f26d47e15f216849496c1dd6613035e0bccbca107b031aed869558064`；其实际结果单独注明 |
| 分发边界 | 本轮直接 exe，没有生成新安装器，不套用旧 preview.6 release manifest；元数据和伴随通知见[程序来源报告](desktop-release-closeout.md) |

后续验收文档提交不会重新绑定二进制源码；`e349eea` 是本程序的实际构建来源。程序没有捆绑 fingerprint-chromium，两个精确已核验版本由真实 `Kernel.SelectArchive/Install` 安装到合成工作区后使用。

## 五类缺口

| 项目 | 最终判定 | 本轮实际结果与边界 |
| --- | --- | --- |
| 批量未执行分支 | 仍阻塞（实际桌面取消未截获） | preview7 全部成功、preview8 同名冲突的 2 成功/1 失败、继续只处理失败项实际通过。服务取消/资源暂停的精确未执行后缀已按真实合同自动化通过；preview8 64 项在取消按钮被观察前已完成，不能写成取消通过。[批次来源与范围](desktop-batch-closeout.md)。 |
| 指纹生成与迟到回复 | 仍阻塞（真实持续等待/迟到回复未捕获） | 新桌面普通编辑、关闭重开、运行中关键字段禁用和已保存身份保持实际通过；旧成功/失败/异常/finally 的请求归属定向自动化 6 项通过。native 创建时观察到未完成预览及创建保护、随后生成完成；未截获持续等待下关闭重开和真实迟到 IPC，不能以注入延迟冒充。[指纹记录](desktop-fingerprint-closeout.md)。 |
| 回收历史查询 | 实际通过 | 按正确 `Recycle.ReadPage` 查询冻结历史及逐项结果；preview7 移除/找回 B 后原 ID、seed、148 内核和数据引用保留，实际三存储读回。没有用 `Operation.Read` 顶替历史页；旧错表查询/旧未证保留。[回收报告](desktop-recycle-migration-closeout.md)。 |
| 迁移丢失来源标识 | 仍阻塞（同会话恢复能力不可用） | 保持安全阻断；缺 token 不猜来源、不制造 token、不另建来源。原 worker/scratch 的等待及清理保护已修复并定向通过；preview8 对已知来源正常预检/明确丢弃/实际清理通过。它不能充当真实丢 token 恢复；现合同没有可靠的原来源查询接口。 |
| 多项创建部分完成 | 仍阻塞（桌面取消部分未证） | preview8 真实 Batch 三项中 2 已创建、1 `NAME_CONFLICT`；改占位环境名字后，原计划重试只补第 2 项，旧两项 ID/seed/ref 和第 2 项预配身份完全保持。真实部分成功/失败/重试已通过，取消桌面证据缺口同上。单条 Create 的多项合同不适用，不能将 Create→Batch 移交算单条多项部分完成。 |

## 不适用且有依据（均不计通过）

| 旧条款/分支 | 契约依据 |
| --- | --- |
| native Batch 返回 `skipped` 项 | 实际公开状态只有 `completed/failed/not-executed`；内部 prepared 投影为未执行，重试略过已完成索引但不产生 skipped 结果。 |
| native Runtime 的一个批量 skipped 响应 | Runtime 每次处理单个准确 environmentId；批量打开是逐 ID FIFO 任务，取消/失败有各自任务结果。 |
| 单条 `Environment.Create(count>1)` 部分提交 | count!=1 返回 `CAPABILITY_UNSUPPORTED`、没有提交；真正多项使用 Batch。 |
| Batch 创建后打开失败的内置重试 | 这个批次入口创建而不启动浏览器；它没有 create-open 移交。单条“创建并打开失败只重试原 ID”的完整新 exe 链本轮未完成，不能因此算通过。 |
| demo 的异步生成等待 | 当前 demo 同步生成；保持原合同，不造异步分支。 |
| 新安装器安装/升级/卸载 | 本轮直接运行 exe，未生成新安装器；旧安装器验收不适用于新文件。 |

## 实际 Windows 桌面验收表

所有“实际通过”限定下表已操作的范围，不表示列名对应的所有组合或正式整票均完成。

| 流程 | 判定 | 操作、实际结果及来源 |
| --- | --- | --- |
| 启动、退出、再启动；持久化 | 实际通过 | preview8 两次启动与正常退出 exit0；重开保留 73 个合成环境、两份已核验内核、Clash 7897/revision2。重开 B 实际读回三类存储；没有使用默认真实工作区。 |
| 创建、编辑、固定身份 | 实际通过 | preview7 A/B 创建并真实打开；A 运行中改备注。preview8 单项占位创建、改名和真实三项 Batch。初始 5 项 ID、fingerprintId、seed、精确内核及数据引用与最后快照逐项一致；73 项 ID、seed、引用唯一。72 个真实目录，未打开单项占位环境只有合法懒创建引用，未伪造目录检查通过。 |
| 真实内核、进程归属与隔离 | 实际通过（所列成功会话） | 精确 `148.0.7778.215` 与 `150.0.7871.186` 的真实 exe/进程版本/hash、工作区内可执行路径、独立 profile 与 PID/创建时间已核对。B 的实际 CPU14、A 曾 CPU24，语言/时区读回。150 稳定打开另列阻塞。 |
| 150 普通打开与迁移可用性 | 仍阻塞 | GUI7/8 新 150 迁移试用各自绑定 Windows Error1000、`chrome.dll 150 / 80000003 / 77b71bd`；GUI8 A 普通打开及正常重试也失败。没有改 seed、降版、禁用 sandbox/GPU 或换空数据来伪称修复。 |
| 普通编辑/重开身份，运行中关键字段保护 | 实际通过 | A 运行时启动网址、窗口参数、精确内核、语言/时区、CPU/换一套受保护；备注保存后 seed/core/ref 不变。B 经过恢复、回收、找回、迁移失败及 preview8 重开仍是原身份。不是所有指纹输出/跨版本完全一致的承诺。 |
| 直连、Clash 代理入口与 HTTPS | 实际通过（当前请求） | 明确 direct 的 A/B 会话曾读自有 HTTP 观察页。preview8 P 绑定保存的 HTTP `127.0.0.1:7897`、无凭据，真实 150 浏览器打开 api.ipify HTTPS，页面实读出口与生产前检相同；TLS校验通过。Clash 未切节点或改配置。 |
| Clash 的最终选中上游节点链 | 仍阻塞 | TCP controller不可用，正式YAML解析确认 pipe 后仍 ENOENT；运行配置参数不可读，UIA 没有选择元数据。只证明指定 7897 服务与真实出口，不能推断 rule 最后使用了哪条上游。 |
| 代理故障不静默直连及重试 | 实际通过（受控真实桌面 HTTP 范围） | preview7 P 绑定自有代理；只停止该代理 listener/socket、自有 origin 仍 HTTP200。生产 runtime 报 PROXY_UNREACHABLE 并停止整个 P 树，观察页无额外直连请求；恢复同一自有端口后原环境 ID/seed/ref 重开通过。这个受控故障与原始 Clash实际接入分开，不是模拟 bridge；未覆盖全部 DNS/WebRTC/UDP 泄漏矩阵。 |
| Cookie 导入、真实读回及另一环境隔离 | 实际通过（所列操作） | preview7 B 用合成 JSON 写入，native 报 written1/verified1；真正 B 浏览器读到 B-IMPORT，A 原 Cookie/LS/IDB 保持 A-OLD。解析后清空预览输入不写；非法 JSON 反馈保留为失败夹具，不冒充有效导入。preview8 失败恢复、程序重启后 B 仍读到 B-IMPORT。native 已受理 Cookie 任务中途取消/其它内核写入故障未在新 exe 完整实测。 |
| 备份预检和真实恢复 | 实际通过（所列操作） | preview7 完整包 597 entries/18,306,105 解压 bytes；先把 A/B 三类数据改为 NEW，原包只读预检后明确覆盖 A/B2 项并正常停选定进程，实际重开回到 A-OLD、B Cookie-IMPORT/LS-IDB-OLD；三个批次环境不变。包 SHA `de55d3ed156988cc0cd60b2bed455fc194338e1a3fc158ae34403fd896594a68`。preview8 同包已知来源预检→明确丢弃，没有执行恢复；73 环境/档案行前后完全一致，暂存0。 |
| 回收、历史、找回 | 实际通过 | preview7 运行中 B 移除正确拒绝 PROFILE_BUSY；正常停止后仅回收 B、历史显示该准确项，找回后仍原 ID/seed/148/ref，真实三存储读回。最终正确 environment_trash 查询为空。 |
| 迁移原身份、独立副本及失败保护 | 部分实际通过，成功切换仍阻塞 | 正式服务的真实两版本独立 NTFS 副本/试用/切换/完整回滚通过，来源单列[真实服务迁移](desktop-native-migration-closeout.md)，不能替代 GUI。两次 GUI失败均 backupVerified、trialExited、未 committed，原 148 原数据真实重开通过。preview8 新增安全 cause 已真实保存。 |
| 新 exe 单项 create-open 失败原 ID 重试、Batch取消、真实迟到回复、丢来源恢复 | 仍阻塞 | 未完成或未捕获的实际桌面分支保留；服务/故障注入通过不转换成新 exe 通过。 |
| 正常收尾与数据核对 | 实际通过 | preview8 两次 exit0、ownedChrome0、preflight/staging0；网络 journal 3 closed/18 released；两个自有观察/代理 helper 已停止，Clash 原进程仍在。SQLite integrity=ok、foreign key errors=0；正式 native-workspace 合成数据与迁移失败副本保留，两个自有发布诊断 payload 的清理另记。 |

## 实际修复与检查

- `be5b176`：指纹生成的请求序号/目标归属，关闭及回滚使旧成功/失败/异常/finally失效；demo仍同步。正确校验 `Recycle.ReadPage` 历史页身份/计数。原恢复预检未结束或 scratch 未确认清理时阻止新来源/新预检；保留原 token，只确认原 worker 和原 scratch 后释放。关闭等待 worker 后重试原清理。Vite 排除 `.appdata` watcher，实际发布环境失败和对照保留。
- `e349eea`：迁移保留 `MIGRATION_INCOMPLETE` 与原数据保护，同时持久保存安全 `{code,reason,retryable}`。不保存原始错误、路径或脚本；未知为未知，nil/启动补偿不造历史原因。preview8 真实失败读到 `CONTROL_CHANNEL_LOST/control-read-ended`，这是诊断修复，不是 150 崩溃修复。
- 定向检查已完成：6 个指纹请求归属点击、3 个 Batch UI、Batch RPC/SQLite、真实 Windows DACL 分支、正确回收页/迁移清理/关闭 worker 的 Go 与 Node/UI、类型检查；cause 2 项/5 子场景通过。各结果、初轮失败与准确来源分别见子报告。没有循环全量测试、截图或审计。
- preview7/8 Go/Wails 生产构建实际完成。最后只读汇总 `final-readback.json` 核对实际已存证据、初始 5 身份、73 引用/seed、批次重试、预检不写入、重开 B 三存储、退出资源和新 exe hash。首次汇总错误地要求未启动单条创建也已有目录，实际 ENOENT 已保留；按单条目录懒创建合同改为72实际目录+1未启动引用，未制造目录填覆盖。
- 最终文档检查 `npm run check:docs` 实际通过：82 documents、local links、12 requirements、6 routes、4 embedded documents；`git diff --check` 通过。来源与验收边界另经只读交叉核对，没有为文档发布重跑全量测试。

## 必要证据索引与剩余限制

本机 ignored `output/goal/desktop-closeout/`：`ui-actions.jsonl` 保存7/8真实UI操作；`release-artifact-manifest.json` 保存新程序来源；`batch8-first-attempt.json` / `batch8-retry-attempt.json`保存原失败和原计划继续；`batch8-cancel-attempt.json` 保存“64已完成，取消未截获”；`recycle-history-native-ui.json` 保存正确历史；恢复前后 DB/浏览器观察、`preview8-preflight-ui.json` / `preview8-preflight-canceled-db.json` 保存预检与丢弃；`clash8-browser.png` 是一次必要的自有内核HTTPS实际读值取证，不是视觉复刻截图扩展；`migration150-crash-owned.json` / `migration8-crash-owned.json` / `preview8-a-start-failed-owned.json`分别绑定自身故障；最后 `preview8-final-*.json` / `final-readback.json` 保存实际收尾。

已知关键限制：150桌面崩溃根因未证；当前系统提交量曾为96.11%，没有故障时采样或可用 minidump，不归因内存/路径/seed。短根的两次真实 Launcher 对照成功不能冲销原GUI失败。Clash选中节点链未核实；所有网络协议泄漏、干净Windows/安装升级卸载，以及上述未捕获分支仍未完整验收。缺来源标识时停止该恢复流程；不能以退出成功提示、另造token或手动删除资源认定原请求清理完成。

启动本轮程序：正常关闭已有本程序实例，打开 `build/bin/prism-browser.exe`。默认工作区是 Windows LocalAppData 下的 `PrismBrowser`；本轮测试没有打开它。需要隔离空白试用可用本机 `launch-preview8-synthetic.ps1`，其静态/InspectOnly检查通过，实际启动分支未单独实测；不要将本轮合成 root 和失败副本作为业务数据。导入可信精确fingerprint-chromium构建，新建环境选“本机网络（直连）”或保存 HTTP `127.0.0.1:7897` 并绑定；普通编辑保持原身份。当前优先使用本轮已真实读回的148；150普通打开及升级试用仍有已知阻塞，不能作为已收口版本使用。

## 历史正式验收

正式计数仍 **4/21**。本轮实际通过行已追加，T05–T21 的完整正式条件未全部满足；没有因为目标取消、模拟、局部真实通过或新构建预设整票通过。#32–#37 保持 OPEN、PR #38保持DRAFT及原base依赖；元数据按本轮真实结果更新，不自动合并。后续只推进具体阻塞，不恢复旧1:1任务或继续图库审计。
