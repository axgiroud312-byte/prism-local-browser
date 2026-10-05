[返回项目首页与启动说明](../README.md) · [产品需求](PRD.md) · [内核适配](KERNEL.md)

# 本地指纹浏览器开发方案

版本：1.0 · 日期：2026-09-30

本文件将 [产品需求](PRD.md) 转成实施边界、应用接口、状态与数据一致性规则。仓库保留 React/TypeScript 原型及 T01 的应用契约与 DemoAdapter；T02 的 Go/Wails、SQLite 和 WailsAdapter 本机配置底座已验收，T03 用户级安装预览正在验收（见 [安装说明](INSTALLATION.md)）。已选定的 adryfish/fingerprint-chromium 仍未安装或运行。下文完整接口是分票目标，不表示已全量实现；实际范围见 [PROGRESS.md](PROGRESS.md)。

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

演示状态由 DemoAdapter 集中写入 localStorage；写入失败不发布新状态，创建编辑保留可重试草稿，不提示保存成功。损坏记录保留原文并以阻断界面提供原始记录导出和显式重置；写入前及 storage 事件（含 clear）检测到跨标签页更新时阻断并要求重载，避免旧页面覆盖新数据。不要将这些保护等同于下面的 SQLite 事务、journal、原子替换、回收区及真实运行监督器。

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

原型把 seed、language、timezone、cpu、窗口大小和 Cookie 数组直接放在 Environment 中，内核引用字段为 coreId，分组为 group；T05 在额外演示元数据中保存档案历史，不改变原型快照格式。原型状态是 ready/starting/running/stopping/error，ready 对应已停止。以下 kernelId、fingerprintId、revision 等字段是桌面模型，不能作为当前原型导入文件必需字段。

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

### T08 原生代理增量（已实现，未运行验收）

- SQLite schema5新增 `proxy_config`（配置/修订/当前检查）、`proxy_credentials`（DPAPI密文）、`proxy_request_key`（DPAPI保护的随机HMAC key），旧环境/档案/数据引用不变。普通proxy配置只有受保护引用，用户名和密码都不回显；更新/清除/删除凭据与配置同事务。跨用户恢复不能只拷贝DPAPI密文，见[D009](DECISIONS.md#d009--代理凭据与配置同事务且普通响应不回显2026-10-01)。
- `Proxy.ParseImport({text})`返回native预览、原行号、安全配置、错误与共享重复组，不返回raw URI或认证。规范URI/兼容文本/IPv6/IDN；同地址仅为候选、不禁止另存。2MiB输入边界仅保护内存，无保存节点配额。预览15分钟清理；新输入/显式Discard/提交/退出清除秘密。
- `Proxy.CommitImport({previewId,selectedRows,requestId})`只保存服务预览中的所选有效行，同批次事务。`Proxy.Update({proxyId,expectedRevision,configuration,credentials,requestId})`认证需keep/replace/clear，keep不带凭据，replace接受新username/password；旧检查一律失效。被环境引用的代理不能直接Delete，关联busy环境的网络/认证修改被阻止，名称/地区标签允许保存且不换seed。
- `Proxy.Check({proxyId,expectedRevision,requestId})`受理operation而非成功。固定经该代理CONNECT访问HTTPS出口目标，区分连接/代理TLS/认证请求/目标TLS/目标访问/实际IP与时刻。407、TLS失败、不可达、超时或SOCKS5未支持不直连回退；不跟随重定向、不跳过验证、不将任意peer body/原始error写报告。
- 网络锁外执行、4并发资源调度和25秒有界；取消/终结关闭本次socket，包括等待CONNECT的资源。会话内报告及任务终态、活动同事务；结果待保存时保留busy/临时覆盖，查询只重试持久化、不重复网络。重开标未完成任务APPLICATION_INTERRUPTED，不自动重发。失败活动关联operation的实际errorCode，不能只存失败文案却标成功。
- NativeProxyManager单独接入，旧网页demo不变。普通state.proxies只空认证投影供环境绑定；新编辑只读nativeProxyRecords的hasAuthentication，不能从空密码投影回填。原始输入默认mask，未选/错误行可继续修正；秘密不持久化到localStorage，取消/卸载和迟到返回有清理。
- 所有新增回归仅编写，实际DPAPI/网络/native页面均未验证；[清单](verification/T08.md)。HTTP认证链路無TLS加密，DPAPI不保护传输；HTTPS才提供TLS到代理。T08提交时绑定proxy的Start阻断，T09随后接独立浏览器通道；独立前检不代替运行期断线保护。

### T09 每环境认证通道（已实现，未运行验收）

- [`Bridge`](../internal/proxy/bridge.go)为每次运行session建独立TCP4 loopback监听，仅向固定HTTP/HTTPS上游拨号。标准HTTP转发与CONNECT分开；HTTPS上游先校验TLS，不跳验证，认证只给上游。临时响应继续读最终状态、流式正文刷新；明文HTTP Upgrade明确阻断，TLS隧道字节透传。
- [`准入`](../internal/kernel/proxy_guard_windows.go)将loopback与身份核对分离：前检需hostTCP caller和私有token；正常浏览器需准确Job成员进程句柄、反向TCP tuple和二次新鲜查询。QUERY副本创建前绑定、与本次channel/进程同生命周期，不按裸PID/进程名放行；边界[D010](DECISIONS.md#d010--每会话代理通道只接纳其受控调用进程2026-10-01)，普通用户/沙箱兼容性未实测。
- `Runtime.Start`策略direct/proxy必须匹配保存绑定，proxy不能降direct；T09提交时SOCKS5未接，后续T10接入。预留busy后锁内读密文、锁外解密/建桥；同监听前检及报告事务提交后才启动。endpoint/token/上游秘密不进入RPC/日志/参数，只返回安全ChannelID/修订/阶段。
- schema5表不变，runtime session JSON增加代理ID/修订/channelID/安全报告。取消/失败/Job全树退出/应用关闭收敛对应通道；Done含桥接和进程资源，存储重试不重复网络，重开仅核对不恢复桥。创建后时间观测失败仍保留原始句柄/Job并观察全树退出，空时间特殊记录不猜PID；恢复同时核对精确Job、实际锁和原session元数据。
- 前端[`NativeRuntimeNetwork`](../src/components/NativeRuntimeNetwork.tsx)显示启动前的同通道阶段与本次/历史IP，不把它叫持续网页出口；待核对原PID只能展示历史报告，明确当前未接管/重建旧桥。未绑定环境仍显式确认直连，已绑定按proxy策略；Wails拒绝演示报告与channel/配置修订错配。
- 所有新增回归仅编写，没有实际Windows/API准入/公共网络/真实浏览器或新页面验收，[清单](verification/T09.md)。代理参数及QUIC/WebRTC UDP限制不是全路径无泄漏证据，T11运行期故障/隔离与恢复仍待实现验收。

### T10 SOCKS5与远端目标解析（已实现，未运行验收）

- [`SOCKS5`](../internal/proxy/socks5.go)共用Bridge/会话生命周期，RFC1928 CONNECT及RFC1929无认证/用户名密码单一方法，不降级。认证各1–255个UTF-8字节，SOCKS用户名冒号合法，HTTP Basic不合法；存储解码中性，keep不读取/改写，切协议不兼容建桥前PROXY_AUTH_INVALID。
- IDNA DOMAINNAME交上游解析，IPv4/IPv6按字节；只Dial保存代理，不本机解析目标或直连，代理host自身仍可本机DNS。完整BND帧不当出口IP，地址不支持/不可达准确报告。[D011](DECISIONS.md#d011--socks5目标域名固定远端解析且认证不降级2026-10-01)。
- HTTP经SOCKS流写origin-form，HTTPS目标CONNECT后透传，认证不进目标/RPC。独立Proxy.Check也走临时Bridge；环境仍自己的新桥同通道重检。底层proxy.Check只是桥内HTTP入口原语，不是SOCKS公开服务路径。
- 报告JSON增可选安全resolutionPolicy，schema5表不变；取消/超时优先、result与最终错误一致。两native页面用[`共享阶段`](../src/application/proxy-network.ts)显示策略/认证字节范围/不加密边界，RPC不得覆盖DNS/目标/认证或DIRECT回退。
- 7条代理库/6条服务/2条adapter回归仅编写未执行，无真实DNS/API/Windows浏览器或新页面证据，[清单](verification/T10.md)。远端目标解析不是全路径DNS保证，SOCKS UDP/BIND未支持，T11关闭/隔离/故障恢复待后续。

### T11 网络故障监督与安全门禁（部分实现，完整隔离未实现）

2026-10-05当前开发目标按[六阶段计划](V1_DELIVERY.md)：先持久记录容器、权限增量和准确进程/Job归属，再创建隔离资源、专属桥并接入正式启动。记录必须支持创建结果未知后的重开核对；只有全树退出及权限/桥/容器清理确认才释放环境。恢复不能按裸PID或旧路径字符串操作不明对象。首版支持前提已确认是Windows网络隔离正常，底层服务损坏保护留后续；应用/桥/上游故障仍必验。以下旧条目保留历史实际实现边界，不表示生产provider已存在。

- [`门禁`](../internal/kernel/network_protection.go)在workspace真实代理Start读取凭据/建桥前及kernel实际CreateProcess前分别执行，当前NETWORK_PROTECTION_UNAVAILABLE不可重试。独立代理检查保留，真实代理浏览器不能启动；host-only合成launcher不让真实kernel绕门禁，RPC无关闭保护参数。
- [`Bridge故障`](../internal/proxy/bridge_watch.go)每桥闭锁/Failed事件、30s同桥前检巡检及4背景资源调度；仅实际上游/监听故障触发，单请求取消/超时/上传失败不误关会话。准确Job独立安全停止不等服务锁或SQLite，全部资源确认退出前不释放目录。
- schema5表不变，session JSON增networkFault：network_error、安全根因/观测时刻及stopping/stopped/exit-unconfirmed。旧Environment status仍error。含启动期间、未返回process的真实闭锁故障，清理/强制结束/重开均保留根因；copy-on-write保留先前阶段，失败落盘只重试持久化，不重发网络/终止。未終结Stop的预留不随error展示提前释放。

2026-10-05生产接入准备：[`BridgeIngress`](../internal/proxy/bridge_ingress.go)以host-only listener/前检dialer成对提供会话入口，错误不退普通socket；前检/巡检仍保调用方准入和随机token。Bridge关闭取消全部前检并等待未返回dial清理，入口数据在创建时冻结。仅完成bridge资源接点，未增加schema或生产隔离provider；私有资源日志、目录授权/恢复和系统故障仍待实现，[T11记录](verification/T11.md)。

同日资源收尾修订：进程Done与通道Close均需成功才结束会话资源owner；创建浏览器之前的通道失败也保留环境占用与正常关闭重试。新响应字段`resourcesPending`由当前owner投影，不持久化为恢复许可、不受RPC覆盖；页面不以PID0当空闲。应用后续明确CloseContext锁外重试既有owner（包括首次shutdown scan后登记项），保留原完成信号；崩溃/启动根因不被清理异常覆盖。新回归仅编译未运行，见[T11](verification/T11.md)。
- native页面明确门禁、独立前检不授权浏览器与资源退出事实。7桥/2内核/9服务/1adapter回归全未运行，Go测试包未编译，[清单](verification/T11.md)。全路径WFP broker/委托DNS/崩溃保持拒绝未实现，不宣称T11已完整开发。
- 用户允许管理员安装后续隔离组件，但不现在提权/改系统；已选择先保存部分、继续不依赖T11的Cookie/批量/备份，[D012](DECISIONS.md#d012--安全边界缺失先阻止代理启动并允许隔离组件管理员安装2026-10-01)。特权WFP匹配独立程序路径不自动覆盖DNS Client委托或准确Job，禁止全局封DNS/关闭沙箱/换内核掩盖缺口。

## 6 本地应用接口

### T21 独立脱敏诊断（本地源码，未运行验收）

[`DesktopApp`](../main.go)在普通Service分发之前提供`Diagnostics.Preview/Export/EndVerification`，因此数据库Open失败也能返回最小报告；只有native模式。预览无客户端输入，后两者只接收`reportId/requestId`，路径由host系统保存器提供；不调用Workspace.Read、待写flush、浏览目录、DPAPI或内核探测。

[`报告`](../internal/workspace/diagnostics_report.go)采用独立结构/固定枚举，未知文本变other/unknown。只取一致只读事务中的计数和有界记录，schema10无需新迁移。TryLock繁忙立即最小报告，SQL查询有10s context；坏节标partial。最多100任务/100会话/20内核，单条过大或不可解析计omitted；只导出已保存状态与host维护标志，不声称实时进程/网络结果。进度含明确`progressMetric`。

[`导出host`](../internal/workspace/diagnostics_host.go)冻结15分钟预览及实际JSON字节/SHA-256，保存至工作区外新JSON，使用原目录/文件句柄发布，不覆盖。失败仅对自有未发布临时对象标删除；发布后关闭未知只核对原文件，不重写。应用会话内保留按requestId的输入/结果账本，旧请求换报告拒绝；账本不跨应用持久化，重启不自动重放。EndVerification在无在途操作时返回已知回执或明确unconfirmed，保留原记录，允许用户结束核实再发新请求。

[`adapter`](../src/application/diagnostics-client.ts)持有离页状态，合并同操作在途Promise，未知响应保留原请求。固定格式/模式/报告ID/摘要匹配才确认；明确写入前失败才允许新请求。页面展示报告排除字段与not-checked签名状态，实际签名仍来自发布构建记录。没有云上传与新依赖。[T21清单](verification/T21.md)。

### T13 持久批次与服务端分页（源码已编写，未运行验收）

- `Batch.Preview`接受`kind: create`与原生创建草稿/配置/数量，或`clone`与明确sourceIds，或`assign`与逐环境proxyId映射。30分钟计划冻结安全配置/固定档案、源/目标环境修订及节点修订；不接受秘密、任意路径、浏览数据或客户端身份。创建用模板与正JS安全整数count虚拟表示，预览不为所有项分配ID/seed/目录；不是环境总数配额。
- `Batch.Commit({planId,requestId})`与`Batch.Retry({operationId,requestId})`将受理、去重、当前尝试及计划同事务提交；Retry只接当前失败/取消尝试，索引跳过已完成行，虚拟创建的未物化后缀仍保留。提交结果未知先核对原request/plan/op，确认已落盘才挂原worker一次，无法核对保持acceptance-pending，不换ID重做。重开中断不自动执行；当前终态/观测待存储只重试日志保存，不重复环境或目录副作用。
- schema6新增`batch_plans`、`batch_items`、`batch_item_events`、预约身份/seed与历史seed/未完成项索引。每项先持久准备新ID/seed/完整档案，再取得[`空目录归属Lease`](../internal/kernel/empty_profile_windows.go)。userdata外marker匹配plan/index/id/ref且持固定句柄；拒外来/无标记/非空目录，绝不清空/覆盖/删除或复制源登录数据。文件系统与SQL不原子，遗留准备目录保持，不悄悄认领；同SID恶意竞争不是本票强隔离承诺。
- 每项环境副作用、完成/失败、统计及安全事件同事务。创建首项保草稿明确seed，其他创建/所有clone新seed并按原精确构建重新编译；原pending仍不可启动，不新增非法生成器版本。普通创建/档案提交也检查批次seed预约，不能消费另一准备身份。Assign同事务改配置JSON/proxy引用/环境revision，不换seed、不增加档案revision。忙状态用归属明确的独立batch lease保护，旧Runtime.Reconcile/Runtime/Cookie释放不能删除它；提交前重核真实会话与预览修订。
- `Batch.ReadPage({planId,operationId?,offset,pageSize})`最多100条；不指定尝试查询当前计划，指定旧尝试按其原安全item事件与此前已完成项返回冻结结果，不展示后来准备身份/成功。报告有completed/failed/notExecuted/attemptCompleted/全计划direct/shared/sequence，操作不积累全量CompletedIDs。`Operation.Cancel`只持久取消此批次剩余项，资源不足暂停保已提交项，失败/取消活动不标成功。
- `Workspace.Read({environmentQuery?:{page,pageSize,search,group,status}})`默认8条；SQL筛选/计数考虑待保存会话观测，环境/档案/ref/session只投影当前页。活动最多100、Cookie和batch尝试最近30、kernel任务20；旧操作可按ID读取，恢复查询独立于展示窗口。关联UsedBy最多100但UsedCount准确，保护检查全部host leases，不用样本裁定。
- [`native批次页`](../src/components/NativeBatchDialog.tsx)明确整个计划的直连/共享数量、逐目标节点、当前/历史尝试及取消/继续。异步响应按plan/op/页位置/选择代次匹配，终态后独立读最终页；不同页缓存不互挡。跨页启动通过[`精确ID读取`](../src/application/runtime-start-plan.ts)确认每个目标网络策略/环境修订，缺失不当直连；普通Runtime.Start现在也可带expectedRevision并拒旧配置，proxy门禁不变。Demo继续旧契约，不混schema1导入。
- 服务16条、Windows空目录3条、adapter/模型6条回归仅编写未执行，Go测试包未编译；当前只读复核及必要静态核对不代表真实目录/恢复/规模/UI验收，[T13清单](verification/T13.md)、[D014](DECISIONS.md#d014--批次冻结计划逐项提交与空目录归属2026-10-01)。

### T12 指定会话Cookie增量（源码已编写，尚未运行验收）

- `Cookie.ParseImport({environmentId,text})`独立纯解析JSON数组/Netscape，返回环境名称/ID/修订、15分钟previewId和不含value的行/数量。运行时绑定当前session且内部读取现存键；停止时现存冲突未知、不自动启动。输入同键按name/domain点/path/完整分区识别，同键只选一行。`Cookie.DiscardImport({previewId})`或空ID丢弃当前预览/迟到结果，关闭/新输入/超时也清理。
- `Cookie.CommitImport({previewId,environmentId,expectedRevision,sessionId,selectedRows,policy,requestId})`只用服务内存中的所选值，严格当前revision/准确受控运行session/no stop/no fault，RPC无值、任意CDP或启动策略覆盖。policy为merge默认或明确replace-all；accepted不代表写入。schema5无新表，requests只保存元数据签名；operations/activities只保存安全结果。
- [`私有pipe`](../internal/kernel/cookies_windows.go)固定Storage.get/set/clearCookies作用于原自有浏览器default context；每一条先读同键，已匹配不重写，否则单条写后完整读回。整个组合持commandGate，未将应用sessionId当CDPcontext，不加URL改变secure。匹配包括真实值/安全属性/SameSite缺省/session/秒expiry/分区；写入未知或读回差异不计verifiedCount。
- JSON-1/省略expiry为session，JSON0是过期epoch；Netscape0/-1为session。空value合法、过期跳过不续期，未知字段/opaque分区明确错误。分区两成员含false保留，读回opaque字段即使false也不能当普通Cookie。仅接受内核可原样保存的ASCII规范path，拒点段/百分号/URL分隔/空白等；拒双前导点/私有PSL域Cookie/非规范numericURLhost，IPv6Cookie域本票暂不支持。sourceScheme/secure冲突和除-1外负expiry拒绝，不写后才发现改了另一键。
- [`任务/观测`](../internal/workspace/cookie_worker.go)2分钟、每命令10s；取消保已核对/未知条目，清空须读回空集合；同preview全量清空只受理一次、失败重试合并且先核对。重开未终APPLICATION_INTERRUPTED/unknown不重放秘密；终态与活动同事务，写失败保Cookie lease只重试保存，Runtime退出/核对不能提前释放。
- [`native导入`](../src/components/NativeCookieImport.tsx)显示目标/统计/域名/hostOnly/分区/逐项结果，raw输入默认隐藏、严格UTF-8文件、预览后清空。需要启动时先明确调用原Runtime.Start：`purpose: cookie-import`与expectedRevision仅抑制本次恢复标签/URLs，不改保存配置/身份；direct确认和proxy门禁保留。关闭后仍可接管迟到任务/取消；新draft不混历史终态报告，CookieReport保存previewId，重试只接同preview的所选失败行。失败新attempt使用新requestId，受理未知才重发旧ID。
- 14条解析/匹配、2条内核合成pipe、8条服务、3条adapter回归仅编写未运行，Go测试包未编译。固定二进制行为/持久化/A-B隔离/新页面仍未验证，[T12清单](verification/T12.md)、[D013](DECISIONS.md#d013--cookie命令只作用于指定会话重试先核对同键2026-10-01)。原型Demo继续旧行为，native不存示例Cookie。

下表为**完整目标本地服务契约**；T01 环境创建编辑先导见 [contract.ts](../src/application/contract.ts) 与 [demo-adapter.ts](../src/application/demo-adapter.ts)。T02 经 [WailsAdapter](../src/application/wails-adapter.ts) 调 [Go 服务](../internal/workspace/service.go)，绑定负责 UI 与本机服务通信，不开放 HTTP 管理服务。

T02 已实现的原生 RPC 为 `Workspace.Read`、`Environment.Preview`、`Preview.Regenerate/Discard`、单条 `Environment.Create`、`Environment.Update`、`Operation.Read/Cancel`（本票操作为同步已提交终态，取消不回滚完成项）。SQLite schema v1 保存环境/固定档案、内核/代理引用、revision、成功请求去重结果、终态操作与活动；这不是长任务恢复实现。请求仅接受 `mode: native`，配置按白名单转换，预览由当前服务会话管理；成功请求缓存则跨服务重开持久保存。数据库的新建/迁移在事务中提交，未知版本、缺失 schema 和坏配置不自动覆盖或重置。

原生初始 `kernel-pending` 仅代表未安装、未就绪；无真实版本或可执行文件校验声明，不能启动。桌面初始无示例环境/代理，不读取或导入原型 localStorage/JSON；后续内核安装、生成器修订、真实浏览数据和批量任务按 T04/T05/T06/T13 验收。UI 投影的 schemaVersion 1 不是生产导入格式，不得据此直接复制原型对象到数据库。

T04 增量接口为 `Kernel.SelectArchive`（系统文件选择器→当前会话token，不接任意路径）、`Kernel.Install`（来源/精确版本/预期摘要/token/可信确认/requestId→受理操作）、`Kernel.List`、`Kernel.Verify` 与 `Kernel.Delete`（后两者接精确kernelId/requestId→受理操作）。Windows内核维护单任务并发仅作资源/维护保护，不限制安装数量。下载/解包/真实探测在worker中运行，`Operation.Read/Cancel`可查询/取消；页面以持久服务状态为准，重开不自动重装或换版本。

SQLite schema v2 事务新增 `kernel_evidence`，保存来源、tag、源码commit或null、架构、归档/主程序/完整文件清单摘要、内部相对位置及带adapter/能力版本和会话时间的真实报告。v1迁移保留既有ID/seed/配置和pending引用；同一证据不允许UPDATE。新安装采用同卷暂存/边界检查/摘要与PE/CDP验证，再分配新ID发布，登记内核与完成操作同一事务提交；失败清理本次资源，持久日志支持重开时清理已分配的未提交暂存/发布目录，不触碰无关目录。重新核验不把损坏字节重算成可信摘要；被当前或历史档案引用的构建不能直接移除。正常环境启动和批量任务恢复仍分别留T06/T13。

### T05 固定档案增量（已实现，完整验收待补）

- `Fingerprint.Generate({ previewId, kernelId, templateId, overrides: { language, timezone, cpu, width, height }, regenerate })` 从服务会话的预览生成Windows固定输入、能力报告和相对当前档案的变化。默认保留 seed；`regenerate: true` 显式生成新 seed。不启动内核、不写数据库、不改变环境修订。只接受白名单字段，拒绝客户端提供参数、GPU读值或完整档案。
- `Fingerprint.CommitRevision({ environmentId, previewId, profileHash, configuration, expectedRevision, requestId })` 校验服务预览、摘要、当前环境修订/原预览基线和精确内核证据，事务写当前引用、不可变历史、请求结果、操作与活动；返回 `newRevision`（环境修订）及 `fingerprintRevision`（档案修订）。普通名称/备注/代理等保存不增加档案修订，也不换 seed。旧 `Environment.Create/Update` 同样校验，不能绕过生成流程；`kernel-pending` 仅保留待绑定配置兼容，不可启动。
- `Fingerprint.ListRevisions({ environmentId })` 返回倒序历史；`Fingerprint.PreviewRestore({ previewId, revision })` 只预览同一精确内核的旧设备输入。保存后追加新的单调档案修订，不倒退编号、不恢复旧名称/代理、不清空浏览数据；pending或跨内核恢复拒绝。
- SQLite schema3增加 `fingerprint_revisions`、当前档案 `config_revision` 和环境 `user_data_ref`。历史禁止UPDATE，保持原 `fingerprint_id`，避免为历史复制受当前seed唯一约束的行。v1/v2升级事务保留原ID、seed、展开字段和旧生成器版本；未知版本/坏输入阻断，不悄悄重生成。管理引用为 `environments/<environment-id>/user-data`，本票只建立引用、不自动创建或打开真实浏览目录。
- 固定档案保存模板/生成器/独立档案schema版本、语言顺序、明确IANA时区、CPU/窗口偏好、真实内核身份/摘要、适配器/能力版本和能力编译的参数。空时区和 `Local` 拒绝。规范化SHA-256基于固定字段顺序的JSON（`configHash`置空）；包含档案修订，不包含名称、代理和网页Cookie。
- host-only `AcquireProfileUse` 是供T06监督器复用的忙租约，持有期间拒绝关键档案及代理修改；不通过RPC接收伪造running/ready。T05自身未实现正常启动，不能把此租约的契约测试说成真实运行验收。
- Demo仅在 `_application.fingerprintRevisions` 记录演示历史，checksum带 `demo-`，无真实内核证据；localStorage写成功才发布。`prism-prototype` schemaVersion 1不增加必需字段，旧记录可读。外层/嵌套模式均与Wails隔离。[实际测试与剩余边界](verification/T05.md)。

### T06 真实会话增量（已实现，尚未运行验收）

- `Runtime.Start({ environmentId, requestId, networkPolicy: "direct" })` 只消费已保存配置。要求当前 `proxyId==""`，且每次启动由用户明确确认直连；已有代理绑定返回 `PROXY_UNSUPPORTED`，不能由直连确认覆盖。客户端路径、参数、PID、状态和临时档案一律拒绝。`requestId`与受理操作同事务登记，重复启动返回原operation；昂贵启动一次一条只是资源门控，不限制环境/保持运行数量。
- worker读取固定档案/构建证据、规范化独立引用，kernel核对实际文件清单与PE，取得环境目录独占锁。目录链拒绝WRITE/DELETE访问、防原地reparse转换，句柄属性复核；既有浏览文件的hardlink共享拒绝。元数据记录environmentId/sessionId/PID/创建时间，锁不按年龄删除；内核文件pins和根目录锁持有至本次Job的全部进程退出。浏览文件本身仍可正常写入/重命名，不声称防同用户运行期恶意新增文件别名。窗口偏好与参数来自保存档案，不生成新seed，实际目录为 `environments/<uuid>/user-data`，停止/启动失败不删除浏览数据。
- 默认可见浏览器，不带headless/incognito/no-sandbox或探测死代理；直连显式 `--no-proxy-server`。身份/seed/地区/CPU由能力编译器输出，私有pipe只继承两条限定句柄、进程原子归属本次Job；不开放调试TCP、通用CDP RPC或完整路径。Browser.getVersion和page target可响应、固定字节复核完成且主进程仍活着才发布running；主进程存活与Job资源退出信号分开，受理和创建进程均不等于就绪。
- `Runtime.Stop({ environmentId, requestId })` 正常Browser.close并等待Job全树退出；pending启动则取消本次准备。重复停止复用原任务，等待退出超时保持明确error与忙锁、不擅自强杀其他进程；控制写端永久断开则单独返回不可正常重试的 `CONTROL_CHANNEL_LOST`，不虚报“可重试停止”。仅失败启动/应用退出使用该会话的自有Job清理；无法确认清空不得释放租约。应用关闭在服务mutex外停止会话，所有Close调用共享真实完成信号；单次等待超时不放弃后台清理，进程资源退出后关库，再次等待可得到真正完成结果。复杂崩溃/重开/强制选择属于T07。
- `Runtime.Inspect({ ids })`及Workspace投影只返回native的环境/会话ID、实际状态、保存修订、相对数据引用、PID与创建时间等安全字段；不返回pipe端点、任意控制方法或真实Cookie。Wails/App按真实查询刷新，不从模拟兼容层写running。活跃会话只能保存名称、分组、备注，提交响应同样投影实际运行状态；关键配置/启动偏好仍拒绝，事务中还须确认完整档案hash不变，同输入的旧回滚预览也不能在忙状态追加修订。原型旧模拟流程保留。
- [生命周期回归](../internal/workspace/runtime_test.go)、[目录锁回归](../internal/kernel/profile_lock_windows_test.go)及[实际A/B存储用例](../internal/workspace/runtime_real_test.go)已编写未运行，代码存在不代表验收通过。真实用例需要明确 `PRISM_RUNTIME_VERIFY=1`，会开正常窗口，当前不自动执行；完整状态见 [T06](verification/T06.md)。

### T07 异常监督与恢复增量（已实现，未运行验收）

- SQLite schema4新增 `runtime_sessions` / `runtime_events`，旧档案与数据引用不变；启动、停止、强制结束、核对以environmentId/sessionId关联。启动受理、进入创建阶段、实际PID/创建时间、最终就绪分开持久记录。进入启动阶段的PID0必须核对目录元数据，不解释为从未创建进程。
- 就绪超时、浏览器崩溃、控制通道丢失及停止超时分开投影；退出码仅来自实际进程观察。根退出不等于Job全树退出，退出未确认或存储失败仍保护档案和目录。启动失败主动清理记录意图，不覆盖原始错误为crash；停止结果迟到须匹配当前slot，持久UPDATE还比较sessionId，不能释放后来新会话的租约。
- 会话/任务终态/脱敏活动同事务提交。失败保留 `persistencePending` 与待落盘终态，在后续查询重试持久化，不重复外部启停动作；前序仍有待写状态时拒绝新受理，不能用旧ready覆盖新核对租约。临时失败操作不是不可变终态，前端在保存恢复后可接真实完成，真正终态仍不回退。关闭等待超时不弃后台清理，未保存观察在退出结果明确报告。
- 重开核对PID+创建时间+实际session元数据+独占目录锁，以及带资源版本的确切Job。正常Job是当前SID/SYSTEM限定的全局session标识、句柄不继承；重开只取QUERY权限，资源ActiveProcesses==0或对象已销毁才算树退出，不能由主进程死/应用锁已释放推导子树退出。匿名pipe不跨应用重建；原身份仍存活、锁占用、资源版本缺失或核对未知时 `needsReconcile=true`，保留关键配置保护。只有原进程退出/ctime复用、树资源退出且目录空闲、元数据对应才允许原档案重试；PID复用不取得控制权。[D008](DECISIONS.md#d008--重开只核对不按pid接管2026-10-01)。
- `Runtime.Reconcile({ environmentId, sessionId, requestId })`只检查保存身份，锁外执行进程/目录I/O，晚返回前核对会话。`Runtime.ForceStop`同样只收这三个ID，普通Stop失败且当前持有确切Job才可受理；不能按客户端PID/名称强杀。查询返回 `canControl/canForce/needsReconcile/nextAction/reconciledAt/lastExitCode` 等安全字段，无控制通道或Cookie。
- Wails/App的表格与活动提供明确核对/指定会话结束，强制动作再次确认且不由批量关闭隐式升级；旧活动须匹配当前sessionId。待核对无PID或待存储结果仍锁关键配置，仅名称/分组/备注可保存。术语见[领域用语](../GLOSSARY.md)，未运行证据见[T07](verification/T07.md)。
- A/B真实用例的故障部分另需 `PRISM_RECOVERY_VERIFY=1`，会实际打开窗口并终止自身创建的准确根进程。证据区分“实际用到指定Job强制结束”与“根/Job自行退出”，不涵盖应用自身崩溃；当前暂停期不执行、不自动开关。

当前 ApplicationService 统一 `{ ok, mode, data/error, operationId? }`、预览 ID、requestId、expectedRevision 与 Operation 事件。创建返回 `status: accepted`，需查询/事件确认 `completed/cancelled/failed`；编辑仅在存储写入成功后返回 `status: completed`。只提交配置白名单，不能从草稿修改 ID、Cookie 或运行状态。UI 按操作 ID、模式、序号和终态过滤迟到事件。demo 的预览、幂等请求缓存和 Operation 在当前服务会话有效，不宣称任务重开续作；revision 用额外存储元数据持久保存，旧 v1 演示记录可显式读取且快照类型不变。其他页经 demo-only compatibility 逐步接入；native 不可使用该兼容入口。

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
| Runtime.Start              | environmentId、requestId、明确networkPolicy → operationId             | ENV-003          |
| Runtime.Stop               | environmentId、requestId → operationId；服务固定正常退出时限           | ENV-003          |
| Runtime.Inspect            | ids → observed runtime sessions                                        | ENV-003          |
| Runtime.ForceStop          | environmentId、sessionId、requestId → operationId；普通关闭失败后       | ENV-003、DATA-001 |
| Runtime.Reconcile          | environmentId、sessionId、requestId → operationId；真实身份及锁核对     | ENV-003、DATA-001 |
| Proxy.ParseImport          | text → previewId、原行号/安全配置/错误、共享重复组                      | PRX-001          |
| Proxy.DiscardImport        | previewId → discarded；释放秘密草稿                                     | PRX-001、UX-001  |
| Proxy.CommitImport         | previewId、selectedRows、requestId → importedIds、importedLines       | PRX-001          |
| Proxy.Update               | proxyId、expectedRevision、configuration、credentials、requestId → record | PRX-001       |
| Proxy.Delete               | proxyId、expectedRevision、requestId → deletedId；引用保护              | PRX-001          |
| Proxy.Check                | proxyId、expectedRevision、requestId → operation；分阶段/实际出口      | PRX-001          |
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

当前快照等旧页经 DemoAdapter compatibility 更新，localStorage 写入成功后才发布新状态；写入失败保留旧状态，仍不能声称具有 SQLite 级事务保证。后续桌面版须完成持久事务和目录一致性后发布成功；其完整记录/引用/摘要校验以以下真实备份要求为准。

原型还保存上一次已读取/写入的存储文本用于检测跨标签页更新。存储已被其他页面改动时停止覆盖并要求重载。初始化遇到损坏原文时保留该文本，阻止自动写回，用户可导出诊断副本或明确重置；此原文导出不是可直接恢复的有效快照，不能更改其格式标识来绕过校验。

### 桌面备份

#### T15 原生完整导出（本地已实现待验收）

- `Backup.SelectDestination({})`调用host保存对话框，返回30分钟token/文件basename；取消不受理。`Backup.Export({scope:all|selected,environmentIds,destinationToken,stopRunning,requestId})`只接受明确范围/ID和token；all须空ID数组，由数据库确定全范围而非当前页，selected缺失ID阻断。明确允许正常停止时调用原`Runtime.Stop`，不按PID杀进程/不自动ForceStop，待核对/维护/Cookie任务先处理；备份owner预约保护范围，停止中不能重启。
- `backup_exports`新增schema7，私有destination/temporary/targets/publishing日志关联操作；普通report只含native独立格式/安全requestId/统计/sequence/发布事实/摘要。请求和受理同事务，未知COMMIT将原请求、操作、发布日志的身份/范围/输出同时核对才调度一次；临时观测重读只保存、不重做浏览目录或输出副作用。操作状态与文件发布分开，只有完整核对并正式rename才published。重开先完成全部可失败同步恢复和map初始化，再在锁内启动只读摘要核对worker。worker锁外只使用锁内冻结的`backupExecution`，不读取被取消整体替换的operation。
- 一致配置通过[`snapshot`](../internal/workspace/backup_snapshot.go)的独立只读WAL连接、显式读事务、SQLite online Backup API每64页检查取消；服务唯一连接可继续取消。离线副本保全schema/索引/触发器，过滤明确环境，selected取绑定代理及当前/历史档案的精确内核闭包，凭据原ref+DPAPI字节不解密、不换引用。VACUUM清未选记录freelist、integrity/外键/非FK凭据引用/当前及历史档案核对。全量保未使用代理/内核元数据；两种范围均不恢复会话、任务/活动、混合批次、备份历史、请求去重key，只保配置及档案修订。
- `.prismbackup`为`prism-local-backup`/schemaVersion1 ZIP，内含`configuration.sqlite`与规范引用下真实user-data-dir和空目录；manifest带workspaceSchema7/appVersion、原ID/seed/配置与档案修订/hash/模板/生成器、环境/代理/内核关系、精确版本/archive/executable hash与每文件长度SHA256。不携带内核本体，pending明确未绑定无捏造hash。不是prism-prototype JSON，不包含任何恢复执行授权。
- [`CaptureProfile`](../internal/backup/profile_windows.go)只读已有目录/锁，父到子固定路径对象，每个源文件READ/shareREAD至最终集合/对象复核；拒reparse/硬链接/变化，正常停止且持久成功后才复制。未初始化目录不为备份创建，有使用/准备证据却丢失目录失败。浏览数据可能含敏感Cookie/网站信息；DPAPI保护代理不等于整包加密，恢复仅原Windows用户及密钥上下文，不承诺重装/跨机器登录。
- [`初始化事实`](../internal/workspace/data_initialization.go)独立`environment_data_state`：新建/批次与环境事务分别保存never-initialized/directory-prepared；运行通过代理门禁/通道准备后、launcher之前提交runtime-claimed，含不明COMMIT时不调用launcher，失败不降级。schema6旧环境迁移legacy-unconfirmed，不依赖最新session或缺history宣称从未初始化。单调触发器禁止重置；备份停止后按冻结ID重读并与一致副本事实核对。仅never允许源目录不存在，directory-prepared允许没有运行锁，其余缺目录/锁保持失败；正常备份不会创建源目录掩盖未知。后票T17复制真实present数据时须将恢复事实至少变成directory-prepared。
- 输出需工作区外本机新文件，不覆盖已有目标；[`Output`](../internal/backup/output_windows.go)同目录`.partial`、关闭ZIP/Sync/原句柄逐项读回与SHA核对、持久publishing/hash后按原文件句柄不覆盖rename。最终短临界区核对cancel；发布后迟到cancel不得回退真实结果，DB失败保published事实/只保存观测；重开不重复制/rename，只核对发布日志对应正式文件原hash。
- [`NativeBackupManager`](../src/components/NativeBackupManager.tsx)全量与环境表明确选定入口、正常停止确认、host文件选择/取消/历史/摘要；viewId/响应式读取代次确保取消前目标匹配且同ID重读不失去轮询。Wails保存会话内未决原请求，跨页面/查看无关历史不丢弃；仅原requestId的已核实报告或服务明确未受理才释放，报告/模式校验失败保已知ID与原请求，真实刷新可恢复原受理。异步迟到响应须仍持有原pending对象，不能修改新请求；页面read/poll/refresh核实原受理时统一消费旧输出授权，只消费一次，无关历史不清掉新输出。demo旧JSON保持独立，预检/正式恢复仍T16/T17。7文件/包+19服务+9adapter回归仅编写未执行，Go测试包未编译；17:44生产static/源码测试TS通过，17:47文档/格式通过，文件、后端、UI最终只读均无剩余可信P1/P2，不等同运行证据，[D015](DECISIONS.md#d015--一致本机备份范围闭包与先核对再发布2026-10-01)、[验收清单](verification/T15.md)。

1. 计算范围并取得维护任务锁，阻止相关环境在备份期间重新启动。
2. 正常停止环境，等待所有受控进程退出及目录锁释放。失败时结束任务，不输出完整成功包。
3. 使用 SQLite 一致性备份方式取得数据库快照；不要在 WAL 活跃时只复制 app.db。
4. 将档案、凭据备份材料、user-data-dir 和内核清单写入临时包，记录文件相对路径、长度及 SHA-256。
5. 写入清单并校验包内容，完成后将临时文件重命名为最终包。失败包保留为明确的临时/失败状态，不能出现在“可恢复成功备份”列表中。

浏览器数据可能包含 Windows 用户绑定的加密信息。首个桌面验收范围先确保同一 Windows 用户上下文中的本软件恢复；重装系统、跨电脑或其他用户账户的登录状态恢复，必须另做显式支持和验证，不由“目录复制成功”推断。

### T17 完整恢复执行增量（本地源码，尚未运行验收）

[`受理`](../internal/workspace/restore_accept.go)消费T16预览及原摘要，冻结包/环境/历史/凭据闭包；全局配置维护屏障、正常Stop及持久观测通过后，重新核对基线与精确内核文件。[`worker`](../internal/workspace/restore_worker.go)使用同卷incoming/previous、旧配置副本和[`稳定目录对象`](../internal/backup/switch_windows.go)，持久prepared计划先于首次rename；目录校验和移动只处理已知对象，未知目录保持保护。取消不自动重新打开已停止会话。

[`schema8恢复日志`](../internal/workspace/restore_storage.go)与[`配置事务`](../internal/workspace/restore_commit.go)保持同一app.db，不用包内SQLite覆盖本机库。保留原ID/seed/档案历史，全部历史只做已核对的精确kernel ID映射及hash重算；环境配置revision推进防旧批次ABA，包外记录保留。实际安装目录至少directory-prepared，旧runtime-claimed/legacy-unconfirmed不降级；不导入旧运行控制身份。代理原ref/DPAPI密文保持，不可解密须重输；未变包外共享代理不改修订或当前通道。native v1包仍schema7，导出剔除本机restore_jobs。

配置与`committed=1`同事务。未提交反向还原旧目录；已提交核对新目录；未知标记或目录冲突保持维护，不发布成功。已观测终态仅日志写失败时，Workspace/Operation读取重试原结果保存、不重做目录动作。受理整体确认不存在才释放原request占位；未知继续查询。重开当前只加载保护，自动收敛留T18。native[`恢复界面`](../src/components/NativeRestoreExecution.tsx)提供覆盖/停止/凭据确认与原请求核实、任务取消/历史，实际验证见[T17待验收](verification/T17.md)。

### T18 启动恢复增量（开发中，尚未运行验收）

[`启动恢复`](../internal/workspace/restore_recovery.go)在其他worker/环境操作之前加载唯一未完成journal，校验受理关联、内部版本2、原身份/规范路径/目录对象/清单；[`配置摘要`](../internal/workspace/restore_consistency.go)只覆盖本次可能改动的环境/档案历史/初始化事实/代理与密文引用。旧摘要和原配置副本SHA在prepared前持久，新摘要与`committed=1`同事务；恢复先确认DB标记与对应配置匹配，再核对实际目录决定回滚旧侧或确认新侧，不读取原包或重做导入。

启动期间保持原restoreTask屏障，目录结果确认后先恢复其余kernel/runtime/proxy/Cookie/batch/backup控制记录再开放操作；bootstrap前不持久化终态，独立内存容器锁外加载后发布，失败保留已完成步骤并明确重试。会话核对按持久初始化事实只读既有目录/锁，不创建已回滚为空的目录；Close可取消启动清理。已核对终态仅保存失败时只重试原观测，普通重试不复用过时finalPending去覆盖新保护。原生页面显示recoveredAfterRestart/interruptedStage及保留数据的修复步骤。5阶段与回滚再中断子进程Kill、真实浏览器组合用例仅编写，见[T18清单](verification/T18.md)。

### T19 本机回收增量（本地源码，未运行验收）

schema9保留原环境/档案行，以`environment_trash`标记回收成员，`recycle_jobs`保存原请求、计划SHA、逐项prepared/committed及配置前后摘要。正常环境列表/统计/业务来源和全量备份排除回收项；名称、编号、身份及共享内核/代理引用继续保留。移入/找回推进环境revision，保持seed、档案revision/hash与规范数据引用，旧编辑/批次不跨往返复用。

[`回收服务`](../internal/workspace/recycle_api.go)接明确ID的影响预览和分页，确认仅收原previewId/requestId。已停止且退出/持久化已确认后，[`目录worker`](../internal/workspace/recycle_worker.go)固定原树身份/清单，prepared日志先于移动，成员和逐项committed同配置事务；未知提交先重读原日志，不用内存旧决策覆写。部分失败保已完成项，未提交移动回原侧；原目录冲突不覆盖、不调用克隆。

永久删除只接明确回收项，保存purge开始授权后才执行已验证句柄删除；首次删除前检查全树与原运行锁的对象身份/摘要，拒绝未知、变化、重解析、硬链接、占用和只读文件。授权前取消保整项，部分删除后收敛当前项再停后续项；delete-pending尚未消失不能提交删除配置。移除停止会话控制行，历史操作/活动和既有备份保留；不删除共享内核/代理，不声称擦除所有历史介质字节。

[`启动恢复`](../internal/workspace/recycle_recovery.go)检查唯一目录writer、完整决策字段、原受理和配置摘要；restore/recycle未完成日志并存时保护。目录结果后锁外加载其余启动记录，再落终态/解除维护；未加载会话不报告已停止。终态写失败仅重试保存原观测。native包仍v1/schema7，排除回收环境及本机日志；新restore journalVersion3摘要包括回收成员，v2算法保留。原生[`回收界面`](../src/components/NativeRecycleManager.tsx)接移除/找回/永久删除/原请求核实，记录与[待验收入口](verification/T19.md)均区分源码和实际结果。

### T20 内核迁移与升级前完整恢复（本地源码，未运行验收）

schema10增加默认构建与独立迁移日志/保留引用；[`默认选择`](../internal/workspace/kernel_default.go)按expectedRevision/requestId保存，仅编译之后新建草稿，旧环境/旧草稿不变。当前/历史档案、默认和全部迁移备份历史构建共同阻止删除。native v1备份仍schema7，排除本机默认与控制日志。

[`迁移接口`](../internal/workspace/migration_api.go)为`Migration.Preview/Prepare/Action/SelectRollback`；一次明确选一个已停止环境，冻结原配置摘要、seed及两精确构建。独立kind `migration`及全局维护屏障贯穿[`完整备份/工作副本`](../internal/workspace/migration_backup.go)、[`旧新试用`](../internal/workspace/migration_trial.go)和[`明确切换`](../internal/workspace/migration_commit.go)，不复用普通克隆身份或嵌套restore任务。旧/新参数及能力差异可查看，副本诊断包含实际指纹与同origin持久合成存储；正常停止要求准确全树及已知0退出码。

prepared保存目录对象/清单，旧目录留previous，试用目录移到原规范引用；配置与committed/新摘要同事务。任何提交返回均读持久决策，不用旧计划补偿未知结果。[`启动恢复`](../internal/workspace/migration_recovery.go)先核对唯一writer，选择完整旧/新侧，加载其余启动记录后才保存终态；失败保屏障并支持重试，不把中断试用当授权。兼容问题通过绑定原SHA的短期token走正式`Backup.PreviewRestore/ApplyRestore`，完整撤回备份后变化。

[`原生迁移页`](../src/components/NativeMigrationManager.tsx)独立25条查找与原请求核实；adapter保默认/迁移pending，回滚离页清理不能废弃未决恢复，同请求在途恢复Promise合并。代理绑定环境仍保T11门禁，待全路径隔离及试用路由集成。[测试入口与未验收边界](verification/T20.md)分别记录合成服务、Process.Kill与两不同真实构建；没有运行结果。

### 原子替换与崩溃恢复

#### T16只读预检增量（源码已完成，未运行验收）

`Backup.SelectRestoreSource`经host选择器返回短期token；`Backup.PreviewRestore({sourceToken})`只读验证原生包，返回短期previewId/包与清单摘要/安全统计/精确内核及凭据范围。`Backup.ReadRestorePage({previewId,offset,pageSize})`最多100条影响，`Backup.DiscardRestore({previewId,sourceToken})`取消指定读取/丢弃预览。它们在所有持久任务flush之前分发，不调用Workspace.Read或Kernel.Verify，不停止会话。私有暂存仅配置副本，当前app.db及浏览数据不写入。

native v1/schema7未知字段、重复/大小写JSON键、缺少必需字段/null及不支持版本严格拒绝；ZIP逐字节流式核对CRC/SHA/长度和清单闭包，路径/链接/父子冲突拒绝。清单32MiB、配置256MiB、元数据50万项上限是解析资源边界，超限准确拒绝，非环境保存配额。可信schema在独立内存库生成比对，不执行包提供的DDL；全部当前/历史档案与密文引用校验。完整影响绑定当前配置基线，提交前必须重验，详见[T16](verification/T16.md)。

原ID/seed/参数保留；同版本、架构、archive/executable/full file hashes及能力相符的本机精确内核可映射内部ID，pending不补造证据。只读文件/PE核对不是实际新会话探测。代理原ref+DPAPI解密仅短期内存，不可用报告重输，不称跨Windows用户/机器登录便携。

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
| BACKUP_OUTPUT_UNAVAILABLE / BACKUP_EXPORT_FAILED | 输出选择或完整导出未全部成功 | 核对正常停止、原数据目录、权限及空间；临时包不是完成包 |
| BACKUP_ACCEPTANCE_UNCONFIRMED / BACKUP_RESULT_UNCONFIRMED | 导出受理或响应尚未核实 | 只核实原请求或已知任务ID，不另建导出、不冒称未受理 |
| BACKUP_PUBLICATION_UNCONFIRMED | 退出后的正式文件未按原摘要确认 | 保留原工作区与现有文件，查看原任务；不自动再次发布 |
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

存储验证覆盖损坏原文不被覆盖、阻断界面可导出原始记录及显式重置，以及跨标签页更新后旧页被阻断并要求重载。存储写入失败检查“本次修改未保存”反馈及旧状态不变，不能按原型测试结果宣称生产级回滚已通过。生成器/档案历史回滚、批量复制、批量代理分配、选定环境备份和回收区仅在对应桌面阶段验收。

## 13 公开仓库与资料引用

仅提交本项目独立编写的源码、文档及明确许可的依赖声明。真实 user-data-dir、日志、代理凭据、Cookie、备份、内核二进制和旧发布归档均不应进入 Git；配置模板只放占位数据。

可引用资料：

- [fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium)：已选定内核，实际发布遵守其许可证及依赖要求；源码可用性以固定 tag 核实。
- [Ant-Browser](https://github.com/black-ant/Ant-Browser)：结构与功能参考，独立实现，不能把公开可读源码等同于已取得复制授权。
- [Wails 官方文档](https://wails.io/docs/introduction/)：未来 Go 与 Web 前端桌面桥接。
- [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/)：真实浏览器协议能力按固定内核核对。
- 旧材料文件 `02-原始主进程与预加载代码/readable-7.1.5/main/electron-main.js`、`preload/electron-preload.js`：只作流程研究，不收入本仓库。尤其不搬其云签名流程、Cookie 强制续期、吞错返回成功和专用内核配置协议。

每次新增真实能力应同步 [PRD 验收矩阵](PRD.md#7-验收矩阵) 与首页交付说明，写明验证条件、实际结果和仍未验证的边界。
