[继续补核](issue37-continuation-evidence.md) · [原274项清单](issue37-supported-states.md) · [动作账本](issue37-environment-actions.json) · [草稿PR #38](https://github.com/axgiroud312-byte/prism-local-browser/pull/38)

# 环境、Cookie、批次与回收：有限补证

2026-10-07。**本页不是完整1:1或桌面验收。六票保持OPEN、PR保持草稿。** 原113 COVERED / 60 PARTIAL / 101 MISSING、600/32/22/2记账层、历史FAIL/MISSING、完整原指南缺口与正式4/21不回写。

## 已实际取得的340来源记录

拍图/动作源码为 `340a22c1e8489fad2db091a25bb14bb7d2e66984`，79份渲染输入SHA为 `0df5a445b4a16f1984f2944ac7367d1202322b892876f2d31b9e853d9a8708ce`。五位案例作者只提交未执行计划；MAIN冻结、检查并执行。另一个F32案例仍使用同一冻结来源，不借Activity或行独立证明。

| 范围 | 已取得PNG | 单独动作记录 | 像素复核状态 |
| --- | ---: | ---: | --- |
| 环境列表 | 56 | 4 | 14 MAPPED / 16 ADAPTED / 4 MISSING / 22 FAIL |
| 创建、编辑与草稿往返 | 54 | 18 | 16 ADAPTED / 2 MAPPED / 2 MISSING / 34 FAIL |
| Cookie | 20 | 4 | 8 MAPPED / 12 ADAPTED；20图直接同业务原参考仍MISSING |
| 持久批次与demo创建 | 24 | 6 | 4 MAPPED / 12 ADAPTED / 2 MISSING / 6 FAIL |
| 回收与demo移除 | 30 | 4 | 6 MAPPED / 14 ADAPTED / 10 MISSING / 0 FAIL |
| 原drawer上下文的强制动作 | 0 | 2 | 仅动作，不新增强制照片要求 |
| 合计 | 184 | 38 | 70 ADAPTED / 34 MAPPED / 18 MISSING / 62 FAIL |

[184图来源包](../screenshots/issue37-environment-gaps/index.html)已经逐图独立审查并公开到本地开发页；92个状态没有剩余无PNG图位，仍保留4条首轮图片采集失败。私有并排/叠加索引仅追加184项、共1212项，原1028项逐条不变；原图只留本机。

[动作账本](issue37-environment-actions.json)还保留**66份图后回调**：属于同一原图片context的有限后续保存/重试，不是66张结果图、新context或新增通过项。[独立保存合同复核](issue37-environment-contract-review.json)已为 **SCOPED_CONTRACT_CONSISTENT / 0新问题**，38动作、66回调和8个首轮失败与原记录一致；原账本的待审查字段不回写。复核只是有限合成页面与保存证据一致，不是像素、安全全审或真实native通过。

## 原始失败与限定修正

八条原采集失败保留，所有重试只选择原失败项和对应双视口，已成功原图/动作不重跑：

- Cookie图片2条：行过滤器的内部定位器错误带着dialog作用域。MAIN另fork只从同一精确checkbox观察祖先行；Unix秒、过期、Secure None文字及全部输入、回复、行索引、预算、安全断言不改。
- drawer图片2条、动作4条：原作者边界仅接受环境列表pageSize10，误拒实际内核路由pageSize25空筛选第一页查询。MAIN另fork仅在pending-metadata/suspended变体允许精确已支持查询、每context最多3次；Workspace.Read总上限10与副作用预算不改。不能称产品或内核安装失败。

另外两个取证工具源码P2不是业务故障：V1先发布manifest再核验归档工具，以及全局拒绝可能漏存此前逐对错误。另建V2先核验后发布、全局拒绝保留原逐对错误，所有消费者拒绝带全局拒绝artifact的attempt。四份成功V1结果没有全局拒绝、末尾归档核验实际成功，原结果/工具不重写、不重跑或改绑V2。

F32原候选另有一个静态P2：把关闭aria-label当作可见文字。原候选未执行；MAIN另fork只改精确等待值为已打开，独立源码复核限定一致。它不是第九条已跑失败，原报告和原候选保持。回收计划的一处GOAL规则哈希抄写错误也只追加私有勘误，不改原三份作者交付或产品输入。

## 强制动作的准确边界

F32每视口通过真实“编辑”请求等待原preview，再在drawer尚未出现时从原行打开最高force警告，释放原回复，使真实父草稿挂在其下、inert且不抢焦点。先取消，然后由实际App轮询取得原候选缺失、canForce撤销、needsReconcile三种新只读快照；均0次ForceStop。最后仅一次原environmentId/sessionId/UI requestId精确dispatch。

积极路径返回确定的**合成VALIDATION_FAILED拒绝**，不是后台接受或成功终止。缺失候选不等于换成另一个session；按钮取消不等于Escape/双击全部路径；五次preview不等于一个preview贯穿所有轮次。不能据此验收PID/Job归属、真实Go、网络或Windows进程。原未知请求历史、seed、精确内核、数据和代理引用不改。

## 布局续修：新来源单列

两个CSS候选分别独占批次局部样式和全局环境表样式，已独立源码复审并普通合入 `03d0f6836399a9eae83895424c6a13e4d00c0abd`；[源码与集成记录](issue37-environment-css-source-review.json)区分独立源码审查、作者检查和MAIN执行，不把源码一致当成像素验收：

- 批次首列原内容空间21px不足容纳#26，导致编号换行；仅首列60px/nowrap。作者6/6有限检查、0PNG。密集表/继续未执行项的分页只是外层正文顶部位置被裁，自然外层滚底可见，不改窗口高度。
- 环境结果/队列栏的固定高度扣减截行并留空。改为按实际剩余空间容纳完整48px行/10px间隔；合法安全文案不删，普通10/8行保持，带提示状态容量属于适配而非原行数同等。详情以分页上方8px为底界，长正文自然滚动。作者8/8、0PNG；真实Wails/WebView渲染仍未验证。

旧28张FAIL不会消失。MAIN已在新来源冻结79份输入，对有限17状态取得34张新图、0采集失败和14份图后回调；[独立34图复核](../screenshots/issue37-environment-repairs/index.html)为 **34 ADAPTED / 0 FAIL**，确认整行、完整分页和编号单行。直接同业务原图仍MISSING、未查看的正文滚动尾部仍限定；[14份图后回调保存合同](issue37-environment-css-contract-review.json)独立限定一致，0新问题，不新增动作或context。不把旧184图重绑成新HEAD或用作者GREEN代替像素结论。渲染SHA为 `df5b45f48531cbff945dc13eb0eac1d51fc91b8789aea7608c640b2110c46d3e`；只有两个CSS输入变化，其余77份原渲染字节保持，另359份非文档非自有路径的raw/canonical集成守卫一致。私有对照现为1246项；新增34项各绑自己的来源，不改旧1212项。没有重复共享196项检查、生产构建、打包、安装/卸载、Go/Wails/UIA或真实内核/网络/目录恢复探针。

drawer的34张FAIL涉及三类实际容量/浮层问题（内核状态截字、任务盖页脚、长提示盖标题）及两组技术详情取景未滚到目标；不是34个不同缺陷。独立实现者自己的 `10b0820` 和合入03后的候选 `b01d17d` 已[独立源码复审并普通合入](issue37-environment-drawer-source-review.json)MAIN `8ca2e79`；相对03仅三个组件/局部CSS文件变化。作者10/10定向、两项noEmit、0PNG；MAIN另冻结新79输入并完成两项noEmit，不用作者结果代替独立复核。首轮新图取证在启动前被工具文件名排序误拒，0图、0动作，原全局拒绝保留且禁止准入；只在新工具fork修正变化路径集合的排序比较，其余76份字节、完整来源和安全守卫不放宽。

MAIN新取得17状态34图、0案例采集失败，渲染SHA `a83d927c931dd81b76656f98d6616aafe5f8e8df9b168d68953bab9d193537f2`；[独立逐图复核](../screenshots/issue37-drawer-repairs/index.html)为 **34 ADAPTED / 0 FAIL**：26个可见已选内核标签完整、4图技术来源块可读、2图任务与6图实际长toast不盖标题/页脚。直接同业务原图仍MISSING，未同时可见的技术行或不兼容按钮不借其他尺寸充当照片证据。[34图/16同context回调保存合同](issue37-environment-drawer-contract-review.json)独立限定一致、0问题，15份实际归档工具身份匹配；[原始取证事实](issue37-environment-drawer-checks.json)不把回调当新动作或PNG。旧34张drawer及28张列表/批次FAIL原封保留。技术详情仅自然wheel到真实目标，不为截图改变窗口几何。

导航现14包906份记录，各包分源；私有对照仅追加新34项、共1280项，原1246项不变。仅普通清理已完成三个自有worktree并停5196/5197/5199；注册已解除，八个占用空目录保留未强删，主依赖目录、5173/5190/5198和无关V1工作区/其他会话进程未触碰。

## 原47个有限定义：37已补证，10仍有限定

[环境归一化层](issue37-supplementary-environment.json)只使用340来源的184图、38动作、66回调，原47个定义逐字沿用：35个有限缺口已补证、12个保留剩余子句。它不修改原274项状态，也不合并后续修复来源。

[F13/F15单独续层](issue37-supplementary-environment-repairs.json)只用8ca的4张技术来源视图及完成的独立合同/实际看图结论，另补2个原有限定义；F15历史只沿用340旧图已接受的可见部分，不声称新图展开了历史。派生为 **37/47有限定义已补证、10个仍有未证明/不支持/来源限制子句，整项PASS新增0**。旧35/47、原184/38/66、8条失败、62张FAIL和所有历史层均不变。

| 原定义 | 尚未证明或不支持的部分 |
| --- | --- |
| E18 | native没有skipped分配分支，不能构造五类同时结果 |
| F07 | 独立idle-create底部内容未证明；重复fingerprint图不能借作底部证明 |
| F11 | demo忙碌/旧设备/失败重试不能借native观察补齐 |
| F26 | demo部分创建的ID保留不证明native部分完成分支 |
| F32 | 换会话、Escape及双确认路径未观察；合成拒绝dispatch不是真实终止 |
| B08 | ID查询空/错误结果未在准入证明中各自序列化；密集底部续修另绑03，不重绑340 |
| R09 | 历史实际为Recycle.ReadPage，未证明定义里的Operation.Read |
| P07 | demo没有256 UTF-8字节提交前限制，native合成拒绝不替代 |
| M22 | 未知discard的核实/恢复入口与清理结果仍未确认 |
| H19 | 稳定首次loading图未取得，完整原指南仍缺 |

这些是固定原定义的剩余子句，不新增功能、强制窗口、每回调照片或真实探针来凑分母。直接参考、有限行为和真实native验收是不同维度。

## 最终来源、文本与导航检查

[最终有界审计](issue37-final-evidence.json)匹配906张新增图及原346＋24张历史图，核对各包自己的源码/冻结输入、PR31祖先、后台/服务/领域/依赖/构建与CI保护对象；647原来源/80图及145设置来源/6候选只读哈希不变。15份静态报告1887个本地链接核对，三个新增入口实际自然点击、白底深字可读；静态CSP阻断Vite注入的已知错误保留，未放宽策略。

原公开审计误把README既有项目内验证目录当成私人路径，旧拒绝保留。新fork经独立源码预检，只在该文件确认恰好一个token、所在整行与83完全相同后分类，其他内容仍用原守卫，真实私人/商业路径不获许可。哈希、链接或文本筛查都不是全量安全、许可、像素或native认证；发布后的纯文本闭合检查不重跑整份源码/PNG审计。

## 保留的来源与能力限制

固定原47个剩余定义仍是本层的来源，不改成47张必须补拍图片或新功能。P07 demo256 UTF-8字节拒绝、M22未知discard恢复入口、H19稳定首次loading PNG/完整原指南限制单列。环境各族也保留未证明子句：真实native parser/持久化/调度/桥或进程保护、不可自然获得的同时状态、全计划跨页同意、未知批次重挂、回收跨页restore/purge、demo异步/多构建及native多项创建等不能用有限回复补造通过。

本轮不增加native新能力、不重写后台、不借演示证明真实服务；代理失败绝不直连、已创建但打开失败只重试原ID、普通编辑不重生成seed、取消不提交、批量范围与失败恢复继续沿用原应用服务。旧exe不含这批UI；商业代码/素材/原图、真实凭据/Cookie/profile/用户数据及私人路径不入库。
