[项目首页](../../README.md) · [执行规则](../GOAL.md) · [进度](../PROGRESS.md) · [验收记录](../ACCEPTANCE.md) · [需求追踪](../TRACEABILITY.md)

# #28–#30 核心流程与页面验收

日期：2026-10-06。交付层级：开发检查方式、可操作前端及现有应用服务接入；**不是新安装包或真实桌面验收**。

## 1 当前事实与分工

- [#28](https://github.com/axgiroud312-byte/prism-local-browser/issues/28) 已实现并有本地/远程实际结果，提交 `53ac88334ad813d3bd15eb537bda7af5b1b78695` 已保留。已验证不等于已合并主分支或关闭 issue。
- [#30](https://github.com/axgiroud312-byte/prism-local-browser/issues/30) 唯一承接环境表和统一创建/编辑窗口实现；[#29](https://github.com/axgiroud312-byte/prism-local-browser/issues/29) 对整体方向和全部六项验收逐项核对，不重复实现页面、不因 #30 完成自动勾选。
- 本文是本批次文档及验收准备。基线 `245634c037a929160edc05ee0408cba5c33723c2` 包含新行为测试定义，不代表这些测试已通过。#29/#30 集成源码、点击检查、截图和最新远程结果仍为 **PENDING**，由协调者填入真实命令、源码提交、数量、时间及附件。
- 协调者拥有 `tests/ui/*`、三张截图及最终共享验证/PR；实现者拥有前端源码；本次文档提交不能单独解决 #29。文档代理不运行全 UI 套件、不构建/安装、不修改 issue 或 PR。
- 正式验收保持 **4/21**，T05–T21 / #6–#22 保持 OPEN；历史 candidate.4/candidate.6 的来源、版本、摘要、签名及缺口以 [首版报告](V1-final.md) 和 [候选回执](V1-candidate-acceptance.json) 为准，未重建或改写。

`PASS` 只表示表中明确的证据层级；`PENDING` 表示尚无本轮已核对输出，不能当作通过、跳过或取消验收要求。

## 2 参考分别影响什么

| 参考 | 提炼的交互规律 | 本批次对应界面；待预览核对 |
| ---- | -------------- | -------------------------- |
| [Ant Browser README/界面预览](https://github.com/black-ant/Ant-Browser#界面预览) | 实例优先、顶部分组/搜索/筛选、新建入口、直接启动/停止/配置；名称、内核、网络在一条创建流程中选择 | 环境表及工具栏；行内打开/关闭、清晰编辑入口；一个常用创建窗口，而非让单个操作先读技术计划 |
| 旧前端 `BxDQjoAQ.js` | 紧凑表格、分组筛选、行操作密度 | 分组/网络/内核/状态分列，多选范围和逐项反馈明确 |
| 旧前端 `hVosDY7g.js` | 同窗分区、随机指纹入口、弹窗返回 | 常用字段集中、自动指纹摘要和“换一套”；偏好/技术详情收起，保存和取消明确 |
| 旧前端 `BFRaQcix.js` | 紧凑的服务驱动内核选择 | 从现有服务选可用精确构建，不硬编码参考版本；没有可用内核给准备引导 |
| 旧前端 `BClmd_Pk.js` | 导入入口、解析预览与返回流程 | 复用当前代理导入，返回原环境草稿，保留名称/分组/内核/seed |

旧前端文件相对于归档的 `03-原始发布前端与缓存代码/readable-web-frontend/js/` 定位；它们是 Vue/Quasar 发布 bundle，不是迁入 React 的组件。仅提炼规律，未复制代码、品牌、资源、商业接口或用户数据；归档及私人路径不进入仓库。Ant 的云服务、自动化等不引入，其代理异常临时直连行为也不采用。

## 3 证据登记

| 编号 | 实际或计划证据 | 当前结果及限制 |
| ---- | -------------- | -------------- |
| E28-L | #28 提交上的本地 `npm run check`，原 13 条 UI | **PASS：13/13，41.8 秒**，由 #28 实际记录核对；首次缺 Playwright 浏览器的失败保留。不是本次文档代理重跑。 |
| E28-R | [CI 37419040709](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37419040709) | **PASS：单个 Browser clicks job，1 分 27 秒；13/13，34.1 秒**。成功运行未触发失败附件上传，不声称实测过上传。 |
| E-RED | 协调者基线 `npm run test:ui -- tests/ui/environment.spec.ts -g 'one window selects'` | **预期 RED：1 failed，6.3 秒**，旧窗口没有可见“换一套”；报告/trace 在 `output/playwright/`。失败不改记成功，新实现复验 PENDING。 |
| E-SOURCE | #30 集成提交；`ApplicationService`、Wails/Demo 适配层及请求路径复核 | **PENDING**：记录最终源码 SHA 和人工复核结论，不把未提交或未集成源码当最终交付。 |
| E-DEMO | [environment.spec.ts](../../tests/ui/environment.spec.ts) 的创建/编辑/取消/重载、服务内核、自动档案、代理导入、分组/搜索/部分失败、窄窗口 | **PENDING**：记录定向命令、实际数量/时间、失败及修复，不删除旧断言。localStorage 只证明 demo 配置持久化。 |
| E-BRIDGE | [native-boundary.spec.ts](../../tests/ui/native-boundary.spec.ts) 的创建一次/打开失败重试、显式 direct、混合批量策略、无可用内核、坏桥阻断 | **PENDING**：合成注入 Wails bridge 的请求与页面反馈；fixture 的 sessionStorage 只用于测试重载，不是 SQLite。 |
| E-CHECK | 集成后共享的一次 `npm run check`（#29/#30 共用） | **PENDING**：直接启动 Vite，记录最新源码 SHA、完整 UI 数量、时间与报告。不重复按票构建或再跑全套。 |
| E-CI | 最新交付 head 的远程 Browser clicks job | **PENDING**：记录实际 run URL/head、单 job 及结果；不能用 E28-R 代替新行为远程检查。 |
| E-SHOT-LIST | `docs/screenshots/issue30-environments.png` | **PENDING**：协调者从 Vite 源码页面生成、检查合成数据；展示分组/搜索、网络/版本/状态、行操作。 |
| E-SHOT-CREATE | `docs/screenshots/issue30-create.png` | **PENDING**：展示同一常用窗口、直连/代理、服务内核、自动摘要/换一套和两种创建按钮，高级默认收起。 |
| E-SHOT-NARROW | `docs/screenshots/issue30-narrow.png` | **PENDING**：展示窄窗口核心表单/列表与主操作可达；截图不替代实际点击和焦点检查。 |
| E-DOCS | 本次文档准备的 `npm run check:docs`，以及文本/边界核对 | **PASS（文档静态范围）**：52 份文档、本地链接、12 个需求、6 个路由、4 份内嵌文档。首轮因追踪表漏写 `activity` 路由失败，补回后复验通过；不弱化检查。源码集成后的实际行为/截图一致性仍 PENDING；截图不存在时只保留计划路径。 |

上述新测试覆盖定义仍可能随一致的实际控件标签调整；不能为通过而放宽稳定身份、操作范围、无技术确认、失败恢复或窄窗口断言。`output/playwright/` 中的报告、trace 和临时截图只记录实际产物，不把未来文件当已存在。

## 4 #28 验收矩阵（已有结果，不重复建设）

| 项 | issue 原验收要求 | 证据/结果 |
| -- | ---------------- | --------- |
| 28-1 | 普通 PR/main push 自动运行一个浏览器点击测试 job。 | **PASS**：E28-R；[workflow](../../.github/workflows/check.yml) 只含 Browser clicks。 |
| 28-2 | 默认流程没有生产构建、Go/Wails、安装包、安装卸载或产品内核探测步骤。 | **PASS**：workflow、[package.json](../../package.json)、E28-R 实际步骤。 |
| 28-3 | 页面测试自动启动/关闭 Vite，现有可观察行为断言通过；不靠跳过测试掩盖失败。 | **PASS**：E28-L/E28-R，13/13；[Playwright 配置](../../playwright.config.ts)。 |
| 28-4 | CI 失败可下载合成数据测试报告、截图和 trace。 | **配置核对通过**：`if: failure()` 上传 `output/playwright/`、保留 7 天；成功 run 上传跳过，实际失败上传未执行。 |
| 28-5 | 当前开发约定与命令一致，记录本地与远程实际结果。 | **PASS（#28 范围）**：E28-L/E28-R、[工程规范](../ENGINEERING.md)、[执行规则](../GOAL.md)。 |

## 5 #29 整体流程验收矩阵（逐项保留）

| 项 | issue 原验收要求 | 协调者必须核对的具体结果 | 证据/当前状态 |
| -- | ---------------- | ------------------------ | ------------- |
| 29-1 | 提供与 Ant Browser 对照后的环境列表/创建流程预览。 | 当前源码可操作，不只静态图；核对第 2 节各参考影响，并提供新环境表、常用创建窗口预览。 | E-SOURCE、E-SHOT-LIST、E-SHOT-CREATE；**PENDING** |
| 29-2 | 浏览器点击可完成创建、编辑、分组/搜索、启停交互及重载后的配置保留。 | 创建/创建并打开、编辑保存、分组筛选/修改、搜索空结果恢复、单个及批量打开/关闭；关闭重开并重载后保留名称/分组/代理/精确内核/seed；部分失败不影响其他项。 | E-DEMO、E-BRIDGE、E-CHECK；**PENDING** |
| 29-3 | 单个环境无需用户查看批次计划、配置哈希或内部修订即可完成常用操作。 | 默认收起高级详情仍能创建、创建并打开、编辑保存和重开；native 单条走 `Environment.Create` 而非 `Batch.Preview`；打开无内部 ID/修订确认。保留未保存放弃、删除/恢复等必要确认。 | E-SOURCE、E-DEMO、E-BRIDGE、E-SHOT-CREATE；**PENDING** |
| 29-4 | 直连/代理和内核版本可明确选择，指纹可一键生成且保存后稳定。 | 来自服务的不同版本、直连和已有代理均可选；新建自动预览，换一套仅改草稿，取消不提交；普通编辑/换代理/关闭重开不换 seed，不换已存内核/数据引用；无可用内核引导、代理失败不直连。 | E-SOURCE、E-DEMO、E-BRIDGE；**PENDING** |
| 29-5 | 界面连接现有应用服务，demo 与注入 bridge 的验证结果如实标记；真实桌面未执行的部分保持未验证。 | 保存/打开消费原契约及白名单、修订/幂等保护；打开失败只重试已创建 ID；坏 bridge 不回退 demo。分别登记 demo/合成桥结果，并明确未运行 SQLite、真实内核/进程/流量/安装。 | E-SOURCE、E-DEMO、E-BRIDGE、第 7 节；**PENDING** |
| 29-6 | 默认只运行 #28 的浏览器点击测试，按改动更新相关用例，不反复构建或打包。 | 按实际标签调整行为测试而不弱化断言；定向检查后共享一次 `npm run check`，核对最新远程单 job；交付记录含命令/实际结果，不运行默认范围外构建。 | E-CHECK、E-CI、E-DOCS；**PENDING** |

**六项全部尚需协调者给出最终证据和结论。** #30 的实现、本文、旧 13/13 或历史候选通过都不能替代 #29 的这一轮检查。

## 6 #30 实现验收矩阵（不自动转为 #29 通过）

| 项 | issue 原验收要求 | 证据/当前状态 |
| -- | ---------------- | ------------- |
| 30-1 | 提供当前源码可运行的页面和合成数据截图，并简述两份参考分别影响了哪些界面；完成服务接入，不止交付静态图。 | 第 2 节、E-SOURCE、E-DEMO/E-BRIDGE、E-SHOT-LIST/CREATE；**PENDING** |
| 30-2 | 浏览器点击可完成分组/搜索、单个与批量启停交互，列表显示正确的网络、内核和状态，失败有可恢复反馈。 | E-DEMO/E-BRIDGE；核对所选 ID、逐项成功/失败和修复后重试；**PENDING** |
| 30-3 | 同一新建窗口可选择直连/已有代理、内核版本、生成或更换指纹，完成“创建”及“创建并打开”；代理导入返回后草稿仍在。 | E-DEMO/E-BRIDGE；补核对代理导入成功/取消/失败往返、顶层 Escape/焦点及所有草稿字段；**PENDING** |
| 30-4 | 编辑保存后重新打开或刷新能读到保存内容；取消不提交；普通编辑不换 seed；打开失败不重复创建。 | E-DEMO/E-BRIDGE；核对创建提交次数、原 ID 和保存/取消边界；**PENDING** |
| 30-5 | 常用屏幕和窄窗口下，表单、列表及主操作可操作，不被遮挡；日常单个环境操作无需经过技术报告或批次确认页。 | E-DEMO、E-BRIDGE、三张截图；实际点击/滚动和焦点核对，不只截图；**PENDING** |
| 30-6 | 沿用 #28 的轻量检查：按改动调整现有 Playwright 点击用例并运行 `npm run check`，直接使用 Vite 源码页面；不默认运行构建、打包或完整桌面验收。在 PR 中记录 demo/模拟 bridge 的实际覆盖与真实桌面未验证项。 | E-CHECK/E-CI；最终 PR 由协调者记录；**PENDING** |
| 30-7 | 按实际行为更新相关 PRD 和需求追踪；保留原有数据及身份保护，关联本 issue 提交交付记录。 | 本次文档准备、E-SOURCE/E-DOCS；源码集成后再次核对 [PRD](../PRD.md)/[追踪](../TRACEABILITY.md)，提交关联 #29/#30；**PENDING** |

## 7 验证边界与最终收尾

- demo：Vite + Playwright 的实际页面点击及 localStorage 演示配置读回；不启动产品 Chromium。
- 注入 bridge：合成请求/响应、服务选项和页面状态；native 标记来自 fixture，不是安装核验、SQLite 事务、DPAPI、Windows 进程或网络流量证据。fixture `sessionStorage` 重载保留不能写成“原生数据库重开通过”。
- 三张新截图：只证明对应源码/尺寸下可见布局，用合成数据，不代替服务或桌面验收；生成、内容检查与引用更新均待协调者完成。
- 本次文档代理不安装依赖/工具链；本批次不新增工具链、不生产构建、不运行 Go/Wails 全套、安装卸载、Windows UI Automation、真实内核探针或系统级实验。#28 的现有 CI 仍按需安装 npm 依赖和 Playwright 测试浏览器，不是安装产品内核。历史安装/真实桌面证据只保留原范围，不重用来证明新 UI。
- 独立远端出口、人工桌面完整流程、精确候选的干净 Windows 等历史缺口仍保留；没有新真实 native 验收声明，也不自动关闭 T05–T21。

协调者收尾顺序：集成 #30 源码与本文 → 对照上表核实全部 #29/#30 条目（包括取消、身份、代理导入恢复、部分失败、窄窗口/焦点、低频入口与忙字段保护）→ 记录定向输出及三张已检查截图 → 共享一次 `npm run check` → 核对最新远程 Browser clicks 输出 → 在本文、ACCEPTANCE/PROGRESS 和最终 issue/PR 如实登记结果。若某项仍缺证据或失败，保持 PENDING/FAIL 并修复，不豁免，也不以文档提交代替完成。
