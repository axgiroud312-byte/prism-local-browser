[唯一视觉参考](../UI_REFERENCE.md) · [公共责任](../UI_CONTRACT.md) · [使用指南](../USER_GUIDE.md) · [合成截图索引](../screenshots/issue36/README.md)

# #36 备份、恢复、记录、帮助与诊断组件交付

日期：2026-10-06。关联 [#36](https://github.com/axgiroud312-byte/prism-local-browser/issues/36)，分支 `codex/issue36-local-pages`。依赖 #33 的冻结 shell/公共控件已可用；基线含 `e170099` 与 `9e256a7`。本票只改所属组件、scoped CSS、定向测试、指南与证据，未自行修改 App、入口、全局样式或共享追踪表。

**结论：所属模块可交付集成；不是 #36 已关闭、生产 App 全部接入或全产品 1:1 通过。** demo/记录/帮助页面的 App 替换、旧标题/restore overlay 移除与整体终验由 MAIN/#37 负责。备份/恢复/诊断原有 native 挂载已使用本票组件；另有实际 App 的工作区故障优先级用例。其余组件证据来自明确标记的独立 wiring harness，不能冒充主入口已验收。

## 实现与保留边界

- `BackupManagementPage` 分流既有 native 完整包与 demo JSON；`DemoBackupPage` 使用原创建/恢复/下载回调，不另造 adapter。写入成功才退出，失败保留原状态与窗口；兼容格式、示例 Cookie、代理密码排除与运行中恢复阻断不变。
- native 列表、创建、系统文件选择、导入、1040px只读预检、400px确认/结果及恢复历史连接原 application/controller。选择冻结准确 ID（含离页 ID）；全量用 `scope: all, environmentIds: []`，不是当前页。普通操作不重生成身份。
- 预检/分页/丢弃不刷新工作区、不提交恢复、不替换目录。确认后仍保留覆盖/凭据/正常停止授权；停止失败不强杀。未知提交重发原请求，保留 requestId、源摘要与输出授权；没有任务 ID 也能直接进入原请求核实窗口。
- 发布需匹配报告、完整终态与非待保存状态；手动读取导出终态后刷新服务列表。进行中、失败、回滚、维护保护与未知均不冒称成功。取消和收尾只作用明确原 operationId，不自动重开已关闭会话。
- `NativeRestoreExecution` 原 application/workspace/preview/onConsumed/onLockChange props 与 controller 保持兼容；仅新增可选 `confirmationRequested`，#35 仍可用原签名和默认确认入口。
- `ActivityPage` 有准确对象/结果、过滤、分页与详情。只有 environmentId **和** sessionId 都匹配当前会话才给动作；沿原 `canForce`/`needsReconcile` 标志，不把正常控制通道失效误当成不能强制结束。实际执行与确认仍归 MAIN 回调，blockedIds 禁用动作；native 不提供原始日志导出。
- `NativeDiagnostics` 继续使用 adapter-owned `DiagnosticsClient`。仅展示明确白名单投影，不渲染任意 JSON、名称、私有路径、seed、凭据或 Cookie。预览、取消、匹配保存回执、离页返回、原请求核实与结束核实均保留；结束核实仍明确旧文件可能已发布。
- `HelpPage` 保留四份 raw 文档、六个原路由、Markdown 下载、内部文档跳转与正文滚动重置；指南说明新入口和证据边界，12 项需求仍保留。
- `LocalPageWindow` 复用视觉 frame，拥有 portal、最上层 Escape/Tab/CtrlK、inert 引用计数、正文滚动、固定 footer 与焦点交接。不会 inert 整个 React 根；新出现的高优先级工作区 blocker 接管键盘。从 blocker 新开的诊断同时取得更高绘制层级；不活动/被 inert 的下层遮罩透明，只保留一层有效遮罩。诊断结束回到 blocker，故障解除仍保护下层窗口，最后关闭才恢复背景。

源码提交：`26106c2`（模块/定向用例）、`10fde5a`（实际 App blocker 焦点回归）、`1bb5dd4`（绘制层级/单一遮罩/30px记录分页与两视口回归）；首版证据 `53df6ad`。本分支已吸收最新集成 tip `c6371eb`（含#35模块，merge `48fbd0d`），随后重新运行下述检查并重拍78图；不是MAIN已合入本票的声明。

## 定向检查（不等于桌面验证）

使用已有依赖、独立 Vite 5196、1 worker。测试先拦截非本 origin 请求，并关闭 HMR websocket，避免开发服务器排队的 full-reload 重建测试 adapter。没有安装工具或另建测试体系。

```powershell
npx playwright test local-pages.spec.ts --config $proofConfig
npx playwright test environment.spec.ts modal-boundary.spec.ts --grep 'compatibility snapshot|all six pages|blocking workspace|loading native blocker|lower native Cookie' --config $proofConfig
npm run typecheck
npm run check:docs
git diff --check
```

`$proofConfig` 是仓库外的本票配置，指向 `tests/ui`、5196 与本票输出。最终源码检查：

| 检查 | 实际结果 |
| --- | --- |
| 本票完整定向文件（两视口 + 窄窗口 + 实际 App 双视口 blocker） | **57/57，57.9秒** |
| 既有 App 快照/Cookie写失败、六页导航、三项工作区浮层边界 | **5/5，10.7秒** |
| `npm run typecheck` | 通过 |
| `npm run check:docs` | 通过：61文档/本地链接、12需求、6路由、4内嵌文档 |
| `git diff --check` | 通过（仅现有LF/CRLF提示，无空白错误） |

此前 **55/55，44.6秒** 与补绘制断言前的 **56/56，46.0秒** 是中间结果，不能替代新增视觉层级回归后的57项结果。两组最终定向范围分别运行，不称为整组 `npm run check`。

覆盖：两视口与820×600固定 footer；格式拒绝/兼容导入/导出；保存失败和重试不改旧数据；只读预检与丢弃；精确离页 ID、all 范围、原未知请求（含无 ID）；匹配报告发布条件、待保存不得成功；恢复进度/取消/回滚/保护收尾/未知/冲突/缺构建/完整提交报告；诊断白名单、最小报告、取消/匹配回执/原请求/结束核实；旧/当前/待核对/blocked 会话；四文档/下载/导航/滚动；实际 App 故障盖过恢复窗口、嵌套诊断的绘制层级/鼠标命中/单层遮罩/实际返回点击，与故障解除后的焦点/inert恢复。

### 实际失败与修复记录

1. 第一轮29项因120秒外部命令超时而中断，不能记作完整通过；后续29项完成为27通过/2焦点失败。修正替换窗口与父子同时退出时的原焦点交接。
2. trace 确认一次导航重启来自 Vite 发出的 `full-reload`（LocalPageUi 源码变更），非业务导航；本票 test/capture 不连接 HMR。未放宽原数据断言。
3. 新增51项为49通过/2失败：手动读到导出终态后没有刷新已发布列表。补服务 refresh，再核对真实列表可见。
4. 视觉检查发现全局 `table:not(...)` 的 specificity 压过 scoped 样式；修正本票选择器，补40px表头/48px行/行间距和30px分页实测断言。
5. 一轮53项的窄窗口失败是 harness 改为图标导航后旧文字 locator 不可达；改点实际可见顶栏记录按钮并等待确认挂载/关闭，单项通过。
6. 自查保留 force 的既有 `canForce` 语义，修正未知无 ID 时锁在创建页的恢复入口，区分 pending publication；没有更改后台或格式。
7. 实际 App 故障用例正确先失败：全 React 根 inert 让新 blocker 不可达，下层窗口又抢焦点。改为区域 inert、按真实层级暂停较低 trap，并保留 React 更新后的所有权；扩展故障解除后的恢复断言并通过。
8. 最后截图复核发现“键盘焦点通过”仍可能被更高但 inert 的故障遮罩盖住：诊断层124、blocker层160，且遮罩叠黑。新断言先在下层遮罩颜色失败，随后相对绘制层级断言复现124不大于160。改为新开窗口高于当时有效层、临时透明下层遮罩；双视口核对绘制/中心命中、鼠标返回与恢复，57项最终运行通过。取证脚本另拒绝两层有效遮罩，旧截图已全部重拍。

`implement` 流程已使用；`tdd` 与 `code-review` skill 均返回 unavailable。替代为可观察失败→修复→回归和源码自查；不把自查写成独立审查。#37 仍需独立审查与整轮共享检查。

未运行全套 `npm run check`、构建/打包、新桌面编译、Go/Wails、安装/UIA，或真实内核/网络/目录/备份/恢复实验；历史 **4/21** 与旧候选 exe 内容未变。

## 视觉来源、测量与差异

冻结 `202609160208`。已实际查看10种原状态各两个尺寸：activity-list、proxy-list、group-add、group-delete-confirm、proxy-add、proxy-edit、proxy-bound、proxy-import-file、batch-progress-fixture、settings-page。只读保存 PNG/测量，不运行商业前端，不复制原代码/资产/原图进 Git。

| 本票对象 | 参考关系 | 实现实测（1440 / 1280）与适配 |
| --- | --- | --- |
| 操作记录列表 | activity-list 直接布局，字段适配 | sidebar200/topbar42；toolbar y93/h52；表头x220/y165/h40、宽1200/1040；首行y215/h48；与冻结y164.58/214.58相差约0.42px。分页y283/h30。 |
| 备份列表 | proxy-list / activity-list 容器映射 | 页签y50；白面板y89；表头y141/h40、行48/间隔10；不沿用旧卡片。 |
| 创建 | group-add 500px短表单映射 | 500×340，x470/y288 或x390/y238；与冻结500×339.86差0.14px。 |
| 文件导入 | proxy-import-file 映射 | 500×359，x470/y278.5 或x390/y228.5；保留本机选择器、不上传。 |
| 恢复预检 | proxy-bound 影响列表映射 | 1040×592，x200/y162 或x120/y112；标题40、正文独立滚动、footer58；冻结592.25的约0.25px取整差。 |
| 恢复确认/结果 | group-delete-confirm / batch-progress-fixture 映射 | 宽400；确认262px、结果按真实状态内容伸缩。覆盖/凭据/停止与保护说明使其不可能等高于原短删除确认151.86px。 |
| 活动详情/诊断预览 | proxy-edit 620px详情映射 | 活动详情620×440、诊断620×580；删原代理表单字段，填安全投影；诊断正文482px可滚动，footer固定58px。不是找到同名原详情。 |
| 四份指南 | shell/标题/正文滚动映射 | 左索引230px；正文x450/y151、970×730 /810×630；原设置页有客户端覆盖层，不能作为完整指南1:1证据。 |
| 工作区故障/嵌套诊断 | 继承公共500px blocker / 620px诊断容器映射 | 额外实际 App 边界取证；blocker层160，子诊断层162，单层有效遮罩。仍有MAIN拥有的旧备份页标题，明确不是最终页面接入截图。 |

能力裁剪：操作记录去掉无本机服务的登录/员工账号/云操作用户等过滤，用准确对象、结果与详情替换原列；固定10条/页、上一/下一/真实过滤，不做无回调跳页器。危险窗口增加服务保护说明，不缩掉内容伪装为原短确认。品牌、字体/图标使用本项目独立实现。日期统一 `YYYY-MM-DD HH:mm:ss`。

最终合成取证为 **39状态 × 2视口 = 78张PNG**，页面错误0。通过实际组件点击到达；fixture 的任务/报告/失败均为作者定义的内存情形，不能证明实际文件发布或原桌面执行。已通过10张联系表检查全部缩略图，并读取代表性原尺寸及四张实际App边界图；吸收c6371eb后重新取证的78图与已检查版本逐字节相同。逐图尺寸/DPR1/scale1/唯一索引/SHA-256核对通过，弹窗仅一层有效遮罩。截图与测量见[完整索引](../screenshots/issue36/screenshots.json)；只有明确 `guide-kernel-bottom` 使用正文底部滚动，其余从对应支持内容顶部取证。

已说明差异：本机专用页无同名直接参考；指南缺完整原画面；确认/结果的高度随安全语义变化；App三页最终接入与共同浮层集成仍待MAIN/#37。**不宣称未经裁剪的商业逐像素全复刻或本票终验已完成。**
