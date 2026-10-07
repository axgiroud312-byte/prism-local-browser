[唯一视觉参考](../UI_REFERENCE.md) · [公共责任](../UI_CONTRACT.md) · [合成截图说明](../screenshots/issue34/README.md)

# #34 环境窗口交付与检查记录

日期：2026-10-06。关联 [#34](https://github.com/axgiroud312-byte/prism-local-browser/issues/34)。分支 `codex/issue34-environment-windows`，从 `9e256a7` 开始，已快进合入集成分支的 `0e0a517`（含公共模态/工作区阻断修复）。

实现与新增定向测试提交：`c52122c`。本页/72张合成图在后续独立证据提交中；未改变受测UI。取证后补入的缺可选提交方法保护与窗口根焦点属性不改变这些合成状态的可见布局。

**交付为可集成的前端窗口、既有服务行为和合成证据；不是全状态 1:1 或真实桌面验收。** shared App 的正式挂载、400px 未保存/强制结束确认及 #35 唯一代理导入的最终往返由 MAIN / #37 收尾，本票没有写入 shared 文件。

后续接入已经完成：正式窗口与确认见 [49项及16图记录](issue34-integration.md)，唯一代理导入及原草稿/实际可编辑返回见 [#37整组记录](issue37.md)。最终 `70943b0` 50状态/100张实际App图全部核对，22项有直接支持参考并列适配、78项仅容器映射；包含强制会话、pending/accepted/unknown创建、文件错误/未知Cookie、写失败/原ID打开失败、批次/回收保护。Cookie hover修差后6/6，通知最后修差后5/5，双尺寸计算与真实chooser回草稿通过。共享196/196只归属其实际受测 `681f823`，不冒称最终head全套/全量1:1/native。本页72张外部组件预览仍保留原来源，不当作最终App视觉证据。

## 1 已交付与责任

| 文件/导出 | 行为与接入责任 |
| --- | --- |
| `EnvironmentForm` / `EnvironmentFormProps` | 连续的基础→代理→常用→指纹四区和外挂导航；受控草稿、服务内核/代理、能力禁用、指纹详情/历史。原 props 兼容，新增可选 `showGenerateAction` 默认 true。 |
| `EnvironmentEditorWindow` | 660px 右贴边 drawer，固定标题/底部和独立正文；底部“换一套”、创建/创建并打开/保存及未知创建核实。App 继续拥有草稿、请求、保存/打开重试、关闭与焦点生命周期。 |
| `EnvironmentKernelSelect` | 264px DOM 下拉，选项只来自传入可用服务记录；保留 `select[aria-label="浏览器内核"]`。Escape 只关菜单，Tab 关菜单并留在窗口。 |
| `EnvironmentWindowFrame` / `EnvironmentConfirmation` / `EnvironmentTaskResult` | 仅视觉结构；不新增键盘监听、body lock、请求或 adapter。400px 确认用于既有管理器，也提供给 MAIN 的未保存/force 控制器。 |
| `DemoCookieImportWindow` / `DemoEnvironmentRemoveWindow` | demo 1050px 文本预览与400px移除确认；App 持有原解析、写入失败保护和精确选择回调。不得套进旧 modal 留下第二层 frame。 |
| `NativeCookieImport` | 保持原 `application/workspace/environment/onClose`；1050px 文本/预览和520px UTF-8文件选择。未知受理冻结原请求，失败子集仅合并重试；结果单调合并。 |
| `NativeBatchDialog` | 原 props 不变；1040px冻结计划/当前和历史逐项结果，400px结果容器。取消只影响剩余项，继续不重做已完成 ID。 |
| `NativeRecycleManager` | 原 props 不变；页形密集回收列表、400px移除/删除确认、440px原身份恢复、400px保护结果。列表快照作为 inert 背景，确认 scrim 正确覆盖它。 |

样式仅在本票的 `environment-windows.css`、`environment-batch.css`、`environment-recycle.css`、既有 `native-cookie.css` / `fingerprint-revision.css` 内调整。未改 `src/App.tsx`、全局样式、入口、公共 Reference UI、`native-proxy.css`、领域模型、服务、适配层、后台、SQLite、RPC 或文件格式；无新增依赖/商业资源。

### 保留的业务边界

- 自动预览/普通编辑/代理变化不重生成 seed；“换一套”只改草稿，取消不写入。克隆新身份/空数据，找回原身份/原数据引用。
- native running、resourcesPending、persistencePending、needsReconcile 仍只允许名称/分组/备注；关键字段和“换一套”禁用。服务仍作最终验证。
- App 保留原 `requestId/previewId/expectedRevision`、未知创建核实和创建后原 ID 打开重试；不新建重复环境，不静默直连。
- Cookie 解析成功清除原文，预览/结果无 value；全量清空需明确确认。失败/unknown/cancelled 子集使用新请求、`policy: merge`，不重复已通过行或再次清空。受理未知则重发完全相同的原请求，核实前阻止关闭丢失它。
- Cookie 明确的受理前校验/修订/会话拒绝可返回修改；存储、传输或回执不明不能当作“没有写入”。缺少可选提交方法时没有发送请求，不进入未知锁定。已知任务关闭后可继续/重开查看，关闭不是撤销。
- 批次旧尝试保留冻结结果；26项分页的失败读页不能伪装最终明细，显式重读不重新执行。保留计划整体直连/共享节点确认及精确逐项代理映射。
- 回收保留准确选择、数据/备份影响、原请求核实、取消后已完成项和维护保护；不是用恢复走新建复制。

## 2 实际定向检查

使用已有依赖与两个隔离 Vite：源码5194，外部挂载预览5204；未触碰主预览5173。外部预览转换只在 Vite 内存中替换 App 的 drawer/demo Cookie/移除挂载，机械复用实际 Cookie 保存回调，**没有写回 shared App，也没有替代正式集成**。

`tdd` / `code-review` skill 不可用，采用可观察 RED→GREEN 和本票自审；#37 的独立审查仍待执行。新增 `tests/ui/environment-windows.spec.ts` 5项、`tests/ui/environment-cookie-window.spec.ts` 4项；既有测试未编辑或放宽。

以下 `$externalNotes` 指本机仓库外的本轮取证脚本目录；配置为1 worker、相应 baseURL、1440×900 默认视口和失败附件输出。

```powershell
npm run typecheck

npx playwright test environment-windows.spec.ts environment-cookie-window.spec.ts environment.spec.ts native-boundary.spec.ts --config "$externalNotes/playwright-issue34.config.ts" --grep "environment windows|environment Cookie windows|create, edit and reopen|cancel regenerated|invalid and duplicate|storage write failure|injected native bridge"

npx playwright test environment-windows.spec.ts environment-cookie-window.spec.ts environment.spec.ts native-boundary.spec.ts --config "$externalNotes/playwright-issue34-integrated.config.ts" --grep "environment windows|environment Cookie windows|create, edit and reopen|cancel regenerated|invalid and duplicate|storage write failure|compatibility snapshot and Cookie|injected native bridge"

# 最后追加3项既有工作区/模态边界；仍为定向，不是全套
npx playwright test environment-windows.spec.ts environment-cookie-window.spec.ts environment.spec.ts native-boundary.spec.ts modal-boundary.spec.ts --config "$externalNotes/playwright-issue34-integrated.config.ts" --grep "environment windows|environment Cookie windows|create, edit and reopen|cancel regenerated|invalid and duplicate|storage write failure|compatibility snapshot and Cookie|injected native bridge|blocking workspace dialog|loading native blocker|workspace fault blocks Escape"

node "$externalNotes/check-issue34-native.mjs"
```

- 类型检查通过；源码定向 **16/16，20.2秒**，外部挂载预览定向 **17/17，27.5秒**。
- 最后补入3项既有工作区/模态边界，外部挂载合跑 **20/20，28.2秒**：坏桥/加载阻断焦点，以及工作区故障上层Escape不关闭下层Cookie。窗口根节点保留 `tabIndex=-1` 空焦点回退；外部预览保留当前 `storageIssue` inert条件。
- 外部 native 场景 **20/20**：四类忙状态锁/提交身份、Cookie文件往返/失败输入、partial与unknown的仅失败项合并、未知回执原请求/关闭保护、pending重开/取消/迟到、批次当前/历史/尾页/结果焦点、读页失败、继续/取消、clone预览、精确代理映射、非法数量不冻结、表单→代理取消/失败/重试保草稿、移除只读影响、原身份恢复和永久删除保护/原任务恢复。
- 常规 demo 创建/编辑/重开/非法数量/重复名称/存储失败/取消，以及现有 native 模拟创建后原 ID 打开失败重试与无可用内核，均由上述定向文件复验。
- 外部支持 bridge **故意没有 `Fingerprint.CommitRevision`**：四类忙状态保存检查验证安全元数据可编辑、关键字段禁用、提交配置保持 seed/内核/代理/网址/数据身份、准确失败与草稿保留；不虚称这些 fixture 已验证保存成功。常规 native 创建/保存/打开覆盖来自既有 `native-boundary.spec.ts`。
- 初次针对旧几何/未知关闭/文件与结果焦点/回收遮罩的失败已定位并修复。一次忙保存检查原先错误期待未提供方法成功，改为上述真实边界，不补假成功能力。初次克隆取证定位错标签，改用实际“按模板新建”。早期失败附件保留在仓库外，不计最终证据。
- 自审核对所有本票diff、向后兼容props、原Cookie回调/受理前拒绝与去重顺序、精确ID/seed/策略、异步代次/结果合并、背景层次/焦点，以及shared文件未改。文档检查 **57文档/本地链接、12需求、6路由、4内嵌文档通过**；独立公开证据审计通过72张实际尺寸/哈希、图库链接、修改责任和私人路径检查，`git diff --check`通过。

未执行全套 `npm run check`、生产构建/打包、新桌面编译、Go/Wails全套、依赖安装、UIA、真实 Chromium/CDP/Cookie/代理出口/目录恢复。浏览器 demo 与合成 native 结果不互相冒充，也不证明上述真实桌面能力。

## 3 两视口证据与视觉判定

**36状态 × 1440×900 / 1280×800 = 72张实现 PNG**，DPR1、visualScale1、100%、zh-CN、Asia/Shanghai、viewport-only。全部实际尺寸、SHA-256、pageErrors0、非允许请求0、native演示存储访问0已核对。公开材料没有原归档图/代码/字体/图标/资源或私人绝对路径。

- [安全实现画廊](../screenshots/issue34/index.html)；[逐张索引/几何/滚动/哈希](../screenshots/issue34/manifest.json)；[完整状态映射和裁剪](../screenshots/issue34/README.md)。
- 实际读取本票17个原状态×两视口的34张冻结 PNG；私有并排图36组（每组原/实现×两视口）已逐组查看，包括最终重新取证的回收背景/确认/保护状态。私有对照不提交。
- 外部实际取证：`node --experimental-strip-types "$externalNotes/capture-issue34.mjs"`；发布验证：`node "$externalNotes/publish-issue34-evidence.mjs"`。最终回收5状态×两视口重新拍摄，完整72图清单已重新验证，不以失败/旧层次图代替最终图。

| 对象 | 实现实测 | 对照/差异 |
| --- | --- | --- |
| 环境drawer | 1440：x780/y40/660×852；1280：x620/y40/660×752 | 原关键外框一致，bottom8。不是居中大卡或分步页签。 |
| drawer标题/正文/底部 | 标题40；正文y80、高741/641、padding20；footer y821/721、高71 | 原footer70.86，约0.14px舍入；标题/底部不跟正文滚动。 |
| 外挂rail | 宽40，left本体−41，即x739/579 | portal留在实际dialog内部，四区为真实内容滚动跳转。 |
| 表单密度 | 12px/18px；下划线input32；备注/网址outlined | 字段按现有服务裁剪，基础含数量/名称/分组/备注，代理含已有节点/导入，常用含网址/窗口/恢复，指纹含服务内核/固定身份/支持偏好。 |
| 支持内容高度 | 新建默认1450、编辑1370；新建技术展开2445（1440测量） | 原新建3649、编辑3604；不造空白补高度。代理/指纹滚动位置按支持区对应，默认334/709，不冒称原778/1616相等。 |
| 内核下拉 | 宽264 | 原263.91约0.09px舍入；服务fixture两个可用构建，不伪造原三版本/固定96px高度。 |
| Cookie文本/预览 | 1050×551.72 / 1050×609.72；x195/115，y182.14/132.14或153.14/103.14 | 采用冻结proxy text/preview容器；不是已找到原独立单Cookie同名屏。 |
| Cookie文件 | 520×375.83；x460/380，y270.08/220.08 | 原ZIP容器520×375.84；本机实际仅UTF-8 .json/.txt，不声称ZIP。 |
| 批次计划/明细 | 1040×592.25；x200/120，y161.875/111.875 | 冻结绑定选择容器映射；原档案克隆drawer不能替代native冻结计划。 |
| 结果/确认 | 400px；demo移除高181，批次结果264，native移除315，永久删除297，保护结果408（1440测量） | 原短确认约134–152、显示fixture199；保留具体ID、同意和保护/逐项结果，允许高度扩展。不是这些错误状态的逐像素一致声明。 |
| 回收列表 | x210/y50，right10/bottom10；1220×840 / 1060×740；header40，row最小48、gap10 | 页形现有manager；两条合成回收数据不虚造原12项。原商业分组/原因/操作账号改为真实内核/数据/备份/原身份。 |
| 恢复 | 宽440，高255（1440）；x500/420 | 原440×217.86。服务不接收分组参数，展示“沿用原配置分组（只读）”，加准确ID核对而非假选择器。 |

**判定：支持字段、主要容器/滚动/密度已重做并可集成；能力裁剪、本机映射和安全扩展有明确差异。不记“所有窗口1:1通过”，不以点击通过代替视觉对照。**

## 4 裁剪、缺口与 MAIN 收尾

1. 不增加云账号/组织/权限/计费/实例配额/购买代理/云同步/每次启动随机身份/任意UA或GPU硬编辑；不复制商业品牌资源，使用既有Lucide/系统字体。不以视觉裁剪删除已有本机能力。
2. 原创建期Cookie textarea不塞进 `Environment.Create` 或新契约；用户从既有独立导入流程使用Cookie。原Cookie ZIP只映射文件容器，不实现假ZIP。
3. 修订历史、真实保存错误/未知创建/打开失败、Cookie未知/partial/pending、持久批次执行/历史/读页错误、回收保护结果缺同名直接原屏，按冻结环境drawer、1050文本/预览、1040选择、400结果容器映射，仍不算同名原屏已复刻。
4. MAIN 替换实际App的drawer内部为 `EnvironmentEditorWindow`，保留外层inert/唯一trap/草稿与请求控制；demo Cookie/移除替换旧完整modal，而非叠加一层框架。外部挂载预览验证不是已落地App变更。
5. MAIN 用 `EnvironmentConfirmation` 接入未保存提醒/明确force的400px顶层DOM，保留busy/未知关闭保护、匹配previewId的丢弃和原environmentId/sessionId重核。当前仍是现有浏览器 `window.confirm` 安全基线；没有这些DOM状态的完成截图。
6. #35唯一代理导入保留原表单挂载与context，不再造第二份。外部检查已验证现有导入取消/失败/重试保留名称/分组/备注/数量/内核/seed/网址；#35完成后的成功/取消/失败跨页往返仍需 MAIN / #37 终验。
7. 管理器自己拥有一次键盘/锁生命周期；frame不加第二trap。workspace blocker最高优先级，遮挡下层不等于卸载丢原请求。最终shared层次、窄屏、焦点/滚动及全套点击由#37复验。
8. 历史正式 **4/21**、T05–T21 / #6–#22 OPEN、既有21/21及旧候选二进制内容均未改写。未推送、开新PR、关闭issue或自动合并主线/旧PR。
