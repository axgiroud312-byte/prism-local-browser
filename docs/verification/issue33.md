[验收索引](../ACCEPTANCE.md) · [唯一视觉参考](../UI_REFERENCE.md) · [公共组件与责任](../UI_CONTRACT.md)

# #33 参考 shell / 环境表交付记录

日期：2026-10-06。分支 `codex/issue33-reference-shell`，代码前提为 PR #31 集成基线 `e170099bc25aa15ecb9a8456d782394a2139ec20`。关联 [#33](https://github.com/axgiroud312-byte/prism-local-browser/issues/33)；交付为**可操作前端与既有服务接入，待集成终验**，不是所有页面/桌面能力完成。

实现/定向用例提交：`72cf08a`（`feat(ui): recreate reference shell and environment list for #33`）。截图与本页文档在后续独立证据提交中，不改变受测 UI 源码。

后续集成：本票已合入 `9e256a7`；#37修正独立审查发现的表头字重、分组菜单/标题位置，最终App证据和共享检查见 [整组记录](issue37.md)。下文保留本票原交付时的实际结果，不把旧图重标成最终截图。

## 已交付行为

- 替换旧侧栏/工作区卡片/统计卡/大标题及旧工具栏，不是只换主色。公共 shell、两个工具区、固定密集列、40px 表头、48px 行/10px 间隔、10 条分页与表体独立滚动落到可操作页面。
- 普通/空/无结果/选中/运行/失败、行菜单/创建菜单/分组浮层/高级搜索/分组列表与修改均可检查。原六个导航目标保留，增加派生 `groups` 页，不加无服务账号/配额按钮。
- demo/native 沿用同一个应用服务注入，未改契约、适配层、SQLite、RPC 或格式。精确跨页选择、普通身份、保存策略启动、逐项持久化失败/重试、创建后原 ID 打开及坏桥阻断保留。
- 分组修改冻结准确 ID；native 逐项结果只重试失败 ID。派生分组不制造独立空实体；native 分组页只对已读当前页成员显示计数/修改。
- reconcile、无 PID 资源清理、待保存、force 确认、排队取消、网络细节及低频 clone/assign/backup/history/recycle 仍有入口。密集行的详情经 portal 浮层呈现，不把占用/未知状态写成成功。

## 定向检查（与视觉分开）

本轮使用已存在依赖和 Playwright；`tdd` skill 调用失败（Unable to load skill），采用可观察 RED → 实现 → GREEN。最初几何用例在旧 `.stats-grid/.workspace/.breadcrumb` 数量 3 上正确失败；没有放宽旧业务断言。

实际 Vite 为 `http://127.0.0.1:5193`。`$proofConfig` 是仓库外的本票配置：testDir 指向 `tests/ui`，1 worker、baseURL5193、默认1440×900、失败截图/trace；没有另启默认5183或重构 CI。

```powershell
# 新增7项（demo4 + 合成bridge3）
npx playwright test reference-shell.spec.ts native-reference-shell.spec.ts --config $proofConfig

# 最终一起复验新增7项 + 既有受影响7项；不是全套 npm run check
npx playwright test reference-shell.spec.ts native-reference-shell.spec.ts environment.spec.ts native-boundary.spec.ts --config $proofConfig --grep "reference shell|selection survives|supported filter|derived group|native 10-row|native group partial|native busy rows|group and search|demo persistence failure|all six pages|injected native bridge|broken desktop bridge"

npm run typecheck
```

- 新增 **7/7，9.9秒**；既有定向 **7/7，18.8秒**；最终合跑 **14/14，27.5秒**。类型检查通过。
- `npm run check:docs` 通过：55 文档/本地链接、12 需求、原6路由、4内嵌文档；派生分组路由另在新页面用例检查。另行核对公开表恰好映射全部40个 captured ID，36张合成证据尺寸/SHA及无私人绝对路径通过；`git diff --check` 通过。
- 覆盖两视口几何/滚动分页、搜索/无结果恢复/菜单焦点、跨页精确 ID/分组/启停、seed/内核/代理/Cookie 不变、native 正确修订和 direct/proxy 请求、混合失败与仅失败项重试、reconcile/cleanup/pending/force/FIFO、存储失败恢复、旧六页/390px窄窗口、创建后打开失败只重试原 ID、无内核引导和坏桥不落 demo。
- 中途合成 fixture 错把配置投影当带 ID；该 run 为 6通过/1失败。修正 fixture 为用 `previewId` 解析准确目标，重跑该项通过后完成上述最终14/14，未修改真实契约或放宽目标断言。第二个演示代理的空 country 不合已有 schema，改为 US；不修改 schema。
- 未执行本轮全套 `npm run check`、生产构建/打包、新桌面编译、Go/Wails 全套、安装/UIA、真实内核/网络/目录操作。#37 仍负责整轮共享检查和独立复核。

## 合成证据

最终取证为 **17 个状态 × 两视口 = 34 张 PNG，另 2 张表体滚到底 PNG**；page error 全部 0。每张均 DPR1、100%、viewport-only。原图留在外部只读参考；公开图均为独立 Prism fixture 或模拟 native bridge，**“本机桌面”模式文字不使合成桥成为真实桌面证据**。

- [测量/状态索引](../screenshots/issue33/screenshots.json)：17 状态、页面错误、x/y/宽高/滚动高度；不是原商业响应或私有参考索引。
- 环境页：[1440×900](../screenshots/issue33/1440x900/environment-list.png) / [1280×800](../screenshots/issue33/1280x800/environment-list.png)；[1280表体底部](../screenshots/issue33/1280x800/environment-list-bottom.png)。
- 分组修改：[1440×900](../screenshots/issue33/1440x900/group-edit.png) / [1280×800](../screenshots/issue33/1280x800/group-edit.png)；设置新名称：[1440](../screenshots/issue33/1440x900/group-add.png) / [1280](../screenshots/issue33/1280x800/group-add.png)。
- 其他完整状态各在相同视口目录：empty/no-results/running/selected/row-menu/create-menu/advanced-search/group-popup/group-list/failure；native-busy/details/row-menu/group-failure 为模拟桥。截图名字与索引 stable ID 对应。

外部 `capture-issue33.mjs` 的实际取证命令为 `node --experimental-strip-types <外部脚本>`；脚本加载本票 fixtures、独立 context/viewport/locale/timezone 后截图并测量，不执行归档。第一次分组列表取证未等待 hash 导航完成，发现后加可见状态等待，最终重拍；native 截图也等待分页选择可用，旧过早截图不作为最终证据。

## 视觉核对结论

**本票 shell、环境表主体和分组 dialog 的关键几何对齐；能力/安全适配后可交付公共基础。不是未经裁剪的商业像素级全复刻，也不是 #34–#37 已通过。** 已查看实际 PNG，对照冻结图/测量分别检查普通、空、无结果、选中、已打开、菜单、分组/高级筛选、分组列表与两种分组 dialog；另核对模拟 native 忙/失败/详情不遮挡必要操作。

| 对象 | 实现实测（1440 / 1280） | 对照结论 |
| --- | --- | --- |
| sidebar/topbar/nav | 宽200、高42、环境导航y144、170×40 | 与冻结主几何一致；品牌/图标自行实现，裁剪首页只留44px空槽 |
| 创建 split / toolbar | split134；搜索区y50、高52.86，列表工具区y112.86、高50 | 已修窄创建按钮和旧框架偏差 |
| 表头 | x220/y162.859/40，宽1200 / 1040 | 与参考一致 |
| 行/滚动 | 第一行y212.859、48px、pitch58；body582 / 482、scrollHeight590 | 已去掉尾行额外10px；独立滚动，后两行可达 |
| 普通分页 | y794.859 / 694.859、高30 | 滚动前后位置不变；裁剪无服务底部工具和跳页器，保留真实页码/上一下一 |
| 空/无结果 | body68、分页y280.859 | 保留原框架密度；增加新建/清除入口是可恢复适配 |
| 分组浮层 | x359/y152.359、260×148 | 与参考 x360/y152.86 相差1 / 0.5px锚点边框；无明显结构差异，非逐像素相等声明 |
| 高级搜索 | x988 / 828、y89.688、432×335 | 位置/宽度对齐；原526.86高度裁为335，只留3个服务支持条件 |
| 行菜单 | x903/y247.859、宽176，demo高160/native高224 | 无裁切；原256高度因云功能裁剪/本机入口适配变化 |
| 分组 dialog | x470/y288.062 或 x390/y238.062；500×339.859 | 与原容器一致；删排序/归属，填准确范围与身份说明 |
| 分组页 | category y50、面板y89、表头y141、行48/pitch58 | 无独立实体列/操作被裁剪，不冒称原完整组 CRUD |

### 已说明的差异与剩余事实

- 顶栏换为本机工作区/模式/记录/说明；无云账号、语言/官网营销、计费配额。原侧栏商业背景/品牌不复制；system font/Lucide 不是原字体/图标资源。
- 标签位使用内核/状态；直连不捏造外部 IP。合成概念与原一致（12环境、两组、编号/名称/时间、6成员/组），按棱镜 schema 独立表达；地址/内核等服务字段不为截图复制原商业投影。
- 选中工具条提供真实批量打开/关闭/调整分组，移除及本机低频操作进“更多”。不制造原云按钮或假列排序。已打开文字可直接关闭，title/可访问名称说明用途；不是原查看窗口动作。
- 保存失败与本机 FIFO/pending/清理等没有直接原执行画面。保留准确反馈条/逐项结果，会相应压缩表体或下移表头；不能为了普通图几何抹掉保护。force 继续使用现有明确确认，不宣称其已换成新商业确认窗。
- `group-add` 是“给所选环境新标签”的500px容器映射，背景为环境页，不是原独立空组创建；`group-delete-confirm` 明确不可达，不造图。native 分组页不是全量组管理，范围文字必须保留。
- 原指南/本机内核/备份恢复/迁移/诊断/真实 pending 缺同名直接参考，见 [映射/缺口](../UI_REFERENCE.md#4-所有已有本机页面弹窗的容器映射)。本轮并未修这些页的全部内部布局；交由后续票，而非宣称全产品1:1。
- 交付前已运行 `git merge --no-edit codex/issue32-37-ui-integration`，当时 tip 仍为 `e170099`，结果 Already up to date；后续由合并负责人处理，不自行推送/开PR/合并旧PR/关闭票。旧候选二进制未更新，历史21/21与正式 **4/21、T05–T21 / #6–#22 OPEN** 均不改写。
