[项目首页](../README.md) · [产品需求](PRD.md) · [开发方案](DEVELOPMENT.md) · [内核合同](KERNEL.md) · [验收记录](ACCEPTANCE.md)

# 需求到实现的追踪表

**最新追踪（2026-10-07；0.3.0-preview.10 / `95a600f`）：** CORE-001/ENV-003/PRX-001 已以新真实桌面程序核对同一精确 148 的直连与两个代理三开、代理运行期间直连关开、Clash 环境关开且其他环境原 PID 不变；旧共享 ACL 引起的本机多开 GPU 崩溃触发条件已修复。新会话按 Ant-Browser 标准共享内核，不使用额外 AppContainer、共享 ACL 修改或每会话内核副本，约 425 MiB 副本方案未交付。PRX-001/ENV-003 的受控代理故障只停止目标、不回退直连，故障前检 `no-process-created` 及恢复后原 ID/seed/profile 重试通过。UX-001 两次正常退出 `exit0`，自有 Chrome、未闭合网络会话及未释放资源均为 0；重启 204 项配置身份不变，原代理环境真实页面数据读回及 SQLite 完整性核对通过。见[148 多开修复](verification/desktop-148-isolation-fix.md)。

正式 **4/21** 保持。150/跨版本迁移在当前只用 148 的范围内不适用，不算通过；FP-002 的真实自然连续等待/迟到回复等未证条件仍保留，不合并为原 21 项全通过。用户取消 1:1，下面保留所有旧失败、未证与检查来源。

## 历史：preview9 与更早追踪摘要

2026-10-07 后续增量：BKP-001/UX-001 的原请求来源恢复、准确 scratch 清理重试及退出失败反馈已修复并在 preview9 实际验证；ENV-001/002 的原生 128 项取消/剩余续做、ENV-003 的创建后打开失败仅原 ID 重试、PRX-001 的 148 + Clash 及受控故障关闭、FP-001 的普通编辑身份保持、BKP-001 的实际三存储恢复均有新证据。CORE-001/ENV-003 的 148 多实例重开仍有崩溃，FP-002 连续等待/迟到回复没有新真实时序证明，不能按局部成功判整项通过。见[148 修复验收](verification/desktop-148-repair.md)，正式计数仍 4/21。下文旧来源/未证结果保留。

版本：1.0 · 更新日期：2026-10-07

本轮真实证据的边界：ENV-001/002 的 native 多项批次在 preview8 发生自然名称冲突，2 成功/1 失败后仅续做原失败项，所有预分配身份与旧成功项保持；ENV-003 的 148 启停和故障后重开通过，但 150 普通打开及原 ID 重试失败。FP-001/002 的保存身份、运行中关键字段保护实际核对，连续等待/迟到回复仍仅有定向自动化证据。DATA-001 的 `Recycle.ReadPage` 原回收历史、回收后恢复及三存储读回通过；BKP-001 的 preview7 完整恢复及 preview8 已知来源预检丢弃/73 项记录不变通过，丢失来源标识的可靠恢复仍阻塞。PRX-001 在 preview8 的 Clash 本机 `7897` 真实 HTTPS 访问通过，preview7 受控本机代理故障停止/原 ID 恢复范围通过；实际 Clash 节点链和全协议无旁路尚未证实。CORE-001 的精确内核/独立目录已核对，迁移原 148 身份和数据失败保护通过，真实 GUI 迁移至 150 仍崩溃。UX-001 的新程序退出重开和资源清理已实读。各项详见下方报告，不合并为完整需求或正式 T05–T21 全项通过；正式计数仍 4/21。

当前指派转为桌面可用性收口，用户取消1:1要求。ENV-001/002/003、FP-001/002、DATA-001、BKP-001、CORE-001、PRX-001、CK-001、UX-001 的本轮新程序与实际流程另见 [桌面收口验收](verification/desktop-closeout.md)。批次精确执行范围见 [批次报告](verification/desktop-batch-closeout.md)，指纹请求归属见 [指纹报告](verification/desktop-fingerprint-closeout.md)，回收历史和迁移清理见 [回收与迁移报告](verification/desktop-recycle-migration-closeout.md)。旧视觉差异和来源保留；自动化服务、模拟 bridge、不适用条款及真实新桌面证据分开计数。

当前流程：UX-001 / DOC-001 保留 [#28浏览器点击CI](https://github.com/axgiroud312-byte/prism-local-browser/issues/28) 的 `53ac883`；默认 `tests/ui` 由Vite提供页面，不构建或打包，报告在 `output/playwright/`。#29/#30的环境与统一窗口功能基线由PR #31精确 `e170099` 保留；旧 **21/21** 为 [历史逐票证据](verification/issue28-30.md)，不是本轮结果，不增加桌面验收计数。

本表关联12项需求、原型与native入口。源码入口不代表完整验收；[本轮逐票矩阵](verification/V1-final.md)明确区分服务回归、真实桌面、本机/独立网络及候选交付。[ACCEPTANCE](ACCEPTANCE.md)为实际结果索引；正式计数仍4/21。

CK-001 / ENV-001 / FP-002 / CORE-001 / PRX-001 / UX-001：[4a2五项续修](verification/issue37-recovery-and-layout.md)保护demo文件读取窗口/目标/最新输入、继承内核替换链返回焦点、显示真正保存阻断原因，恢复≤1279px工具栏动作及合法长标题边界。保存/生成条件、原请求核实、稳定身份和准确范围不变。MAIN39新＋原390px一例40/40，两项noEmit；新18图单列原时点与独立结论，旧账本不倒改，不增加严格1:1或正式桌面通过数。

UX-001 / PRX-001 的后续批次预览焦点、旧修订报告标签与primary恢复按钮修复已合入 `9de0e2a`，实际38/38新定向及两项noEmit通过；BKP-001 / DATA-001 / CORE-001 / DOC-001的新支持窗口逐图来源与失败边界见 [补核证据](verification/issue37-continuation-evidence.md)。旧 `83eafe1` 图包保持原source/render，不自动给后来源码或274项清单增加验收通过数。

CORE-001 / PRX-001 / UX-001的密集列表分页、未知请求提示容量、待保存文案与工作区晚到portal所有权修复已合入 `75bc58d`，仅新22/22定向一次及两项noEmit通过。新图须在准确源绑定下独立复核；原始FAIL与未验真实能力不回写成成功。

PRX-001的背景行成功文案防御修正已合入 `340a22c`，已有13场景双视口26/26定向一次及两项noEmit通过；不声称正常Go陈旧回复回归、Runtime授权或全状态验收。准确来源和274项冻结补证层见[增量账本](verification/issue37-continuation-evidence.md)。

ENV-001 / ENV-003 / CK-001 / FP-002 / UX-001的[环境窗口细分补证](verification/issue37-environment-evidence.md)另绑定340：184图独立70 ADAPTED / 34 MAPPED / 18 MISSING / 62 FAIL，38动作与66图后回调保存合同独立限定一致、0问题。编号与环境表整行/分页边界CSS续修已普通合入 `03d0f68`，[源码与精确集成守卫](verification/issue37-environment-css-source-review.json)一致；34新图独立34 ADAPTED、14图后回调保存合同限定一致。创建编辑容量/浮层最小修复另合入 `8ca2e79`，MAIN两项noEmit通过，新34图独立34 ADAPTED / 0 FAIL，[34记录/16回调保存合同](verification/issue37-environment-drawer-contract-review.json)限定一致、0问题，15份实际工具身份匹配。图像、动作、源码和真实native分别记账，不改原113/60/101或增加正式通过数。

原47剩余定义另经[环境层](verification/issue37-supplementary-environment.json)35个有限缺口及[F13/F15续层](verification/issue37-supplementary-environment-repairs.json)2个有限来源说明补证，派生37/47、剩10，whole-cell PASS新增0；原274、600/32/22/2、旧35层与全部失败保留。[最终审计](verification/issue37-final-evidence.json)只证明准确来源与公开材料一致，不代替后台/native、像素或完整原指南。

PRX-001 / CORE-001 / DATA-001 / UX-001：P07与M22候选普通合入 `4eadba4`、`398314c`。[源码与检查](verification/issue37-frontier-code.json)分别绑定十责任路径及作者/MAIN结果：SOCKS5替换认证和解码导入行共用1–255 UTF-8字节，超限保留草稿、修正后只保存原所选行；迁移清理未确认时保留原来源/原预检，隐藏重开仍仅核实原对，有效discarded回执才释放。tokenless丢失来源仍阻断，代理失败不直连回退。MAIN398两项noEmit成功，不重标作者候选检查或681的唯一196全套。

[本轮20图](verification/issue37-frontier-evidence.md)独立10 ADAPTED / 10 MAPPED，另4独立动作与16同图恢复链；[保存合同索引](verification/issue37-frontier-contracts.json)限定一致、0P1/P2。新[有限续层](verification/issue37-frontier-finite.json)按原定义追加7个独立决定，实际派生44/47、剩E18/F11/R09；继承37/47层、原274、历史FAIL/MISSING均不改，whole-cell PASS新增0。F26只证明合成Create→Batch移交与保留，H19直接原指南仍缺；不认证单Create多项部分完成、native持久化或正式桌面能力。

ENV-001 / PRX-001 / CORE-001 / UX-001所涉83→340九文件组合已有[独立固定源码复核](verification/issue37-final-source-review.json)，无可证明P1/P2；不将源码推导代替逐图、键盘或真实桌面结果。75/340的28/4图独立MAPPED保持各自来源，历史FAIL不倒改。

UX-001 / DOC-001另绑定340来源[七路由与指南冷加载动作](verification/issue37-native-route340-actions.json)：双视口2份有限合成记录、0PNG、每份精确5次Workspace.Read。拒绝getter0→1→1但不给Storage，原严格0合同及6条工具失败保留；S02/H19局部行为不提升为整项或原指南内容/真实桌面验收。

当前视觉增量由 [#33](https://github.com/axgiroud312-byte/prism-local-browser/issues/33) 承接：「完整代码」冻结标识 `202609160208` 唯一基准，替换旧 shell/统计卡/工具栏，增加 `/#/groups` 派生标签页。来源/40 状态/能力映射见 [UI_REFERENCE](UI_REFERENCE.md)，公共责任见 [UI_CONTRACT](UI_CONTRACT.md)，定向点击与两视口视觉分别见 [#33 记录](verification/issue33.md)。不覆盖上方 #29/#30 历史 21/21 或增加真实桌面计数。

[#34](https://github.com/axgiroud312-byte/prism-local-browser/issues/34) 连续环境表单、660px窗口、Cookie/批次/回收模块合入 `590a993`；正式App挂载、400px未保存/强制结束确认合入 `0d67ea5`，在 `12303a1` **49/49** 定向及16张实际源码图核对，见 [正式接入](verification/issue34-integration.md)。旧72张组件预览不当作主入口。统一由 [PR #38](https://github.com/axgiroud312-byte/prism-local-browser/pull/38) 交付，#37统一收尾，不因源码合入关闭票。

PRX-001 / CORE-001 / FP-002 / UX-001：[#35模块](verification/issue35.md) 合入 `c6371eb`，唯一 [`ProxyImportWindow`](../src/components/ProxyImportWindow.tsx) / [`内存session`](../src/components/proxy-import-session.ts) 正式用于代理页和环境窗口；保留输入/所选/错误/原未知请求，成功消费已提交行。在途禁止关闭，未知允许隐藏后原请求核实；认证keep/replace/clear与精确修订不变。内核 [`任务owner`](../src/components/kernel-task-owner.ts) 保留受理/待保存/历史重试/迟到保护，受控迁移入口不重复，读取saved策略/revision、不猜direct。组件原限定12+4+2及两份noEmit通过；正式App双视口跨页26项含这些边界，最终视觉/共享检查与真实桌面仍分别记录，100张旧harness图不代替最终App。

BKP-001 / DATA-001 / DOC-001 / UX-001：[#36模块](verification/issue36.md) 合入 `3c93a2a`，demo备份/恢复、活动/帮助已正式替换旧页。native完整包/只读预检/恢复/诊断保留原请求、维护保护和结果发布规则；活动精确会话动作、四文档下载保留。#37跨页定向26/26和既有必要回归8/8通过；公共 [`modal-lifecycle`](../src/components/modal-lifecycle.ts) 统一滚动与portal背景引用计数，低层按键不抢高层，Cookie→故障→诊断保留输入及实际可编辑返回，相关10/10定向通过。

ENV-001/002/003、FP-001/002、PRX-001、CORE-001、CK-001、BKP-001、DATA-001、UX-001、DOC-001：[#37整组](verification/issue37.md) 已完成正式App接线和审查修复；准确native代理模式/ID/修订报告、预检取消失败原上下文恢复、DOM替换/禁用焦点与最高单遮罩均有RED→GREEN记录。一次共享 `npm run check` 在 `681f823` **196/196，4.7分钟**；之后Cookie hover/空预览合成夹具及通知位置小修分别定向6/6、5/5通过，不把共享结果冒称最新head全套。PRD取证等待正文后重拍及native密集备份第二页仅补取证/检查，不改App；当前 `70943b0` 的 **346张实际App图** 绑定全部源码/4份文档及合成夹具，逐项参考适配、映射和缺参考见 [视觉结论](screenshots/issue37/visual-review.json)。后台、ApplicationService/适配层、RPC/SQLite/格式/依赖相对 `e170099` 未改；正式4/21、旧exe不含本轮UI和真实桌面未验边界不变。

ENV-003 / UX-001 / BKP-001 / DOC-001 完整目标复查：普通控制通道不等于准确Job强制归属资格；备份导入头部hover需保持可见；活动详情上的force确认必须屏蔽下层portal。独立审查及修复前24图保留22 MAPPED/2 FAIL，精确原预检重读/最高确认取消4/4。#33–#35恢复OPEN，当前源码入口→语义状态→证据交叉表及迁移实际App合成链继续补齐；原346图和共享196/196不代表全部支持态。最新实际结果见 [继续补核](verification/issue37-continuation.md)。

ENV-003 / CORE-001 / BKP-001 / UX-001：继续修复已集成 `0f7a233`，准确force/低层portal/上传hover与迁移旧确认资格定向62/62及两项noEmit通过。新增 [274项支持入口交叉表](verification/issue37-supported-states.md) 单独绑定读取时源码与旧图，不等于全量通过；帮助比例修正、新源码补图和独立视觉复核另记，完整原帮助缺失与正式4/21不变。

路由是运行应用后的 hash 路由。源码链接指向文件，函数名用于定位；前端持续修改时不依赖易失效的固定行号。领域逻辑自动测试入口为 [`tests/domain.test.ts`](../tests/domain.test.ts)，页面流程仍需真实浏览器操作检查。

## 12 项需求映射

| 需求 ID                       | 原型路由与交互入口                                        | 源码定位                                                                                                                                                       | 建议验收；不表示已执行                                                                                                                                     |
| ----------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ENV-001 环境列表与批量操作    | `/#/environments`；密集表/10条分页、筛选、多选、行操作；`/#/groups` 派生标签 | [`App.tsx`](../src/App.tsx)：`pageItems`、`selected`、`launch`、`stop`、`openGroupAssignment`、`assignSelectedGroup`；[`EnvironmentFilters`](../src/components/EnvironmentFilters.tsx)、[`EnvironmentGroups`](../src/components/EnvironmentGroups.tsx) | 表体单独滚动；空结果可恢复；跨页/筛选不扩大所选 ID；分组失败只重试准确失败项；无独立组实体/产品数量配额。 |
| ENV-002 创建与编辑环境        | `/#/environments`；连续四区窗口，创建/创建并打开/保存 | [`App.tsx`](../src/App.tsx)：`openCreate`、`openEdit`、`patchDraft`、`saveEnvironment`；[`EnvironmentForm`](../src/components/EnvironmentForm.tsx)；[`契约`](../src/application/contract.ts) | 同窗选名称/分组/服务内核/网络；单个不进批次计划；取消不写入；保存失败保旧记录/草稿；创建已提交而打开失败只重试原ID；native运行字段保护，未保存提醒冻结原草稿。 |
| ENV-003 启动停止与失败保护    | `/#/environments`；打开/关闭、批量及逐项结果；`/#/activity` 查看原因 | [`App.tsx`](../src/App.tsx)：`launch`、`stop`、`applyRuntimeOperation`；[`精确启动计划`](../src/application/runtime-start-plan.ts)；[`domain.ts`](../src/domain.ts)：`launchError` | 单个无需 ID/修订技术确认；按保存 direct/proxy 与修订启动；读不到配置不作直连；失败不重复创建或覆盖其他成功；重复点击及正常关闭/取消保护保留；模式明确。 |
| FP-001 固定设备档案           | `/#/environments`；指纹分区自动摘要/换一套，技术详情/历史 | [`App.tsx`](../src/App.tsx)：`generateProfile`、`applyProfilePreview`、`previewProfileRestore`、`saveEnvironment`；[`档案服务`](../internal/workspace/fingerprints.go)；[`DemoAdapter`](../src/application/demo-adapter.ts) | 自动编译/预览重试 `regenerate:false`；换一套只改草稿，取消不提交；普通编辑/代理变化/关闭重开/刷新seed不变；精确内核/数据引用不自动替换或清空。 |
| FP-002 指纹能力分层           | `/#/environments` 摘要及技术详情；`/#/kernels` 能力说明 | [`EnvironmentForm`](../src/components/EnvironmentForm.tsx)；[`FingerprintRevisionPanel`](../src/components/FingerprintRevisionPanel.tsx)；[`NativeKernelManager`](../src/components/NativeKernelManager.tsx) | 四区连续，技术/历史可收起且不阻断；区分可配置/seed/真实环境/待核对，不伪造硬件读值；窗口非屏幕指纹；新预览不冒充真实探测。 |
| PRX-001 代理导入检测与分配    | `/#/proxies`；导入/编辑/检查；统一窗口内导入及绑定 | [`domain.ts`](../src/domain.ts)：`parseProxyText`、`launchError`；[`App.tsx`](../src/App.tsx)：`openProxyImport`、`checkProxy`；[`NativeProxyManager`](../src/components/NativeProxyManager.tsx) | 同窗导入成功/取消/失败返回保留名称/分组/内核/seed/其他草稿，焦点只作用最上层；代理秘密保护及原导入校验保留；改代理不换 seed、失败不回退直连；demo 检查标模拟。 |
| CK-001 Cookie 导入            | `/#/environments`；环境行“导入 Cookie”                    | [`App.tsx`](../src/App.tsx)：`openCookies`、Cookie 预览及提交；[`domain.ts`](../src/domain.ts)：`parseCookies`、`mergeCookies`                                 | JSON/Netscape 格式可预览；错误数据不提交；空 value、会话属性、到期字段及分区信息保留；按完整身份键合并；写入只影响选定示例环境，预览和日志隐藏值。         |
| CORE-001 固定内核版本         | `/#/kernels`；统一环境窗口服务内核选择及无内核引导 | [`App.tsx`](../src/App.tsx)：`usableKernels`、`openCreate`、`saveEnvironment`；[`WailsAdapter`](../src/application/wails-adapter.ts)；[`NativeKernelManager`](../src/components/NativeKernelManager.tsx) | 不硬编码参考版本；demo 可用演示/native 同 ID installed+verified 记录；无可用内核阻断新建/打开并保草稿；保存精确 ID、旧环境不随默认变；普通编辑不能绕迁移换内核。 |
| BKP-001 快照备份与恢复        | `/#/backups`；demo JSON / native完整包明确分流，创建/导入/预检/恢复 | [`DemoBackupPage / DemoRestoreWindow`](../src/components/DemoBackupPage.tsx)；[`NativeBackupManager`](../src/components/NativeBackupManager.tsx)、[`NativeRestoreManager`](../src/components/NativeRestoreManager.tsx)、[`NativeRestoreExecution`](../src/components/NativeRestoreExecution.tsx) | demo格式/seed/历史/密码排除不变；native准确范围/原请求/备份摘要、只读预检不提交，停止失败不强杀，回滚/未知/待保存不冒称成功；真实桌面恢复另验。 |
| DATA-001 数据隔离与删除       | `/#/environments`；单个/批量移除与回收；Cookie准确目标 | [`domain.ts`](../src/domain.ts)：`Environment.id`、`mergeCookies`；[`NativeRecycleManager`](../src/components/NativeRecycleManager.tsx)、[`NativeCookieImport`](../src/components/NativeCookieImport.tsx)、[`DemoEnvironmentRemoveWindow`](../src/components/DemoEnvironmentWindows.tsx) | A修改不影响B；明确ID/数量/数据含义，运行/未知保持保护；native找回原身份、永久删除授权不扩大。demo不操作文件，页面注入不证明真实目录隔离或回收。 |
| UX-001 可访问性与本地持久演示 | 原六个目标及派生分组页；统一窗口、浮层、逐项反馈及存储提示 | [`App.tsx`](../src/App.tsx)：键盘/焦点 effects；[`ReferenceUi`](../src/components/ReferenceUi.tsx)、[`EnvironmentRuntimeDetails`](../src/components/EnvironmentRuntimeDetails.tsx)、[`styles.css`](../src/styles.css) | portal 菜单无遮挡、Escape/焦点返回；窗口不丢下层草稿；reconcile/cleanup/force/pending/FIFO 保留；坏存储与 demo/native 边界不变。 |
| DOC-001 文档与页面可追踪      | `/#/guide`；四份文档页签/下载、帮助导航 | [`HelpPage`](../src/components/HelpPage.tsx)；[`App.tsx`](../src/App.tsx)：`docTab`、raw文档传入；[`USER_GUIDE.md`](USER_GUIDE.md)、[`PRD.md`](PRD.md)、[`DEVELOPMENT.md`](DEVELOPMENT.md)、[`KERNEL.md`](KERNEL.md) | 四文档可读可下载，文件名保留；12需求一致、链接有效；当前源码/合成native/真实桌面/旧候选分别标注；逐票实际结果可追溯。 |

## 已发布的桌面开发任务

### #33 参考 shell 与环境表（2026-10-06；本地前端交付，待集成）

- ENV-001/003、UX-001：公共几何、密集环境表/筛选/分页、派生分组、精确跨页选择和逐项分组恢复接既有服务；未知创建/原 ID 打开/代理不直连链未重写。后台/格式/应用契约未改。
- DOC-001：[40 状态索引](UI_REFERENCE.md) 区分直接参考、裁剪、本机容器映射与待核实；[公共组件责任](UI_CONTRACT.md) 给 #34–#36 插入约定，由集成负责人唯一修改共享文件。
- 新增 [demo shell 用例](../tests/ui/reference-shell.spec.ts)、[合成 native 用例](../tests/ui/native-reference-shell.spec.ts)，独立 Prism 数据不是原商业接口。**7/7** 新用例与 **7/7** 既有受影响回归通过，类型通过；视觉与实际命令见 [验收](verification/issue33.md)。未运行本轮全套/生产构建/桌面探针；#37 和未核实能力保持待验。

### #29/#30 核心流程增量（2026-10-06；本地页面验收通过，待合并）

- #30 独占页面实现，#29 六项逐条整体核对；实现 `9e5b819` 和修复 `d039147` 已集成。相关需求为 ENV-001/002/003、FP-001/002、PRX-001、CORE-001、UX-001、DOC-001；不改变后台、数据库、档案/目录/凭据合同。
- 自动稳定档案、服务内核、直连/代理、创建/创建并打开/保存、代理导入保草稿、创建提交与打开失败恢复的规则见 [PRD](PRD.md#2-核心工作流程) 和 [应用交互契约](DEVELOPMENT.md#3-路由和界面契约)。已核实 [`EnvironmentForm`](../src/components/EnvironmentForm.tsx)、`App/saveEnvironment`、`App/launch`、`App/stop` 和原适配层路径；行菜单、非法数量及存储失败回归通过。
- 可重复验收入口为 [demo 点击用例](../tests/ui/environment.spec.ts)、[合成注入 bridge 用例](../tests/ui/native-boundary.spec.ts) 及 [逐票矩阵](verification/issue28-30.md)。本地 **21/21，45.2 秒**、三张截图已检查；只沿用 #28 Vite 检查，#29/#30 共用该整轮结果，最新远程结果另记，不按票重跑。
- README/使用指南明确新源码不等于旧候选 exe；历史 4/21、T05–T21 OPEN、候选身份和独立远端/人工桌面/干净 Windows 缺口不改写。

### 首版范围与执行顺序（2026-10-05）

ENV-003/PRX-001按正常Windows隔离前提验收，底层服务损坏转后续；正式生产代理及本机三种故障/旧端口接管/资源恢复已验证，独立远端全路径仍缺。阶段5–6结果、候选身份与每票缺口见[报告](verification/V1-final.md)；2026-10-06成果推送[草稿PR #27](https://github.com/axgiroud312-byte/prism-local-browser/pull/27)、17评论同步完成。首轮desktop断言失败经用户确认仅修观测，6cf1768[三job复验通过](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37401271261)；范围见[回执](verification/V1-remote-sync.json)。T05–T21仍待完整验收，不因推送或CI解锁blocking/增加计数。

ENV-003、PRX-001、DATA-001阶段2：[`独立持久资源日志`](../internal/kernel/network_journal.go)已接正式容器、差量ACL恢复及启动入口；真实内核与应用服务两轮代理启动/停止/重开均通过，三种存储保留。外部全路径、故障矩阵及人工UI仍待验；[本轮证据与边界](verification/T11-production.md)。

### 本轮行为追踪（2026-10-05；当前事实）

- CK-001/ENV-003：[`cookieStartupAllowed/cookieWriteAllowed`](../src/application/cookie-import.ts)和[`原生Cookie窗口`](../src/components/NativeCookieImport.tsx)修正旧proxy永久禁用；逐会话后端保护不变，明确direct确认不用于proxy。正常running持有resourcesPending不误当故障，无法控制/核对/落盘/网络故障仍拒绝写；6项定向adapter/模型测试和类型通过，人工窗口未点击。
- ENV-003/DATA-001：shutdown容许无cancel的已停止观察，新增[`回归`](../internal/workspace/runtime_network_cleanup_test.go)通过；不改变准确Job/owner和未知清理占用。旧测试seam与provider缺失夹具复核，不放宽真实保护。
- ENV-001/002/DATA-001：[`production目录批次回归`](../internal/workspace/batch_test.go)使用实际Windows空目录，源合成登录文件未动，clone新seed/ref，31项分页/重复请求通过；百万项仍只虚拟预览，不冒称实际规模。
- BKP-001/CORE-001/PRX-001：[`预检回归`](../internal/workspace/restore_preview_test.go)核对同精确build不同ID候选且错hash拒绝；真实DPAPI当前用户可用、拒解注入后提示重输，密文原样/响应无秘密。不是跨SID实测。恢复/回收/迁移新增硬中断结果见报告。
- DOC-001/UX-001：候选窗口/前端/manifest三层标记，全许可/指南与9文件hash一致，无内核再分发；历史`.3/.4`同干净4b38dc8/trimpath构建成功，NSIS首次PS5传参FAIL修复后通过；首轮含编译路径包不分发。无点击安装绑定SHA/source并拒已有五位置，nonce/只读空库保护；本机6次native加载/正常关闭、升级、保留卸载/重装、仅自有合成删除通过。[历史回执](verification/V1-candidate.4-acceptance.json)，不当作人工/干净机器；当前修复后`.6`见下方增量。

2026-10-06 DATA-001/PRX-001测试观测增量：首轮CI在DACL-only查询的完整SDDL字符串比较失败。经用户确认，仅修[`原目录恢复回归`](../internal/kernel/network_store_windows_test.go)为显式owner/group/DACL查询和[`实际权限快照`](../internal/kernel/network_acl_snapshot_windows_test.go)，逐字节/原顺序比ACE和继承保护，不把系统AI完成标记当授权变化；owner/group缺失、NULL DACL及任何权限扩大/deny/身份/继承/顺序差异仍拒。4顶层/12子例及vet本机通过、只读评审无可信P1/P2；Windows CI的原实际目录回归和全包测试现已通过。生产授权/恢复未改，第一轮FAIL不改写。

### 2026-10-06 本机缺口补验与批次失败修复

- ENV-001/002/DATA-001：[`生产批次wrapper`](../internal/workspace/batch_worker.go)转换接口前规范化nil lease，避免真实目录失败的清理崩溃；有效lease/归属核验与身份保留不变。[默认回归](../internal/workspace/batch_prepare_failure_test.go)、[无seam实际ACL/257项续跑](../internal/workspace/safe_gap_windows_test.go)通过。
- ENV-003、CK-001、BKP-001：普通停止超时→ForceStop、Cookie边界、12项真实队列、两运行环境完整包/三存储恢复、备份/预检ACL、恢复ACL/SQLITE_FULL与永久删除权限原任务收尾均通过。[结果/源码SHA](verification/V1-local-acceptance.md)区分真实浏览器和合成目录，不将SQLite容量当NTFS满、本机上游当独立出口。
- 131 Node、342 Go顶层PASS/30 opt-in/helper SKIP、全包vet通过；11个新opt-in另行通过。新干净67c98db候选`.5/.6`构建/本机无点击安装通过，来源commit远程三job也通过。默认CI新增一次性Windows无点击安装，旧四个点击分支仍显式opt-in，Go junction只指向固定setup-go工具、收据脱敏；实际新CI结果另记。[回执](verification/V1-local-remote.json)。无真实数据/服务故障实验/新产品依赖，旧`.4`仅历史；正式4/21与blocking不变。
- DOC-001构建可复现性：新增runner无点击安装首轮在源码dirty门禁被拒（未安装），全新Windows检出复现go.mod仅CRLF→LF/规范化diff为空。用户批准只修构建管理，[`.gitattributes`](../.gitattributes)仅固定go.mod/go.sum为LF；新全新检出两次production构建/manifest干净，实际改动自有go.mod仍被原脚本在安装前拒绝，精确恢复后干净。[实测](verification/V1-ci-source-guard.json)。依赖规范化内容逐字不变，物理go.sum CRLF/LF的SHA差别明确，不改旧哈希；tidy/状态/hash检查保留。远程实际结果另记，原FAIL不改写。
- DOC-001/UX-001：eec3333[远程三job及无点击安装通过](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37409611901)，已下载核对[安装回执](verification/V1-ci-installation.json)与两preview安装包摘要。Server 2025已有WebView2、development-preview.1/.2、6页面/正常exit0/升级保身份/拒降级25/保留卸载/重装/自有删除成立；实际PR merge和head分别记录。四点击分支跳过，不替代精确交付`.6`在Home新用户/VM、缺WebView2和人工默认框验收。原失败仍保留，[完整远程范围](verification/V1-local-remote.json)。

### T11 隔离可行性增量（历史独立实验，不覆盖当前正式结果）

ENV-001/ENV-003/UX-001/CK-001/CORE-001：阶段4[FIFO及保护链集成](verification/T14.md)已实现并局部通过，真实Cookie双环境隔离和148→150代理迁移/备份回退已验证；完整远端及人工验收待补。

当前ENV-003/PRX-001追加[正式故障恢复验证](verification/T11-recovery.md)：上游断开、实际listener失去/旧端口接管、管理器硬退出、授权后中断及A/B独立均有本机局部结果；原地资源清理重试已补，外部全路径仍待验。下列独立实验不是这次生产结果的替代。

- ENV-003、PRX-001：[`实验工具`](../cmd/network-feasibility/README.md)在固定148/Windows build26200上记录零能力AppContainer、私有pipe、准确Job全树token，以及身份socket/受控DNS和IPv4/IPv6局部观测。
- DATA-001、ENV-003：普通/AC互换及重开三种合成持久存储读回；独立桥进程硬退出、浏览器保持活着时普通host接管端口仍0连接，临时窗口站权限撤销与资源清理有实际证据。[T11记录](verification/T11.md)明确剩余外部网络/OS保护服务故障、生产接入和完整验收，产品代理门禁保持。
- PRX-001：[管理员只读规则/服务观测](verification/T11-system-boundary-observations.json)记录本机关键服务宿主、运行规则和boot策略的实际类别；不将配置快照当作故障时有效或流量命中证明。采集脚本失败准确返回、临时权限恢复及证据目录保护已核对。
- PRX-001：[`生产身份入口接点`](../internal/proxy/bridge_ingress.go)支持host成对提供listener/前检dialer，禁止失败fallback，关闭等待前检及其拨号完成；6回归仅编写，生产局部编译通过。它不提供完整系统隔离或改变现有代理门禁。
- ENV-003、PRX-001：[`资源收尾`](../internal/kernel/runtime_lifecycle_windows.go)确认Job空及通道清理成功后才释放目录，workspace保留无PID通道占用/重试，页面按当前`resourcesPending`显示重试关闭；应用退出允许重试迟到owner并等待迁移资源退出，真实崩溃根因保持。新增7内核+8服务+1adapter回归仅编写，测试包/TS仅编译通过；真实操作未验收。
- PRX-001、ENV-003：[获准BFE实验结果](verification/T11-bfe-stop-observations.json)为提升OpenService组合权限申请被拒（5），实际STOP/START各0次，BFE保持RUNNING。健康基线普通socket/DNS对照到达、AC全0；临时Job/容器/目录已清理。没有故障窗口或恢复guard运行证据，T11及后续完整代理交付仍受阻。

### T21 诊断与指南（服务回归通过，完整交付待验）

- UX-001、DOC-001：[`NativeDiagnostics`](../src/components/NativeDiagnostics.tsx)接活动页/数据库打开失败对话框，冻结预览、原请求核实及明确结束核实；[`adapter`](../src/application/diagnostics-client.ts)跨离页保留未知状态。原native活动原文导出改为脱敏报告。
- DATA-001：[`独立白名单报告`](../internal/workspace/diagnostics_report.go)及[`host保存`](../internal/workspace/diagnostics_host.go)不读取浏览内容/凭据、不flush待写状态；工作区外新文件、原字节SHA及应用会话内回执去重。签名not-checked不冒充签名检测。
- DOC-001与12需求：[使用指南](USER_GUIDE.md)接入指南页及下载；旧[安装说明](INSTALLATION.md)标明旧包适用范围。[T21](verification/T21.md)保留源码/验收区分，T11/T14及完整安装包证据仍缺，未计入完成。

### T20 选定迁移与升级前恢复（本机实跑通过，完整验收待补）

- CORE-001：[`schema10/默认构建与引用保护`](../internal/workspace/kernel_default.go)仅影响后续草稿；当前/历史/默认/迁移备份引用全部参与删除保护，缺失构建不替代。
- FP-002、FP-001：[`预览与受理`](../internal/workspace/migration_api.go)展示精确版本/能力/参数差异，原seed稳定；[`副本诊断`](../internal/kernel/migration_probe_windows.go)读取实际值及持久合成存储，试用明确正常退出后才允许切换。
- BKP-001、DATA-001：[`完整备份与副本`](../internal/workspace/migration_backup.go)、[`目录与配置决策`](../internal/workspace/migration_commit.go)及[`启动恢复`](../internal/workspace/migration_recovery.go)选择完整侧；兼容性回滚复用正式完整恢复，原备份SHA绑定、不把旧内核指向升级数据。
- UX-001：原生迁移页保持原请求/owner；[服务回归、两真实代理build及4个Kill切点](verification/V1-final.md)已实跑，人工UI/外部仍待验，不解除任何逐会话门禁。

### T19 回收与找回（服务/目录/10切点通过，完整验收待补）

- DATA-001、ENV-001：[`schema9/回收日志`](../internal/workspace/recycle_storage.go)、[`明确ID影响/确认`](../internal/workspace/recycle_api.go)、[`逐项事务`](../internal/workspace/recycle_commit.go)和[`目录worker`](../internal/workspace/recycle_worker.go)；回收配置保原身份与引用，正常业务只取active，找回推进环境修订阻旧任务ABA。
- DATA-001、FP-001、CORE-001：原目录对象/清单随同卷移动，找回保seed、档案revision/hash、精确内核和数据引用；永久删除仅授权回收树，未知路径/文件、重解析、硬链接、占用和delete-pending保持保护，共享内核/代理及备份不删。
- UX-001：[`原生回收界面`](../src/components/NativeRecycleManager.tsx)提供分页、具体ID、数据/备份影响、明确永久删除、取消与原任务核实；[`重开`](../internal/workspace/recycle_recovery.go)先核对唯一writer，再完成其余启动加载，失败不假报已停止或解锁。
- [清单](verification/T19.md)：服务/实际Windows目录、10个Kill切点及真实三存储回收/找回已通过；人工永久删除/资源故障与安装版保持待验。

### T18 中断恢复（主切点/回滚再中断通过，完整验收待补）

- BKP-001、ENV-003：[`journal启动恢复`](../internal/workspace/restore_recovery.go)、[`配置原/新摘要`](../internal/workspace/restore_consistency.go)及[`目录收尾`](../internal/workspace/restore_worker.go)从DB标记选择完整侧；其他启动记录恢复完之前保全局维护保护，不自动读取原包或开启浏览器。
- DATA-001、ENV-003：[`现有目录检查`](../internal/kernel/profile_inspect_windows.go)仅打开既有目录/锁，结合初始化事实和准确Job，不在回滚后创建新的浏览目录；启动加载独立内存容器锁外执行，完成前不落恢复终态。
- UX-001：恢复页中断/占用重试和坏日志不重置；[`硬中断`](../internal/workspace/restore_recovery_test.go)及真实五主切点组合、本轮回滚再中断已执行通过，全部自有合成root；人工/实际权限空间仍待验。

### T17 完整恢复与执行回滚（服务/选定A真实恢复通过）

- BKP-001、DATA-001：[`受理/幂等`](../internal/workspace/restore_accept.go)、[`持久日志`](../internal/workspace/restore_storage.go)、[`执行/回滚`](../internal/workspace/restore_worker.go)、[`Windows目录对象`](../internal/backup/switch_windows.go)及[`逻辑配置事务`](../internal/workspace/restore_commit.go)；同卷旧副本、提交标记与配置同事务，不把混合状态发布成功。
- FP-001：恢复原ID/seed/参数/历史，精确构建可映射本机ID，编号/配置revision与身份分开；真实目录提升初始化事实，旧更强事实保留。包外环境与本机历史不被清空。
- UX-001：native与adapter区分受理/回滚/未知/完成；服务/目录/adapter及选定A三存储恢复已通过，[清单](verification/T17.md)保留人工/多真实环境与实际资源故障边界。

### T01 应用契约先导（2026-09-30；原型层级）

ENV-002、UX-001、DOC-001 的创建/编辑/取消/刷新流程已接入 [ApplicationService](../src/application/contract.ts) 与 [DemoAdapter](../src/application/demo-adapter.ts)，[main.tsx](../src/main.tsx) 注入服务，页面不直接写 localStorage。保存失败保留旧记录和草稿，expectedRevision 拒绝过期写入；旧 v1 记录兼容，revision 存入额外元数据。其他页保留 demo-only 兼容接口，仍没有 Go/native 服务。

可重复证据：[契约测试](../tests/application.test.ts)、[UI 流程](../tests/ui/environment.spec.ts)、[T01 验证记录](verification/T01.md)。原型启动、代理、内核和目录仍是模拟/设计，不能由本项推导桌面能力通过。完整实时状态见 [PROGRESS.md](PROGRESS.md)。

### T02 本机配置底座（本地服务 / Windows 桌面增量）

ENV-002、FP-001、UX-001 的单条创建编辑由 [main.go](../main.go)、[WailsAdapter](../src/application/wails-adapter.ts) 与 [SQLite 服务](../internal/workspace/service.go) 接入。环境/初始固定 seed/分组/偏好/必要引用同事务保存；expectedRevision 和持久 requestId 拒绝过期覆盖/重复创建；真实写失败不发布成功。只接受原生预览，缺失内核明确未就绪；网页原型仍独立运行。

证据入口：[Go 契约测试](../internal/workspace/service_test.go)、[adapter 测试](../tests/wails-adapter.test.ts)、[失败桥接页面边界](../tests/ui/native-boundary.spec.ts)、[真实 Windows UI Automation](../scripts/verify-desktop-ui.ps1) 与 [逐票验收](verification/T02.md)。T02 已验收合入，不宣称批量任务恢复、真实浏览器目录/登录数据、完整档案生成/历史、代理认证或备份已经完成。

### T03 用户级安装预览（本地服务 / Windows 安装增量）

UX-001、DOC-001 的发布入口在 [安装器](../build/windows/installer/prism.nsi)、[Windows 边界](../internal/desktopbase/lifecycle_windows.go)、[程序发布](../internal/desktopbase/install_windows.go)、[入口注册与回滚](../internal/desktopbase/integration_windows.go) 与 [构建脚本](../scripts/build-installer.ps1)。版本化程序目录与固定用户数据根分开；默认卸载保留数据，明确选择才删除；WebView2 缺失/过旧阻断并说明，不静默下载。启动/维护互斥、重解析点保护、分发文件哈希、同版本修复、入口失败回滚均有模块测试。

快捷方式在真实程序发布后由 [Windows COM helper](../internal/desktopbase/shortcut_windows.go) 创建，重新加载并核对目标/永久工作目录后才发布入口，不接受空目标的链接为安装成功；全新路径、Unicode、升级和占用回滚有实际Windows读回测试。

实际验收入口：[安装说明](INSTALLATION.md)、[安装闭环脚本](../scripts/verify-installer.ps1)、[目录与失败测试](../internal/desktopbase/install_windows_test.go)、[逐票记录](verification/T03.md) 与 [最终双环境证据](verification/T03-release-acceptance.json)。T03 本机Windows11与干净Windows runner实际安装闭环、最终远程检查均通过；不将安装壳通过推导为真实浏览器/代理/完整恢复通过。不同构建的hash和源清单分别记录。

### T04 精确内核与能力记录（本地服务 / Windows 诊断增量，已验收）

- CORE-001：[`kernel`模块](../internal/kernel/install_windows.go)取得用户明确选择的官方tag或可信本地ZIP，校验实际归档/程序/完整清单摘要、amd64及PE/CDP真实版本；暂存和新ID发布，不允许原地覆盖。SQLite schema2记录证据、操作/失败和引用，旧pending档案不自动换内核，已引用构建受保护。原生[`内核页`](../src/components/NativeKernelManager.tsx)只读真实服务状态，区分受理与完成、可查询/取消任务。
- FP-002：[`受控探测`](../internal/kernel/probe_windows.go)保留沙箱，CDP仅通过限定继承句柄的私有匿名pipe。148实际HTTP/网页UA与UA-CH版本、测试种子、CPU、语言和时区回读通过；报告区分observed、source-derived和not-probed。菜单语言、字体/Canvas/音频、屏幕/定位、网络泄漏等未验收项不宣称可编辑或生效。
- 相关服务/恶意归档/真实junction/文件锁/迁移及前端测试已通过；52JS/13UI票末回归、全部Go/vet及production构建通过。官方/可信本地148真实安装、150缺失/坏hash失败、相同production exe的精确绑定/重开/真实复验/引用保护/闲置移除已记录到[真实证据](verification/T04-kernel-acceptance.json)。补充UI尝试受共享输入干扰后按用户要求stopped，不声称全通过；停止后不再自动点击，默认CI只后台检查与无头实际读值。[CI36728637966](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/36728637966)全通过，PR26合入dc0a148，#5已关闭。正常环境启停、完整指纹修订和升级回滚不是本票范围。

### T05 固定档案与同内核修订（已实现，完整验收待补）

- FP-001、ENV-002：[`固定档案服务`](../internal/workspace/fingerprints.go)与[`事务保存`](../internal/workspace/service.go)实现只读生成、服务预览/hash/原基线校验、schema3历史、同内核回滚新修订及持久幂等。名称/代理不换seed、不增加档案修订；旧pending/ID/seed/生成器版本迁移保留，已有数据引用不变。host-only忙租约供T06接入，不冒充正常运行监督器。
- FP-002：[`能力白名单编译器`](../internal/kernel/fingerprint_windows.go)只下发所选构建已核验身份/seed/网站语言/时区/CPU参数；菜单语言保持system，GPU/字体等未实测具体值不伪造，窗口不写作屏幕。共享[`预览历史面板`](../src/components/FingerprintRevisionPanel.tsx)区分可配置、seed生成、真实环境和未验证，生成不启动浏览器。
- [`服务回归`](../internal/workspace/fingerprints_test.go)、[`Demo回归`](../tests/application.test.ts)及[`Wails模式/白名单`](../tests/wails-adapter.test.ts)覆盖事务失败/重试/幂等、原预览冲突、同内核限制、特殊时区拒绝和历史重开。37项相关JS、类型与kernel/workspace Go通过；三项评审P2已修复。
- [`真实保存档案回读`](../internal/workspace/fingerprints_real_test.go)在本轮最新代码15.12秒通过，保存→重生成→回滚与服务重开保原输入/引用、实际148/CPU8/语言时区读回且正常退出。9月30日样本与10月1日空间失败保留历史，不覆盖新结果；人工抽屉仍未操作，[逐票记录](verification/T05.md)。

### T06 正常会话与独立目录（本机实跑通过，完整验收待补）

- ENV-003/CORE-001：[`运行服务`](../internal/workspace/runtime.go)按固定档案/构建/ref持久去重、FIFO启动；只接受环境/request/revision/purpose与匹配保存绑定的direct/proxy，不接受路径/参数/PID覆盖。正式proxy逐会话保护，就绪才running，准确Job全树退出/清理确认才空闲。
- DATA-001：长期会话/目录pins拒非法、链接与硬链接，Job全树退出才释放；正常profile不删，原seed/ref和三种存储停止重开已实跑核验。
- 服务/目录/adapter、A/B真实三存储隔离和重开均已有通过结果；正式proxy与显式direct证据分开，人工UI未自动执行。[剩余验收](verification/T06.md)不据源码关闭#7。

### T07 异常监督与重开核对（后台/根故障与实际ForceStop通过，人工待验）

- ENV-003、DATA-001：[`监督器`](../internal/workspace/runtime_supervisor.go)区分崩溃/断管/退出未确认，只在普通停止失败后允许指定当前Job结束；[`持久恢复`](../internal/workspace/runtime_persistence.go)与[`Windows身份核对`](../internal/kernel/runtime_recovery_windows.go)结合创建时间、session和实际锁，不接管裸PID、不按文件年龄删除锁。session/任务/活动同事务，存储失败保持保护及待写结果。
- UX-001：Wails/App提供明确核对和指定会话结束及确认，显示安全错误/退出码/下一步；待核对即使无PID仍锁关键配置。活动使用environmentId/sessionId，旧记录不能控制后来新开的浏览器；批量关闭不自动强杀。
- 监督器/Windows身份/adapter后台通过，原A根故障/B独立/管理器Kill保留；10月6日实际普通停止超时后的原Job ForceStop/完整清理、保数据重开/B不变/旧session拒绝通过。[补验](verification/V1-local-acceptance.md)。完整会话恢复/跨登录与人工仍待验；准确Job全树核对不以根退出替代，[清单](verification/T07.md)。

### T08 原生代理配置与前检（后台/DPAPI通过，外部与人工待验）

- PRX-001：[`解析`](../internal/proxy/parse.go)与[`导入/编辑/删除服务`](../internal/workspace/proxies.go)分离；URI/兼容文本/IPv6，原行号/错误和共享重复组，选择有效行提交。schema5及[`受保护存储`](../internal/workspace/proxy_storage.go)的user DPAPI密文引用、HMAC请求去重、事务替换/回滚、修订和引用保护，普通响应无用户名密码，不重生成环境seed。
- PRX-001、UX-001：[`HTTP/HTTPS前检`](../internal/proxy/check.go)的连接/TLS/隧道认证/目标访问/实际出口与时刻，[`异步终态`](../internal/workspace/proxy_checks.go)的取消/有界资源、结果待保存不重发网络及重开中断；活动关联真实代理operation错误，不把失败文案投影成功。SOCKS5可保存、检查不支持，不跳TLS或静默直连。
- NativeProxyManager/Wails安全字段接native，keep/replace/clear不从投影回填认证；代理库/服务/adapter与真实Windows DPAPI通过。网页原型仍demo，新UI/独立公共出口待验，[清单](verification/T08.md)。本票不代替T09/T11。

### T09 独立认证代理通道（后台/protected本机通过，外部与人工待验）

- PRX-001、ENV-003：[`桥接`](../internal/proxy/bridge.go)、[`同通道前检`](../internal/proxy/bridge_check.go)、[`运行接入`](../internal/workspace/runtime_network.go)；固定HTTP/HTTPS上游、独立session监听/生命周期，HTTP转发、HTTPS目标CONNECT与代理TLS分开，失败不直接拨号目标。报告仅安全ChannelID/修订/阶段/时间，秘密不进入参数/RPC。
- PRX-001、DATA-001：[`Windows调用进程核对`](../internal/kernel/proxy_guard_windows.go)和创建前QUERY副本绑定；仅当前host+token前检或准确Job客户端，其他进程拒绝。创建后身份读取失败仍保留准确Job/目录资源直到全树确认，锁外解密不阻塞其他查询/停止；重开只核对不复活桥。
- UX-001/ENV-003：固定保存direct/proxy策略，未绑定才确认直连；错channel/修订拒绝。桥/内核/服务/adapter通过，正式148代理与保沙箱本机链路实跑；外部认证/出口/换代理与人工仍待验，[清单](verification/T09.md)。

### T10 SOCKS5与远端目标解析（后台通过，真实SOCKS5/DNS待验）

- PRX-001、ENV-003：[`SOCKS5`](../internal/proxy/socks5.go)、Bridge及服务，RFC1928/1929指定方法不降级、IDNA DOMAINNAME远端目标DNS/IPv4/IPv6字节，HTTP origin-form/HTTPS隧道不漏认证、只拨上游；BND不当出口IP，错误有准确类型。
- PRX-001：导入/replace协议校验，存储解码中性，keep不解密/改写；切协议不兼容建桥前PROXY_AUTH_INVALID。独立检查临时Bridge，normalStart自己的新桥同通道重检；schema5表不变，安全resolutionPolicy可选，RPC不能覆盖DNS/降级。
- UX-001/ENV-003：共享阶段区分代理host/目标DNS、认证链路；库/服务/adapter回归通过，真实SOCKS5浏览器和独立DNS/IPv6/UDP及人工页待验，[清单](verification/T10.md)。

### T11 网络故障/门禁（正式接入/本机故障通过，远端待验）

- ENV-003、PRX-001：[`闭锁/巡检`](../internal/proxy/bridge_watch.go)及准确Job独立停止，不等服务锁/DB；请求取消/上传故障不误关整个桥。确认全树及桥退出前保护原数据，A闭锁不更改B，普通前检错误不冒充运行故障。
- ENV-003、UX-001：[`网络根因`](../internal/workspace/runtime_network_fault.go)、持久化/恢复与native说明；network_error及清理阶段、启动含nil process真实闭锁、ForceStop/重开保根因；未终结Stop的预留与error展示分离，终态保存前不能被新Start替换。
- PRX-001：双层门禁与正式owner、同package桥/准确Job/实际前检绑定，缺失失败未知拒绝；正式启动/故障/恢复及服务回归通过，独立检查不解锁。外部全路径/跨登录及人工待验，本票不记完整。

### T12 指定环境Cookie导入（双真实会话/后台通过，完整验收待补）

- CK-001：[`解析`](../internal/cookies/parse.go)与[`安全预览`](../internal/workspace/cookie_import.go)分离；环境/修订/session绑定，空值/JSON-vs-Netscape时间/hostOnly/安全属性/分区/冲突和不支持项明确。写前拒会改变另一键的路径/作用域，现存冲突未知不填0，不持久化秘密。
- CK-001、DATA-001：[`窄内核控制`](../internal/kernel/cookies_windows.go)单条读→同键matchskip→写→完整读回组合串行，scope为准确私有pipe；不任意CDP/SQL/URL，无结果/属性差异不计verified。明确全量清空才碰无关键；失败重试只合并、重开不自动重放。
- CK-001、UX-001：[`任务/观测`](../internal/workspace/cookie_worker.go)部分成功/unknown、取消/落盘pending与lease，退出/核对不提前释放；[`native对话框`](../src/components/NativeCookieImport.tsx)明确空白启动原链、不绕代理门禁，隐藏输入/值与安全逐项结果，关闭后可取消。
- 解析/内核/服务/adapter回归及双protected真实写后读回/A-B独立通过；正常资源持有/proxy空白启动UI模型已修并定向通过。分区/到期/清空/取消全真实矩阵及人工窗口见[缺口](verification/T12.md)。

### T13 持久创建/复制/代理分配与真实分页（后台/production目录通过）

- ENV-001、ENV-002：[`批次计划`](../internal/workspace/batch_preview.go)、[`逐项worker`](../internal/workspace/batch_worker.go)及schema6，创建大count虚拟预览、不预展开，环境/结果/统计同事务，取消/资源不足保已提交项；重开中断、明确继续索引跳过完成项、不重做身份。提交不明按[`受理核实`](../internal/workspace/batch_acceptance_recovery.go)挂原worker，未核实不调度。
- ENV-002、FP-001、DATA-001：克隆配置新ID/seed并重新编译原精确构建，独立[`可见空目录`](../internal/kernel/empty_profile_windows.go)与journal归属marker，不复制源Cookie/账号/网站数据、不清空外来目录；普通写入也受[`seed预约`](../internal/workspace/seed_ownership.go)与owner lease保护，不把pins称同SID强隔离。
- PRX-001：预览冻结明确ID→节点与修订，共享/不绑定总数确认；Assign逐项忙/旧修订冲突、只改proxy绑定/JSON和环境修订，seed/档案不变、不自动轮询复用。proxy网络编辑不以UsedBy展示样本裁定忙状态，T11启动门禁保留。
- ENV-001、UX-001：[`服务端列表分页`](../internal/workspace/environment_query.go)与[`native批次对话框`](../src/components/NativeBatchDialog.tsx)，统计/筛选来自实际服务、档案/ref/session随一页加载；[`旧尝试明细`](../internal/workspace/batch_history.go)只用原事件和此前完成项，不混后来成功。按plan/op/offset/选择代次处理迟到结果、终态后读最终页、modal键盘保护；跨页启动每个明确ID重新读真实策略/修订，缺失不作直连。
- 服务/目录/adapter通过；本轮production clone新seed/空目录与源合成文件保留、31项实际目录/分页/请求去重通过。百万虚拟预览不冒充真实规模，人工及资源不足待验，[清单](verification/T13.md)。

### T16 恢复只读预检（后台/DPAPI提示与精确build候选通过）

- BKP-001：[`严格包读取`](../internal/backup/read.go)、[`配置校验`](../internal/workspace/restore_configuration.go)与[`预览`](../internal/workspace/restore_preview.go)；独立分发避免待写日志flush，固定选包/完整摘要/可信schema与记录闭包，当前数据库及浏览目录只读。
- CORE-001、FP-001、PRX-001：原ID/seed与历史、冲突/覆盖影响、同精确hash内核映射与只读文件核对、当前用户凭据可用性；不生成新身份、不运行内核或网络。
- UX-001：[`NativeRestoreManager`](../src/components/NativeRestoreManager.tsx)专用预检与分页、取消、摘要/过期/凭据和内核提示；demo/native分离；正式恢复接续T17。[检查与待验收](verification/T16.md)。

### T15 完整本机导出（本地已实现待验收）

- BKP-001、DATA-001：[`受理/正常停止/worker`](../internal/workspace/backup_worker.go)，范围来自all全库或所选明确ID，owner预约阻重新Start，不把正常Stop受理当完成，不升级强制结束。schema7发布journal、取消/存储pending和重开不重复副作用；已有proxy启动门禁不变。
- BKP-001、FP-001、PRX-001：[`独立一致SQLite快照`](../internal/workspace/backup_snapshot.go)实际WAL/read事务/online Backup API，范围闭包/离线VACUUM、档案当前及全历史原seed/ref/准确kernel hash，凭据原ref+DPAPI密文不解密。操作/会话/活动/混合batch/其他备份/去重key明确不恢复。
- BKP-001、DATA-001：[`只读数据固定`](../internal/backup/profile_windows.go)、[`独立ZIP/全量读回`](../internal/backup/format.go)与[`原句柄发布`](../internal/backup/output_windows.go)，拒links/hardlinks/变化、保空目录，manifest逐文件摘要；临时文件非成功包，不覆盖工作区或已有目标，不称同SID恶意writer强隔离。
- UX-001、BKP-001：[`NativeBackupManager`](../src/components/NativeBackupManager.tsx)正常停止确认/全量与明确选定/只返回host token和安全名称/取消与历史读回/实际published摘要；浏览数据敏感和同Windows用户限制可见，不携带内核、不承诺登录便携。旧原型JSON不进入native，不标恢复已实现。
- 独立[`初始化事实`](../internal/workspace/data_initialization.go)同事务保存、单调不重置；启动受理/最新失败不当未初始化证明，迁移旧环境保持未知、正常停止后只重读原冻结ID，worker锁外不碰mutable observation。Adapter旧响应须仍有原pending归属，错误模式/报告不推未受理，页面读取核实原受理统一消费旧输出授权。
- 文件/包/服务/adapter后台通过，真实选定A三存储导出/恢复实跑；all11非页8及DPAPI原密文/ref由服务回归核对。多真实运行环境正常停后备份和实际空间/权限失败、人工仍待验，[清单](verification/T15.md)。

以下关联于 2026-09-30 发布，表示计划实现范围，不能据此判断已完成。当前状态与 blocking 依赖以 GitHub 为准；完整顺序见 [开发票据索引](ISSUES.md)，共同范围见 [总规格 Issue](https://github.com/axgiroud312-byte/prism-local-browser/issues/1)。

| 需求 ID  | 实现或专项验证 Issue                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | 整体验收                                                                       |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| ENV-001  | [T13 · #14](https://github.com/axgiroud312-byte/prism-local-browser/issues/14)、[T14 · #15](https://github.com/axgiroud312-byte/prism-local-browser/issues/15)、[T19 · #20](https://github.com/axgiroud312-byte/prism-local-browser/issues/20)                                                                                                                                                                                                                                                                                                                             | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| ENV-002  | [T01 · #2](https://github.com/axgiroud312-byte/prism-local-browser/issues/2)、[T02 · #3](https://github.com/axgiroud312-byte/prism-local-browser/issues/3)、[T05 · #6](https://github.com/axgiroud312-byte/prism-local-browser/issues/6)、[T13 · #14](https://github.com/axgiroud312-byte/prism-local-browser/issues/14)                                                                                                                                                                                                                                                   | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| ENV-003  | [T06 · #7](https://github.com/axgiroud312-byte/prism-local-browser/issues/7)、[T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8)、[T09 · #10](https://github.com/axgiroud312-byte/prism-local-browser/issues/10)、[T10 · #11](https://github.com/axgiroud312-byte/prism-local-browser/issues/11)、[T11 · #12](https://github.com/axgiroud312-byte/prism-local-browser/issues/12)、[T14 · #15](https://github.com/axgiroud312-byte/prism-local-browser/issues/15)、[T18 · #19](https://github.com/axgiroud312-byte/prism-local-browser/issues/19) | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| FP-001   | [T02 · #3](https://github.com/axgiroud312-byte/prism-local-browser/issues/3)、[T05 · #6](https://github.com/axgiroud312-byte/prism-local-browser/issues/6)、[T13 · #14](https://github.com/axgiroud312-byte/prism-local-browser/issues/14)、[T17 · #18](https://github.com/axgiroud312-byte/prism-local-browser/issues/18)                                                                                                                                                                                                                                                 | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| FP-002   | [T04 · #5](https://github.com/axgiroud312-byte/prism-local-browser/issues/5)、[T05 · #6](https://github.com/axgiroud312-byte/prism-local-browser/issues/6)、[T20 · #21](https://github.com/axgiroud312-byte/prism-local-browser/issues/21)                                                                                                                                                                                                                                                                                                                                 | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| PRX-001  | [T08 · #9](https://github.com/axgiroud312-byte/prism-local-browser/issues/9)、[T09 · #10](https://github.com/axgiroud312-byte/prism-local-browser/issues/10)、[T10 · #11](https://github.com/axgiroud312-byte/prism-local-browser/issues/11)、[T11 · #12](https://github.com/axgiroud312-byte/prism-local-browser/issues/12)、[T13 · #14](https://github.com/axgiroud312-byte/prism-local-browser/issues/14)                                                                                                                                                               | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| CK-001   | [T12 · #13](https://github.com/axgiroud312-byte/prism-local-browser/issues/13)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| CORE-001 | [T04 · #5](https://github.com/axgiroud312-byte/prism-local-browser/issues/5)、[T06 · #7](https://github.com/axgiroud312-byte/prism-local-browser/issues/7)、[T16 · #17](https://github.com/axgiroud312-byte/prism-local-browser/issues/17)、[T20 · #21](https://github.com/axgiroud312-byte/prism-local-browser/issues/21)                                                                                                                                                                                                                                                 | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| BKP-001  | [T15 · #16](https://github.com/axgiroud312-byte/prism-local-browser/issues/16)、[T16 · #17](https://github.com/axgiroud312-byte/prism-local-browser/issues/17)、[T17 · #18](https://github.com/axgiroud312-byte/prism-local-browser/issues/18)、[T18 · #19](https://github.com/axgiroud312-byte/prism-local-browser/issues/19)、[T20 · #21](https://github.com/axgiroud312-byte/prism-local-browser/issues/21)                                                                                                                                                             | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| DATA-001 | [T06 · #7](https://github.com/axgiroud312-byte/prism-local-browser/issues/7)、[T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8)、[T12 · #13](https://github.com/axgiroud312-byte/prism-local-browser/issues/13)、[T15 · #16](https://github.com/axgiroud312-byte/prism-local-browser/issues/16)、[T17 · #18](https://github.com/axgiroud312-byte/prism-local-browser/issues/18)、[T19 · #20](https://github.com/axgiroud312-byte/prism-local-browser/issues/20)                                                                                 | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| UX-001   | [T01 · #2](https://github.com/axgiroud312-byte/prism-local-browser/issues/2)、[T02 · #3](https://github.com/axgiroud312-byte/prism-local-browser/issues/3)、[T03 · #4](https://github.com/axgiroud312-byte/prism-local-browser/issues/4)、[T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8)、[T08 · #9](https://github.com/axgiroud312-byte/prism-local-browser/issues/9)、[T14 · #15](https://github.com/axgiroud312-byte/prism-local-browser/issues/15)、[T18 · #19](https://github.com/axgiroud312-byte/prism-local-browser/issues/19)       | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |
| DOC-001  | [T01 · #2](https://github.com/axgiroud312-byte/prism-local-browser/issues/2)、[T03 · #4](https://github.com/axgiroud312-byte/prism-local-browser/issues/4)                                                                                                                                                                                                                                                                                                                                                                                                                 | [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) |

## 完整验收证据要求（已有部分结果见集中报告，非全部尚未实现）

| 范围                             | 关联需求          | 完成所需证据                                                                                    |
| -------------------------------- | ----------------- | ----------------------------------------------------------------------------------------------- |
| 真实浏览器进程、独立目录及互斥锁 | ENV-003、DATA-001 | 在 Windows 实际启动两个环境；核对各自目录、退出与崩溃恢复；同一环境重复启动不会得到两个写入者。 |
| 固定内核的下载、安装、校验和升级 | CORE-001、FP-002  | 记录来源、实际版本与校验值；失败可定位；升级前后保留完整备份并验证兼容性。                      |
| 指纹参数生效与持久身份           | FP-001、FP-002    | 从运行中的固定内核读取参数；重开/恢复后核对稳定字段，记录不支持或尚未验证项。                   |
| 真实代理认证、DNS 与出口检查     | PRX-001、ENV-003  | HTTP/HTTPS/SOCKS5 认证成功与失败场景；断线行为及是否直连回退的网络证据。                        |
| Cookie 写入指定浏览器            | CK-001            | 按指定环境通过 CDP 写入并读回核对；逐项错误、分区字段和到期语义符合内核能力。                   |
| SQLite、凭据保护和持久事务       | UX-001、DATA-001  | 在进程终止、磁盘写入失败和恢复场景下，数据有一致且可恢复的状态；凭据不明文落入一般日志。        |
| 真实浏览数据备份与恢复           | BKP-001           | 关闭环境、完整打包、校验、暂存切换、失败回滚，再真实重开验证；JSON 原型快照不替代该证据。       |
| Windows 应用打包及目标网站功能   | ENV-003、UX-001   | 干净 Windows 环境安装/启动；完成目标网站登录、导航、上传下载等明确选择的流程。                  |

## 使用本表进行验收

每项验收记录应标明需求 ID、输入数据、操作步骤、预期结果、实际结果和证据位置。领域单元测试证明解析和状态规则；截图证明当时的界面；真实浏览器与网络能力需要相应进程、读值和网络证据。几类证据不能相互替代。

本表列出的后续能力不要求在当前前端原型中假装完成。是否达到当前交付范围，请同时查看 [PRD 的原型范围](PRD.md#当前原型和桌面目标的差异) 与 [验收记录](ACCEPTANCE.md)。
