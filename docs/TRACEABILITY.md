[项目首页](../README.md) · [产品需求](PRD.md) · [开发方案](DEVELOPMENT.md) · [内核合同](KERNEL.md) · [验收记录](ACCEPTANCE.md)

# 需求到实现的追踪表

版本：1.0 · 日期：2026-09-30

本表将 PRD 的 12 项需求关联到当前前端入口和建议验收。**出现源码入口只代表存在相应原型逻辑，不代表完整桌面能力已经实现，也不代表测试已经通过。** 实际执行记录统一放在 [ACCEPTANCE.md](ACCEPTANCE.md)。

路由是运行应用后的 hash 路由。源码链接指向文件，函数名用于定位；前端持续修改时不依赖易失效的固定行号。领域逻辑自动测试入口为 [`tests/domain.test.ts`](../tests/domain.test.ts)，页面流程仍需真实浏览器操作检查。

## 12 项需求映射

| 需求 ID                       | 原型路由与交互入口                                        | 源码定位                                                                                                                                                       | 建议验收；不表示已执行                                                                                                                                     |
| ----------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ENV-001 环境列表与批量操作    | `/#/environments`；搜索、筛选、选择、分页、批量按钮       | [`App.tsx`](../src/App.tsx)：`App` 内 `visible`、`pageItems`、`selected`，`saveEnvironment`、`launch`、`stop`、`removeEnvironments`                            | 名称/编号/备注搜索及空结果可恢复；筛选和翻页不扩大已选 ID；批量任务只影响所选项，失败单独显示；取消保留已完成项；无产品数量配额。                          |
| ENV-002 创建与编辑环境        | `/#/environments`；新建/编辑右抽屉                        | [`App.tsx`](../src/App.tsx)：`openCreate`、`openEdit`、`patchDraft`、`saveEnvironment`；[`domain.ts`](../src/domain.ts)：`validateEnvironment`                 | 名称不能为空且不重名；非法网址/窗口尺寸不保存；取消不写入草稿；模板新建生成新 ID 和 seed、不复制 Cookie；代理选择明确；运行中的关键配置受保护。            |
| ENV-003 启动停止与失败保护    | `/#/environments`；单个与批量启停；`/#/activity` 查看原因 | [`App.tsx`](../src/App.tsx)：`launch`、`stop`、`event`；[`domain.ts`](../src/domain.ts)：`launchError`                                                         | 未检查/失败代理及缺失内核阻止模拟启动；无静默直连；快速重复点击不重复执行；关闭或取消后没有遗留启动任务；状态与日志一致且持续标明模拟。                    |
| FP-001 固定设备档案           | `/#/environments`；抽屉“指纹”页签                         | [`App.tsx`](../src/App.tsx)：`openCreate`、`regenerate`、`saveEnvironment`；[`domain.ts`](../src/domain.ts)：`uniqueSeed`、`createSnapshot`、`restoreSnapshot` | 新建 seed 在支持范围内且检查冲突；关闭/重开、修改代理及恢复不自动换 seed；重新生成取消不生效、保存后生效；示例 Cookie 不随重生成清空。                     |
| FP-002 指纹能力分层           | `/#/environments` 的指纹说明；`/#/kernels` 的能力说明     | [`App.tsx`](../src/App.tsx)：指纹页签、内核能力展示及 `kernel` 对话框；[`domain.ts`](../src/domain.ts)：`Environment`、`Kernel`                                | 区分可配置、内核按 seed 生成和待核对项；原型不展示伪造的真实硬件读值；窗口大小与屏幕指纹区分；更换版本不能宣称已有真实验证。                               |
| PRX-001 代理导入检测与分配    | `/#/proxies`；导入/编辑/检查；环境抽屉代理绑定            | [`domain.ts`](../src/domain.ts)：`parseProxyText`、`launchError`；[`App.tsx`](../src/App.tsx)：`openProxyImport`、`checkProxy`、代理编辑与绑定控件             | HTTP/HTTPS/SOCKS5、转义凭据和 IPv6 正确解析；错误行有行号；密码不进入列表与日志；被引用代理不能直接删除；检查清楚标为模拟，编辑后重新检查。                |
| CK-001 Cookie 导入            | `/#/environments`；环境行“导入 Cookie”                    | [`App.tsx`](../src/App.tsx)：`openCookies`、Cookie 预览及提交；[`domain.ts`](../src/domain.ts)：`parseCookies`、`mergeCookies`                                 | JSON/Netscape 格式可预览；错误数据不提交；空 value、会话属性、到期字段及分区信息保留；按完整身份键合并；写入只影响选定示例环境，预览和日志隐藏值。         |
| CORE-001 固定内核版本         | `/#/kernels`；环境抽屉内核选择                            | [`domain.ts`](../src/domain.ts)：`Kernel`、`seedState`、`launchError`；[`App.tsx`](../src/App.tsx)：内核页、`kernel` 对话框、`saveEnvironment`                 | 环境保存具体内核 ID；候选/不可用内核阻断模拟启动；现有环境不跟随其他选择自动升级；页面说明未安装、未校验或运行真实内核。                                   |
| BKP-001 快照备份与恢复        | `/#/backups`；创建、导出、导入、确认恢复                  | [`App.tsx`](../src/App.tsx)：`newBackup`、`confirmRestore`、`download`；[`domain.ts`](../src/domain.ts)：`createSnapshot`、`parseSnapshot`、`restoreSnapshot`  | 导出排除代理密码；坏格式、重复 ID、断引用、非法字段被拒绝；运行环境未停止时不恢复；有效恢复保留 seed、重置代理检查、保留原型历史记录；无真实目录备份承诺。 |
| DATA-001 数据隔离与删除       | `/#/environments`；单个与批量移除；Cookie 目标环境        | [`domain.ts`](../src/domain.ts)：`Environment.id`、`mergeCookies`；[`App.tsx`](../src/App.tsx)：`removeEnvironments`、Cookie 提交                              | A 环境修改不改变 B 的记录或 Cookie；运行中的环境不能移除；移除前展示数量与数据含义；原型不操作文件，不声称已验证 Chromium 目录隔离或回收区。               |
| UX-001 可访问性与本地持久演示 | 六个页面；抽屉、弹窗、反馈及存储提示                      | [`App.tsx`](../src/App.tsx)：`App` 的持久化/键盘/焦点 effects、`notify`、表单校验与空状态；[`styles.css`](../src/styles.css)                                   | 键盘打开/关闭、焦点约束和返回位置可用；状态有文字；刷新后演示数据保留；坏存储与多标签页冲突不静默覆盖数据；窄窗口仍能触达关键操作。                        |
| DOC-001 文档与页面可追踪      | `/#/guide`；文档页签及下载入口                            | [`App.tsx`](../src/App.tsx)：`docTab`、指南页 Markdown 展示、`download`；[`PRD.md`](PRD.md)、[`DEVELOPMENT.md`](DEVELOPMENT.md)、[`KERNEL.md`](KERNEL.md)      | 页面可读三份主文档并下载；12 个 ID 在 PRD 和本表一致；相对链接有效；当前原型与后续桌面目标明确分开；实际检查结果可追溯到验收记录。                         |

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
