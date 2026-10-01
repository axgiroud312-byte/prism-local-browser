[项目首页](../README.md) · [产品需求](PRD.md) · [开发方案](DEVELOPMENT.md) · [内核合同](KERNEL.md) · [验收记录](ACCEPTANCE.md)

# 需求到实现的追踪表

版本：1.0 · 更新日期：2026-10-01

本表将 PRD 的 12 项需求关联到当前前端入口和建议验收。**出现源码入口只代表存在相应原型逻辑，不代表完整桌面能力已经实现，也不代表测试已经通过。** 实际执行记录统一放在 [ACCEPTANCE.md](ACCEPTANCE.md)。

路由是运行应用后的 hash 路由。源码链接指向文件，函数名用于定位；前端持续修改时不依赖易失效的固定行号。领域逻辑自动测试入口为 [`tests/domain.test.ts`](../tests/domain.test.ts)，页面流程仍需真实浏览器操作检查。

## 12 项需求映射

| 需求 ID                       | 原型路由与交互入口                                        | 源码定位                                                                                                                                                       | 建议验收；不表示已执行                                                                                                                                     |
| ----------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ENV-001 环境列表与批量操作    | `/#/environments`；搜索、筛选、选择、分页、批量按钮       | [`App.tsx`](../src/App.tsx)：`App` 内 `visible`、`pageItems`、`selected`，`saveEnvironment`、`launch`、`stop`、`removeEnvironments`                            | 名称/编号/备注搜索及空结果可恢复；筛选和翻页不扩大已选 ID；批量任务只影响所选项，失败单独显示；取消保留已完成项；无产品数量配额。                          |
| ENV-002 创建与编辑环境        | `/#/environments`；新建/编辑右抽屉                        | [`App.tsx`](../src/App.tsx)：`openCreate`、`openEdit`、`patchDraft`、`saveEnvironment`；[`domain.ts`](../src/domain.ts)：`validateEnvironment`                 | 名称不能为空且不重名；非法网址/窗口尺寸不保存；取消不写入草稿；模板新建生成新 ID 和 seed、不复制 Cookie；代理选择明确；运行中的关键配置受保护。            |
| ENV-003 启动停止与失败保护    | `/#/environments`；单个与批量启停；`/#/activity` 查看原因 | [`App.tsx`](../src/App.tsx)：`launch`、`stop`、`event`；[`domain.ts`](../src/domain.ts)：`launchError`                                                         | 未检查/失败代理及缺失内核阻止模拟启动；无静默直连；快速重复点击不重复执行；关闭或取消后没有遗留启动任务；状态与日志一致且持续标明模拟。                    |
| FP-001 固定设备档案           | `/#/environments`；抽屉“指纹”页签                         | [`App.tsx`](../src/App.tsx)：`generateProfile`、`previewProfileRestore`、`saveEnvironment`；[`档案服务`](../internal/workspace/fingerprints.go)；[`DemoAdapter`](../src/application/demo-adapter.ts) | 新建 seed 在支持范围内且检查冲突；关闭/重开、修改代理及恢复不自动换 seed；生成/回滚只改预览，明确提交后生效；数据引用不随重生成清空。 |
| FP-002 指纹能力分层           | `/#/environments` 的指纹说明；`/#/kernels` 的能力说明     | [`App.tsx`](../src/App.tsx)：指纹页签、内核能力展示及 `kernel` 对话框；[`domain.ts`](../src/domain.ts)：`Environment`、`Kernel`                                | 区分可配置、内核按 seed 生成和待核对项；原型不展示伪造的真实硬件读值；窗口大小与屏幕指纹区分；更换版本不能宣称已有真实验证。                               |
| PRX-001 代理导入检测与分配    | `/#/proxies`；导入/编辑/检查；环境抽屉代理绑定            | [`domain.ts`](../src/domain.ts)：`parseProxyText`、`launchError`；[`App.tsx`](../src/App.tsx)：`openProxyImport`、`checkProxy`、代理编辑与绑定控件             | HTTP/HTTPS/SOCKS5、转义凭据和 IPv6 正确解析；错误行有行号；密码不进入列表与日志；被引用代理不能直接删除；检查清楚标为模拟，编辑后重新检查。                |
| CK-001 Cookie 导入            | `/#/environments`；环境行“导入 Cookie”                    | [`App.tsx`](../src/App.tsx)：`openCookies`、Cookie 预览及提交；[`domain.ts`](../src/domain.ts)：`parseCookies`、`mergeCookies`                                 | JSON/Netscape 格式可预览；错误数据不提交；空 value、会话属性、到期字段及分区信息保留；按完整身份键合并；写入只影响选定示例环境，预览和日志隐藏值。         |
| CORE-001 固定内核版本         | `/#/kernels`；环境抽屉内核选择                            | [`domain.ts`](../src/domain.ts)：`Kernel`、`seedState`、`launchError`；[`App.tsx`](../src/App.tsx)：内核页、`kernel` 对话框、`saveEnvironment`                 | 环境保存具体内核 ID；候选/不可用内核阻断模拟启动；现有环境不跟随其他选择自动升级；页面说明未安装、未校验或运行真实内核。                                   |
| BKP-001 快照备份与恢复        | `/#/backups`；创建、导出、导入、确认恢复                  | [`App.tsx`](../src/App.tsx)：`newBackup`、`confirmRestore`、`download`；[`domain.ts`](../src/domain.ts)：`createSnapshot`、`parseSnapshot`、`restoreSnapshot`  | 导出排除代理密码；坏格式、重复 ID、断引用、非法字段被拒绝；运行环境未停止时不恢复；有效恢复保留 seed、重置代理检查、保留原型历史记录；无真实目录备份承诺。 |
| DATA-001 数据隔离与删除       | `/#/environments`；单个与批量移除；Cookie 目标环境        | [`domain.ts`](../src/domain.ts)：`Environment.id`、`mergeCookies`；[`App.tsx`](../src/App.tsx)：`removeEnvironments`、Cookie 提交                              | A 环境修改不改变 B 的记录或 Cookie；运行中的环境不能移除；移除前展示数量与数据含义；原型不操作文件，不声称已验证 Chromium 目录隔离或回收区。               |
| UX-001 可访问性与本地持久演示 | 六个页面；抽屉、弹窗、反馈及存储提示                      | [`App.tsx`](../src/App.tsx)：`App` 的持久化/键盘/焦点 effects、`notify`、表单校验与空状态；[`styles.css`](../src/styles.css)                                   | 键盘打开/关闭、焦点约束和返回位置可用；状态有文字；刷新后演示数据保留；坏存储与多标签页冲突不静默覆盖数据；窄窗口仍能触达关键操作。                        |
| DOC-001 文档与页面可追踪      | `/#/guide`；文档页签及下载入口                            | [`App.tsx`](../src/App.tsx)：`docTab`、指南页 Markdown 展示、`download`；[`PRD.md`](PRD.md)、[`DEVELOPMENT.md`](DEVELOPMENT.md)、[`KERNEL.md`](KERNEL.md)      | 页面可读三份主文档并下载；12 个 ID 在 PRD 和本表一致；相对链接有效；当前原型与后续桌面目标明确分开；实际检查结果可追溯到验收记录。                         |

## 已发布的桌面开发任务

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
- [`真实保存档案回读`](../internal/workspace/fingerprints_real_test.go)与[实际样本](verification/T05-saved-profile-observations.json)证明已保存→重生成→回滚→服务重开仍复用原输入，合成文件/引用保持、诊断进程正常退出。样本是2026-09-30评审前工作树结果，不能冒充最终版本全量通过；真实Cookie与正常目录会话未验收。2026-10-01最终复验因C盘不足失败，随后用户要求先开发、停止CI/完整回归；新增UI仍未操作，[逐票记录](verification/T05.md)。

### T06 正常会话与独立目录（本地实现已提交，未运行验收）

- ENV-003、CORE-001：[`运行服务`](../internal/workspace/runtime.go)读取固定档案/实际构建与数据引用，持久受理/去重，串行昂贵启动。客户端仅环境ID/requestId和明确direct；已绑定代理阻断，不接受路径/参数/PID覆盖。进程/控制通道就绪才running，真实状态经Wails/App刷新；正常停止只作用于本次自有Job，超时/清理未确认保留busy。
- DATA-001：[`长期内核会话`](../internal/kernel/runtime_windows.go)、[`实际目录锁`](../internal/kernel/profile_lock_windows.go)持有实际目录与不可变文件pins，拒绝非法/链接路径及硬链接锁文件，Job全树退出才释放。不删除正常profile；停止/重开保持保存seed与数据引用，当前未以运行证据核验此结论。
- 新增[`服务回归`](../internal/workspace/runtime_test.go)、[`目录用例`](../internal/kernel/profile_lock_windows_test.go)、adapter回归与[`A/B真实Cookie/LocalStorage/IndexedDB及重开`](../internal/workspace/runtime_real_test.go)仅编写，尚未运行。该真实用例需要独立开关，会打开可见窗口，当前不自动执行。用户先开发的规则与[剩余验收](verification/T06.md)保留，不能据源码入口声称#7已完成。

### T07 异常监督与重开核对（已实现，未运行验收）

- ENV-003、DATA-001：[`监督器`](../internal/workspace/runtime_supervisor.go)区分崩溃/断管/退出未确认，只在普通停止失败后允许指定当前Job结束；[`持久恢复`](../internal/workspace/runtime_persistence.go)与[`Windows身份核对`](../internal/kernel/runtime_recovery_windows.go)结合创建时间、session和实际锁，不接管裸PID、不按文件年龄删除锁。session/任务/活动同事务，存储失败保持保护及待写结果。
- UX-001：Wails/App提供明确核对和指定会话结束及确认，显示安全错误/退出码/下一步；待核对即使无PID仍锁关键配置。活动使用environmentId/sessionId，旧记录不能控制后来新开的浏览器；批量关闭不自动强杀。
- [`15条监督器用例`](../internal/workspace/runtime_supervisor_test.go)、[`3条Windows身份/锁/Job用例`](../internal/kernel/runtime_recovery_windows_test.go)及3条新增adapter回归仅编写，全部未运行；重开须原进程身份/实际锁与QUERY核对的确切Job资源均满足，不能以根退出代替子树退出。没有实际崩溃/重开/强制结束或新页面证据，[清单](verification/T07.md)。

### T08 原生代理配置与前检（已实现，未运行验收）

- PRX-001：[`解析`](../internal/proxy/parse.go)与[`导入/编辑/删除服务`](../internal/workspace/proxies.go)分离；URI/兼容文本/IPv6，原行号/错误和共享重复组，选择有效行提交。schema5及[`受保护存储`](../internal/workspace/proxy_storage.go)的user DPAPI密文引用、HMAC请求去重、事务替换/回滚、修订和引用保护，普通响应无用户名密码，不重生成环境seed。
- PRX-001、UX-001：[`HTTP/HTTPS前检`](../internal/proxy/check.go)的连接/TLS/隧道认证/目标访问/实际出口与时刻，[`异步终态`](../internal/workspace/proxy_checks.go)的取消/有界资源、结果待保存不重发网络及重开中断；活动关联真实代理operation错误，不把失败文案投影成功。SOCKS5可保存、检查不支持，不跳TLS或静默直连。
- [`NativeProxyManager`](../src/components/NativeProxyManager.tsx)和Wails安全字段接入native专用路由；keep/replace/clear避免空投影回填认证，错误/未选行保留，清理输入与过期预览，删除确认与引用提示。独立网页原型仍为demo。7条代理库/7条服务/3条adapter回归仅编写未执行，无新UI/实际公共出口证据，[清单](verification/T08.md)。本票不是T09浏览器代理通道或T11断线保护。

### T09 独立认证代理通道（已实现，未运行验收）

- PRX-001、ENV-003：[`桥接`](../internal/proxy/bridge.go)、[`同通道前检`](../internal/proxy/bridge_check.go)、[`运行接入`](../internal/workspace/runtime_network.go)；固定HTTP/HTTPS上游、独立session监听/生命周期，HTTP转发、HTTPS目标CONNECT与代理TLS分开，失败不直接拨号目标。报告仅安全ChannelID/修订/阶段/时间，秘密不进入参数/RPC。
- PRX-001、DATA-001：[`Windows调用进程核对`](../internal/kernel/proxy_guard_windows.go)和创建前QUERY副本绑定；仅当前host+token前检或准确Job客户端，其他进程拒绝。创建后身份读取失败仍保留准确Job/目录资源直到全树确认，锁外解密不阻塞其他查询/停止；重开只核对不复活桥。
- UX-001、ENV-003：[`NativeRuntimeNetwork`](../src/components/NativeRuntimeNetwork.tsx)和App固定direct/proxy策略、未绑定确认直连；安全报告与会话channel/修订错配拒绝。6条桥接/3条内核/8条服务/2条adapter回归仅编写未执行，实际Windows/普通用户沙箱/API/网络/新页面未验证；[清单](verification/T09.md)，T11不因参数或前检通过。

### T10 SOCKS5与远端目标解析（已实现，未运行验收）

- PRX-001、ENV-003：[`SOCKS5`](../internal/proxy/socks5.go)、Bridge及服务，RFC1928/1929指定方法不降级、IDNA DOMAINNAME远端目标DNS/IPv4/IPv6字节，HTTP origin-form/HTTPS隧道不漏认证、只拨上游；BND不当出口IP，错误有准确类型。
- PRX-001：导入/replace协议校验，存储解码中性，keep不解密/改写；切协议不兼容建桥前PROXY_AUTH_INVALID。独立检查临时Bridge，normalStart自己的新桥同通道重检；schema5表不变，安全resolutionPolicy可选，RPC不能覆盖DNS/降级。
- UX-001、ENV-003：[`共享阶段`](../src/application/proxy-network.ts)及native两页显示策略/字节范围/计数、不加密链路及代理host/目标DNS区分。7条代理库/6条服务/2条adapter回归仅编写未执行，[清单](verification/T10.md)，无真实DNS/Windows浏览器/页面，T11未通过。

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

## 尚需桌面实现与真实验证

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
