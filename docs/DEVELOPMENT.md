[返回项目首页与启动说明](../README.md) · [产品需求](PRD.md) · [内核适配](KERNEL.md)

# 本地指纹浏览器开发方案

版本：1.0 · 日期：2026-09-30

本文件将 [产品需求](PRD.md) 转成实施边界、应用接口、状态与数据一致性规则。当前仓库交付 React 和 TypeScript 交互原型；未来桌面版使用 Go 与 Wails 提供本地服务、SQLite 保存配置，并管理已选定的 adryfish/fingerprint-chromium。下面的本地服务接口是目标契约，不代表本仓库已经具备对应 Go 实现。

## 1 当前交付与后续实施分层

| 层         | 当前原型                               | 未来桌面版                                 |
| ---------- | -------------------------------------- | ------------------------------------------ |
| 页面       | 环境、代理、内核、备份、活动、使用说明 | 保持导航和主要交互，替换真实服务反馈       |
| 应用状态   | 当前浏览器 localStorage 中的演示记录   | SQLite 持久配置与本地运行任务              |
| 浏览器控制 | 模拟状态变化，不创建 Chromium 进程     | 受控启动停止、目录锁、进程识别和 CDP       |
| 代理       | 本地文本解析与明确标记的示例检测       | 每环境代理通道、认证、出口检查和断线保护   |
| Cookie     | 解析和导入结果演示                     | 绑定指定环境的协议写入及读取核对           |
| 备份       | 带类型标识的 JSON 原型快照             | 配置数据库、真实浏览数据及清单组成的本地包 |
| 发布       | 前端构建和公开源码                     | Windows 安装包、固定内核来源及供应链校验   |

开发和验收记录应分别标记“设计”“原型可交互”“本地服务实现”“真实桌面验证”。构建通过只能证明前端能打包；浏览器截图只能证明被展示的界面；都不能替代真实代理与数据隔离测试。

### 原型实现约束

[App.tsx](../src/App.tsx) 与 [domain.ts](../src/domain.ts) 是当前原型的行为依据。[PRD 的原型差异表](PRD.md#当前原型和桌面目标的差异)列明交付范围；本文件后续服务、事务、错误码和文件系统设计主要属于未来桌面实现。

当前名称唯一、网址仅 HTTP/HTTPS，运行中的环境不可编辑，关闭未保存的环境抽屉会确认。现有单个按模板复制会建立新 ID、seed 和空示例 Cookie，同时保留原代理绑定供用户检查；不支持批量复制、批量代理分配、设备档案版本回滚、更改默认内核、选定环境备份或垃圾回收区。模拟启动只核对示例内核状态与示例代理检查状态，不会运行真实浏览器。

创建批次每 25 条让出 UI，取消余下任务后保留并记录已创建数量。启动批次可以取消尚未开始的队列；批量关闭会取消待启动任务。代理可编辑演示参数及“模拟连接失败”开关，保存后检查状态重置，关闭失败开关并重检可演示恢复。JSON 和 Netscape Cookie 均支持解析；预览标注已过期记录，保存保留原时间，不把样例强制续期。

演示状态在 React 更新后写入 localStorage；空间不足有提示，但不是“持久化事务成功后再发布 UI 成功”的生产模型。损坏记录保留原文并以阻断界面提供原始记录导出和显式重置；写入前及 storage 事件检测到跨标签页更新时阻断并要求重载，避免旧页面覆盖新数据。不要将这些保护等同于下面的 SQLite 事务、journal、原子替换、回收区及真实运行监督器。

## 2 建议模块边界

```text
React 页面
  ├─ 环境列表与配置抽屉
  ├─ 代理、内核、备份、活动
  └─ 使用说明与需求映射
          │ 应用契约
          ├─ DemoAdapter → localStorage 与模拟事件
          └─ WailsAdapter → Go 本地应用服务
                               ├─ ProfileService → SQLite
                               ├─ FingerprintService → 模板与能力适配
                               ├─ KernelService → 固定版本及文件校验
                               ├─ ProxyService → 每环境代理通道
                               ├─ RuntimeSupervisor → 进程和目录锁
                               ├─ CookieService → 对应环境的 CDP
                               └─ BackupService → 清单、暂存、回滚
```

页面不直接拼浏览器命令行，不直接读写真实 Cookie 数据库，也不直接操作任意磁盘路径。UI 的“启动”经应用服务进入受控流程；演示和桌面使用相同的业务结果结构，使错误、进度和操作 ID 能复用。

进程、数据、代理和指纹管理分别负责自己的资源生命周期。一个环境可使用多个浏览器子进程，因此“环境”不能等同于单个 PID；监督器保存本次根进程身份和受控子进程关系。

## 3 路由和界面契约

| 路由              | 主要服务                              | 页面必须展示的边界                               |
| ----------------- | ------------------------------------- | ------------------------------------------------ |
| `/#/environments` | Profile、Fingerprint、Runtime、Cookie | 启停是否模拟，设备档案是否持久，关键字段何时可改 |
| `/#/proxies`      | Proxy                                 | 检测时间、检测阶段、是否为示例结果               |
| `/#/kernels`      | Kernel、Fingerprint                   | 精确版本、能力状态、是否真实安装                 |
| `/#/backups`      | Backup                                | JSON 演示快照与真实浏览数据包的区别              |
| `/#/activity`     | Activity                              | 操作目标、结果、错误码；不包含凭据内容           |
| `/#/guide`        | 文档映射                              | 需求 ID、已交付范围、文档和页面入口              |

创建编辑使用右侧抽屉，页签为基础、指纹、偏好；Cookie 使用独立对话框。对话框管理焦点、键盘和背景滚动，避免只做视觉遮罩。各列表和选择器引用同一份状态，不维护会逐渐不同步的独立副本。

DOC-001 要求以上页面、需求 ID、开发任务和验收记录互相可追踪；修改业务语义时一并更新 PRD 和使用说明，保留演示与真实实现的边界。

## 4 持久数据模型

### 核心记录

以下字段是未来契约建议。原型可用更轻的数据结构，但必须保留业务语义；迁移到 SQLite 时由适配器显式转换，不能把 localStorage 对象直接当成生产数据库。

原型把 seed、language、timezone、cpu、窗口大小和 Cookie 数组直接放在 Environment 中，内核引用字段为 coreId，分组为 group；没有独立 FingerprintProfile/Revision 表。原型状态是 ready/starting/running/stopping/error，ready 对应已停止。以下 kernelId、fingerprintId、revision 等字段是后续模型，不能作为当前导入文件必需字段。

| 记录                | 关键字段                                                                                                                                        |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Environment         | id、name、tags、note、proxyId、kernelId、fingerprintId、preferences、revision、createdAt、updatedAt、deletedAt                                  |
| FingerprintProfile  | id、seed、templateId、templateVersion、generatorVersion、kernelCapabilityVersion、platform、locale、timezone、hardware、renderPolicy、createdAt |
| FingerprintRevision | id、environmentId、previousProfileId、nextProfileId、reason、createdAt                                                                          |
| Proxy               | id、name、protocol、host、port、credentialRef、dnsPolicy、lastCheck、revision                                                                   |
| Kernel              | id、project、version、architecture、archiveSha256、executableSha256、installPath、capabilityVersion、status                                     |
| RuntimeSession      | environmentId、operationId、rootPid、processStartTime、debugEndpoint、dataLockId、proxyRuntimeId、state、lastError                              |
| Operation           | id、kind、targetIds、state、progress、completedIds、failedItems、cancelRequested、startedAt、finishedAt                                         |
| BackupRecord        | id、kind、schemaVersion、path、manifestHash、environmentIds、createdAt、status                                                                  |
| Activity            | id、operationId、environmentId、action、result、errorCode、safeSummary、createdAt                                                               |

SQLite 使用外键约束、事务和 revision 乐观锁。ID 使用 UUID 或同等级全局唯一值；名称只是展示字段。数据库事务提交后再发布成功事件；持久化失败不得只更新页面状态并返回成功。

数据根目录由应用确定并允许用户通过明确迁移流程调整。建议布局：

```text
app-data/
  app.db
  profiles/<environment-id>/user-data/
  kernels/<kernel-id>/
  backups/
  operations/<operation-id>/journal.json
  staging/<operation-id>/
  trash/<environment-id>/<delete-operation-id>/
  logs/
```

不要在普通配置中保存可任意覆写的 userDataDir。代理密码由本地凭据存储保护，例如 Windows DPAPI 封装后引用；活动日志和 API 默认响应不返回明文。需要跨 Windows 用户或重装系统后恢复时，凭据备份必须使用独立的用户口令加密方案，不能宣称仅复制 DPAPI 密文即可跨机器恢复。

### 必须维持的不变条件

1. 一个环境只引用一份当前设备档案、一个固定内核和一个明确代理策略。
2. 两个环境的实际数据目录不能相同；路径必须落在当前数据根目录下。
3. 一个 kernelId 的二进制及校验值不可原地更改；新版本使用新 ID。
4. seed 在创建时显式持久化，改名称、重启、默认内核变化不触发重新生成。
5. 运行状态是可观察结果；重开程序后必须重新核实，不能直接恢复数据库中的 running。
6. 备份恢复必须保持档案内容；更换内部 ID 时不改变 seed。
7. 运行中的环境不能执行数据目录替换、删除或档案重生成。

## 5 一键设备档案生成

生成器输入为平台、地区模板、固定内核、用户明确覆盖项及模板版本。输出包括已冻结档案、字段来源和能力报告。

| 步骤       | 规则                                              | 验证                                   |
| ---------- | ------------------------------------------------- | -------------------------------------- |
| 选模板     | 第一版仅 Windows 桌面组合，模板有版本和说明       | 不产生跨平台互相冲突的组合             |
| 选内核身份 | 依据精确内核能力映射设置品牌和版本                | UA 及 UA-CH 相关项一致                 |
| 定地区     | 语言、Accept-Language、时区成套；代理地区仅作建议 | 保存后启动不重新跟随地理查询改写       |
| 选硬件偏好 | 从有关系的模板选择 CPU 等受支持项                 | 不接受明显无效值；窗口尺寸单独保存     |
| 生成 seed  | 使用系统随机源，生成内核支持范围内的整数          | 保存前检查本地活动档案碰撞并必要时重试 |
| 固化       | 写档案、生成器版本和模板版本                      | 一次事务保存环境与档案引用             |
| 编译参数   | CapabilityAdapter 只输出受支持参数                | 输出保留、转换、忽略及失败项的报告     |

不要将一键生成理解为随意随机全部字段。模板是管理端规则，噪声实现属于内核；内核未提供的独立 GPU/字体/内存设置不得用输入框冒充。高级字段首先问“固定版本是否支持、我们是否验证过”，再决定可编辑性。

当前已选择 fingerprint-chromium。实际接入时将 tag、二进制摘要、平台和适配器版本一起固定；更新默认版本不能自动重写旧环境。官方文档的开关是接入依据，真实生效结论需要在选定构建上记录。[官方参数及源码说明](https://github.com/adryfish/fingerprint-chromium/blob/main/README-ZH.md)

## 6 本地应用接口

所有接口均为**拟定契约**。Wails 绑定负责 UI 与本机 Go 服务通信，默认不开放未经认证的局域网或公网 HTTP 管理服务。当前原型尚未提取独立适配器；后续先建立 DemoAdapter，使演示流程与 native adapter 遵循同一应用契约。

通用成功返回为 `{ ok: true, data, operationId? }`；失败返回为 `{ ok: false, error: { code, message, retryable, field?, itemIndex?, details? }, operationId? }`。details 必须经过脱敏。修改接口接受 requestId 做幂等处理；涉及已有记录的修改接受 expectedRevision。

| 接口                       | 输入与返回摘要                                                         | 关联需求         |
| -------------------------- | ---------------------------------------------------------------------- | ---------------- |
| Profile.List               | search、filters、cursor → items、nextCursor                            | ENV-001          |
| Profile.CreateBatch        | template、count、namePattern、proxyAssignment、requestId → operationId | ENV-001、ENV-002 |
| Profile.Update             | id、patch、expectedRevision → updatedEnvironment                       | ENV-002          |
| Profile.CloneAsNew         | sourceId、name、proxyId → newEnvironment，新 ID/seed/空目录            | ENV-002、FP-001  |
| Profile.Delete             | ids、stopPolicy、requestId → operationId                               | DATA-001         |
| Fingerprint.Generate       | kernelId、templateId、overrides → previewProfile、capabilityReport     | FP-001、FP-002   |
| Fingerprint.CommitRevision | environmentId、preview、expectedRevision → newRevision                 | FP-001           |
| Runtime.Start              | environmentId、requestId → operationId                                 | ENV-003          |
| Runtime.Stop               | environmentId、gracefulTimeout、requestId → operationId                | ENV-003          |
| Runtime.Inspect            | ids → observed runtime sessions                                        | ENV-003          |
| Proxy.ParseImport          | text、format → validRows、invalidRows、duplicates                      | PRX-001          |
| Proxy.CommitImport         | previewId、selectedRows、requestId → importedIds、failedItems          | PRX-001          |
| Proxy.Check                | proxyId → operationId；结果包含连接/认证/出口阶段                      | PRX-001          |
| Proxy.Assign               | environmentIds、mapping、expectedRevisions → updatedBindings           | PRX-001          |
| Kernel.List                | → installed versions、capabilities、usage                              | CORE-001         |
| Kernel.Install             | source、version、expectedChecksum → operationId                        | CORE-001         |
| Kernel.Verify              | kernelId → verification report                                         | CORE-001         |
| Cookie.PreviewImport       | environmentId、text、format → previewId、counts、conflicts、errors     | CK-001           |
| Cookie.ApplyImport         | previewId、mergeMode、requestId → verifiedCount、failedItems           | CK-001           |
| Backup.Create              | kind、environmentIds、includeKernel、destination → operationId         | BKP-001          |
| Backup.PreviewRestore      | packagePath → previewId、conflicts、requiredKernels、validation        | BKP-001          |
| Backup.ApplyRestore        | previewId、conflictChoices、requestId → operationId                    | BKP-001          |
| Operation.Cancel           | operationId → cancellation state                                       | ENV-001、BKP-001 |
| Activity.List              | filters、cursor → safe events                                          | UX-001           |

长任务发布 OperationProgress、EnvironmentStateChanged 和 OperationCompleted 事件。事件包含操作 ID、目标 ID、单调递增序号及时间；前端忽略旧序号，重新加载时以服务查询结果为准。终态不因迟到事件被改回处理中。

批量创建和运行总数没有产品硬上限。调度器可限制“同时进行的昂贵启动工作”，任务完成后继续队列；不能把启动队列并发数当成允许保持运行的实例总数。磁盘、内存或端口不足时使用资源错误码，保留已完成结果。

当前原型将创建工作分为每批 25 条，并在批间让出 UI；该数字只控制处理节奏，不是创建总数限制。取消时已创建记录保留，活动记录报告实际数量。启动取消只影响尚未开始的任务；关闭操作同时取消待启动队列，防止旧异步任务重新打开已关闭环境。未来 Operation 服务还需持久化任务状态，当前前端没有重启续作能力。

## 7 进程与代理生命周期

状态机以 [ENV-003](PRD.md#env-003-启动停止与失败保护) 为准。启动锁以环境 ID 及规范化数据目录为键；实例 ID 不足以防止两个配置错误地指向同一路径。

启动时固定 executable、校验摘要和数据目录，禁止用户覆盖受管理的 `user-data-dir`、代理及调试监听参数。调试接口仅限本机可达并尽量缩小生命周期；完整端点不写公开活动日志。

代理认证由本地适配层解决，不假设所有 Chromium 参数都支持账号密码。协议不支持时返回 PROXY_UNSUPPORTED，而不是清空配置后直连。SOCKS DNS 策略、WebRTC/UDP 策略和浏览器实际流量应一起检查。

启动前检查代理可用不等于持续断线保护。监督器需要观察代理通道生命周期；通道失效时保持浏览器固定代理，不启用 PAC 的 DIRECT 回退，并用受控测试验证 DNS、HTTP(S) 和允许的 UDP 流量。在没有测试证据前，UI 只能报告检测结果，不能标注“已证明无泄漏”。

停止时先正常关闭并等待；超时保留故障上下文，用户要求强制结束时按本次进程身份匹配，防止 PID 重用误杀其他程序。窗口聚焦与进程重连分开处理；CDP 断开不能直接推断代理断线。

## 8 Cookie 处理实现边界

解析器与浏览器写入器分开。原型解析器已处理 JSON 数组和 Netscape 文本，返回结构化条目及诊断，不执行网络请求；预览显示已过期样例，保存保留其原过期时间。未来桌面服务仍需按浏览器实际协议验证写入结果。含制表符、等号、转义字符、空 value、session、HttpOnly 注释前缀和时间单位的样本必须覆盖。

不通过字符串真值判断丢弃合法空 value；不把过期 Cookie、session Cookie 统一改为一年有效；不无声删除 partitionKey 或 secure/httpOnly/sameSite。需要标准化的字段记录转换原因，不改变用户未要求改变的语义。

写入器校验目标环境及当前会话，使用固定内核对应的 CDP 能力。需要启动环境才能导入时，在 UI 明示并提供受控空白启动，不能偷偷登录目标网站。写后读取逐条核对，以实际成功条数返回结果；协议批量调用无错误不等于全部条目生效。[CDP Network 文档](https://chromedevtools.github.io/devtools-protocol/tot/Network/)

预览不显示完整 Cookie 值；不进入 console、活动记录或遥测。内存中的解析内容在对话框关闭、任务完成或失效后释放。原型无需访问真实 Cookie，只证明解析与交互流程。

## 9 备份和恢复的一致性

### 原型快照

当前 JSON 快照格式与 domain.ts 的 Snapshot 保持一致：

```json
{
  "format": "prism-prototype",
  "schemaVersion": 1,
  "createdAt": "2026-09-30T00:00:00.000Z",
  "environments": [],
  "proxies": [],
  "kernels": []
}
```

文件没有 kind 或 payload 字段。数组中仅包含演示记录，环境可包含示例 Cookie 值；导出排除代理密码、历史快照列表和活动列表。该格式不能用于真实 Chromium 用户目录恢复。

parseSnapshot 校验 format、schemaVersion、主要记录字段、Cookie 数组、引用和重复 ID，再进入恢复确认。restoreSnapshot 拒绝在模拟运行/启动/停止状态下恢复；成功后替换环境、代理和内核数组，保留当前备份历史及活动容器。环境重置为 ready，代理密码清空、状态重置为 unchecked、延迟清空，seed 保持不变。

当前更新内存后由 React effect 写入 localStorage，空间不足提示未持久保存，不能声称导入具有 SQLite 级事务保证。后续桌面版须先完成持久事务再发布成功；其完整记录/引用/摘要校验以以下真实备份要求为准。

原型还保存上一次已读取/写入的存储文本用于检测跨标签页更新。存储已被其他页面改动时停止覆盖并要求重载。初始化遇到损坏原文时保留该文本，阻止自动写回，用户可导出诊断副本或明确重置；此原文导出不是可直接恢复的有效快照，不能更改其格式标识来绕过校验。

### 桌面备份

1. 计算范围并取得维护任务锁，阻止相关环境在备份期间重新启动。
2. 正常停止环境，等待所有受控进程退出及目录锁释放。失败时结束任务，不输出完整成功包。
3. 使用 SQLite 一致性备份方式取得数据库快照；不要在 WAL 活跃时只复制 app.db。
4. 将档案、凭据备份材料、user-data-dir 和内核清单写入临时包，记录文件相对路径、长度及 SHA-256。
5. 写入清单并校验包内容，完成后将临时文件重命名为最终包。失败包保留为明确的临时/失败状态，不能出现在“可恢复成功备份”列表中。

浏览器数据可能包含 Windows 用户绑定的加密信息。首个桌面验收范围先确保同一 Windows 用户上下文中的本软件恢复；重装系统、跨电脑或其他用户账户的登录状态恢复，必须另做显式支持和验证，不由“目录复制成功”推断。

### 原子替换与崩溃恢复

文件目录替换与 SQLite 事务不能组成一个天然的整体原子操作，使用维护锁、同卷目录切换和持久操作日志实现可恢复一致性：

| 阶段          | 持久记录                             | 操作                                                               | 失败后处理                                         |
| ------------- | ------------------------------------ | ------------------------------------------------------------------ | -------------------------------------------------- |
| prepared      | 包摘要、目标目录、旧引用、计划新引用 | 在 staging 解包并校验；检查路径越界、符号链接/重解析点、大小和空间 | 删除或保留失败暂存，原数据不变                     |
| locked        | 被锁环境与原运行状态                 | 停止环境并阻止重新启动，创建恢复前数据库快照                       | 释放维护锁，保留现状                               |
| swapping      | 每个目录的旧/新位置及完成标记        | 旧目录移到回滚位置，同卷将验证过的新目录移入                       | 按日志反向恢复已切换目录                           |
| db_committing | 待应用记录及版本                     | SQLite 事务应用引用映射、代理和档案，并提交                        | 数据库未提交则回滚目录；提交情况不明先核对任务标记 |
| committed     | 数据库任务完成标记                   | 验证引用、目录、档案、内核；保留回滚副本一段可配置时间             | 启动前检测不一致并进入恢复向导                     |
| finalized     | 完整校验结果                         | 释放维护锁并显示完成                                               | 后续清理失败只报清理警告，不丢失恢复结果           |

应用重开先扫描未完成日志，在提供启动按钮前恢复到完整旧状态或完整新状态。以数据库中的任务提交标记判断 DB 是否完成，不仅凭 journal 的最后一行猜测。不同卷目录不能假设重命名具有相同保证，应先复制到目标卷暂存后再切换。

全量恢复不以“部分成功”作为总体成功终态。必要组件出错返回 RESTORE_INCOMPLETE，展示已完成和未完成项目，环境保持维护锁定直到回滚或修复完成。

## 10 错误码和用户反馈

| 错误码                                       | 含义                             | 用户可执行动作                           |
| -------------------------------------------- | -------------------------------- | ---------------------------------------- |
| VALIDATION_FAILED                            | 字段、导入行或参数无效           | 定位到字段或行号修正                     |
| REVISION_CONFLICT                            | 已被其他操作更新                 | 重新加载最新记录后重试                   |
| PROFILE_BUSY                                 | 环境启动、运行、停止或维护中     | 等待或停止后操作                         |
| DATA_DIR_LOCKED                              | 数据目录已被占用                 | 查看占用状态，禁止重复打开               |
| PATH_OUTSIDE_ROOT                            | 路径越出允许根目录或含不允许链接 | 更换目标，保留原数据                     |
| KERNEL_MISSING                               | 固定版本未安装                   | 安装同版本再启动                         |
| KERNEL_INTEGRITY_FAILED                      | 内核摘要不匹配                   | 重新获取并校验                           |
| CAPABILITY_UNSUPPORTED                       | 字段不受当前内核支持             | 使用已支持配置或明确更换内核             |
| PROXY_INVALID / PROXY_UNSUPPORTED            | 配置格式或协议不可用             | 修正代理配置                             |
| PROXY_AUTH_FAILED                            | 认证失败                         | 更新凭据后检测                           |
| PROXY_UNREACHABLE / PROXY_ROUTE_LOST         | 连接失败或运行通道断开           | 修复代理，保持失败保护                   |
| PROCESS_START_FAILED / PROCESS_READY_TIMEOUT | 进程启动失败或未就绪             | 查看阶段与日志摘要后重试                 |
| COOKIE_PARSE_FAILED                          | Cookie 输入不可解析              | 修正格式或错误行                         |
| COOKIE_WRITE_PARTIAL                         | 部分条目未实际写入               | 查看条目原因，保留成功明细并重试失败项   |
| SNAPSHOT_INVALID                             | 演示快照损坏或格式不匹配         | 保留现有数据，选择正确文件               |
| BACKUP_INVALID / BACKUP_VERSION_UNSUPPORTED  | 真实包校验失败或版本不支持       | 不修改现状，选择兼容完整备份             |
| RESTORE_INCOMPLETE                           | 恢复必要组件未完成               | 回滚或完成修复后解锁                     |
| RESOURCE_EXHAUSTED / DISK_FULL               | 内存、端口、磁盘等实际资源不足   | 降低同时处理量或释放资源，保留已完成项目 |
| STORAGE_WRITE_FAILED                         | 原型或桌面持久化失败             | 重试或导出当前可用数据，不能提示保存成功 |
| OPERATION_CANCELLED                          | 用户取消剩余任务                 | 查看已完成与未执行清单                   |

错误 message 用简短中文说明“哪里失败、原数据是否仍在、下一步是什么”。详细技术信息可展开；不把栈追踪或凭据直接当成弹窗正文。

## 11 实施顺序和验收门槛

| 阶段              | 交付                                               | 进入下一阶段的证据                                                                                             |
| ----------------- | -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| P0 交互原型       | 本次前端、PRD、开发文档、公开仓库                  | 六个路由互通；抽屉/对话框/正常持久化/全工作区演示快照闭环；通过 PRD 原型验收列；构建通过；示例及未实现边界清楚 |
| P1 本地配置底座   | Go/Wails、SQLite、迁移、档案生成、代理和内核元数据 | 新建编辑后重开应用数据仍在；seed 不变；事务失败不产生悬空引用                                                  |
| P2 真实内核与隔离 | 精确版本启动、目录锁、进程监督、代理适配           | 两个环境真实打开，Cookie/LocalStorage/IndexedDB 隔离；错代理阻断；断线测试通过                                 |
| P3 Cookie 与批量  | 真实写入核对、批量创建/启动队列、取消重试          | Cookie 语义样本通过；逐项结果准确；大批次不锁死界面                                                            |
| P4 备份恢复       | 完整包、校验、操作日志、回滚、重启恢复             | 正常恢复及中途崩溃/磁盘不足/坏包等故障测试，旧数据可恢复                                                       |
| P5 Windows 交付   | 安装卸载、升级、签名策略、内核说明和操作指南       | 干净用户环境实装及重开验证，用户数据保留策略符合文档                                                           |

当前不实施云账号、权限、平台/插件专项和第三方迁移。没有为每个阶段承诺固定工期；以门槛证据推进，不用已写代码行数替代可用结果。

## 12 验证策略与测试数据

| 范围     | 必要检查                                                                                                              | 不可替代的真实证据                               |
| -------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ |
| 前端交互 | 路由、唯一名称创建编辑、分组/状态筛选、已有批量操作与取消、模拟失败、键盘及正常刷新持久化；不把未实现功能纳入已通过项 | 实际浏览器点击与布局截图                         |
| 生成器   | 相同已保存档案反复读取不变；显式再生成变化；内核不支持项拒绝                                                          | 固定内核逐字段的页面读取结果                     |
| 代理     | URI 解析、认证错误、绑定引用和日志脱敏                                                                                | 测试代理的成功/断线/恢复及实际出口捕获           |
| Cookie   | 空 value、session、过期、SameSite、HttpOnly、特殊字符和分区字段                                                       | 写后读取对照，逐条错误报告                       |
| 目录隔离 | 路径规范化、目录锁、重复引用和越界拒绝                                                                                | 两个真实环境存储隔离及重开                       |
| 备份     | 清单格式、摘要、引用一致、失败注入                                                                                    | 同用户 Windows 上完成备份→改动→恢复→真实重开     |
| 崩溃恢复 | 每个 journal 阶段中断后再启动                                                                                         | 回到完整旧状态或完整新状态，禁止混合数据继续启动 |

测试只使用合成账号信息、示例代理和临时测试环境。Cookie 样本不包含真实登录凭据；公共演示不展示私人 IP、地址或商户数据。原型模拟成功和错误应该可重复，测试时不依赖随机等待导致偶发失败。

原型操作建议按顺序检查：新建两个不同名称的环境 → 分别编辑绑定不同演示代理 → 修改一个已停止环境的名称 → 刷新 → 验证两个 seed → JSON/Netscape Cookie 导入预览 → 模拟启动停止 → 导出全工作区快照 → 修改数据 → 停止所有环境 → 导入恢复 → 验证环境/代理/内核关联页。恢复后检查代理为空密码且待检查，环境为 ready，seed 未改变。

另外验证同名拒绝、about:blank 网址拒绝、运行中编辑拒绝、未保存抽屉关闭确认、无效 JSON/错误 schema 拒绝。批次验证覆盖每 25 条让出 UI、取消后保留实际创建数量、启动队列取消以及批量关闭不被旧队列重新启动。按模板复制检查原代理仍绑定、新 seed 不重复、示例 Cookie 为空；代理模拟失败开关关闭后可重检恢复；过期 Cookie 的时间不得被改成未来。

存储验证覆盖损坏原文不被覆盖、阻断界面可导出原始记录及显式重置，以及跨标签页更新后旧页被阻断并要求重载。存储写入失败检查“尚未持久保存”反馈；当前内存与持久数据可能不同，不能按原型测试结果宣称生产级回滚已通过。生成器/档案历史回滚、批量复制、批量代理分配、选定环境备份和回收区仅在对应桌面阶段验收。

## 13 公开仓库与资料引用

仅提交本项目独立编写的源码、文档及明确许可的依赖声明。真实 user-data-dir、日志、代理凭据、Cookie、备份、内核二进制和旧发布归档均不应进入 Git；配置模板只放占位数据。

可引用资料：

- [fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium)：已选定内核，实际发布遵守其许可证及依赖要求；源码可用性以固定 tag 核实。
- [Ant-Browser](https://github.com/black-ant/Ant-Browser)：结构与功能参考，独立实现，不能把公开可读源码等同于已取得复制授权。
- [Wails 官方文档](https://wails.io/docs/introduction/)：未来 Go 与 Web 前端桌面桥接。
- [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/)：真实浏览器协议能力按固定内核核对。
- 旧材料文件 `02-原始主进程与预加载代码/readable-7.1.5/main/electron-main.js`、`preload/electron-preload.js`：只作流程研究，不收入本仓库。尤其不搬其云签名流程、Cookie 强制续期、吞错返回成功和专用内核配置协议。

每次新增真实能力应同步 [PRD 验收矩阵](PRD.md#7-验收矩阵) 与首页交付说明，写明验证条件、实际结果和仍未验证的边界。
