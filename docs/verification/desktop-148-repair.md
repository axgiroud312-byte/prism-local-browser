# 148 桌面恢复修复与真实验证（2026-10-07）

后续修复：用户要求采用 Ant-Browser 的共享内核方式，新 `95a600f` / preview10 已取消导致本页故障的额外共享权限操作，并通过限定真实多开/重开、代理故障恢复和退出重启验证，见[最新结果](desktop-148-isolation-fix.md)。下文保留 preview9 的原始失败与当时结论，不改挂新程序来源。

用户取消1:1要求；当前只使用 fingerprint-chromium **148.0.7778.215**，不验证 150 或跨版本迁移，不限制独立环境数量。本轮沿原集成分支继续，保留 `8688906`、PR #31 精确 `e170099bc25aa15ecb9a8456d782394a2139ec20` 祖先及用户改动。旧商业参考、失败与原检查来源保留；没有扩展截图或图库审计。

**已修复普通备份恢复的来源丢失/清理重试，以及退出时清理未完成的反馈与安全阻断。新程序的真实操作已验证这些修复。整体仍未完成：148 在本轮普通打开和 Cookie 专用启动发生崩溃，后续真实单变量对照已定位到本程序共享内核的代理 package ACL 方案触发 GPU 拒绝访问，具体加载对象与可行修复仍在调查。** 单独重开成功不抹去并发时的失败，不把现有数据保护称为内核修复，也不再将这个问题笼统归咎于上游。

## 程序与操作范围

| 属性 | 核实结果 |
| --- | --- |
| 本机程序 | `build/bin/prism-browser.exe`；直接运行的 Windows amd64 Wails production 开发预览 |
| 应用版本 | `0.3.0-preview.9`；实际窗口核实 |
| 源码 | `490f198f97a4e568054ca49dd87071059d399b4c`，tree `0205e6a26aa0ba2ac730efb0ad4128889c312ab1` |
| SHA-256 / 大小 | `b62caaa5929435b2ec3186d53713cc3f7aa554c44a3a2caf35aa89fb952586f7` / 21,045,760 bytes |
| 构建 | `npm run build:windows -- -PreviewRevision 9`；51.12 秒；Go 1.27.1 / Wails 2.16.0 |
| 分发边界 | 未签名、未生成新安装器；不复用旧 release manifest。伴随许可通知保留。 |
| 内核 | 精确 148.0.7778.215；实际 PE 与受控进程读值核实；exe SHA-256 `1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915` |
| 数据库 | 工作台配置/操作使用本机 SQLite `app.db`；浏览器 Cookie、LocalStorage、IndexedDB 分别位于每个环境自己的 profile，不集中保存到工作台数据库。 |

仅使用带 `synthetic-only` 标记的隔离工作区、合成环境与 Cookie。当前 Clash 的本机 `127.0.0.1:7897` 被用作实际代理，未读取凭据、切换节点、修改配置或停止 Clash。受控故障只影响本任务自己的代理 helper 或准确预检文件句柄。程序没有使用模拟 bridge；可控故障的注入方式明确列出。

必要原始证据位于 ignored `output/goal/desktop-148-repair/`。后续报告提交不改变本次 exe 的构建来源。

## 已修问题及实际验证

| 范围 | 判定 | 真实行为和边界 |
| --- | --- | --- |
| 普通恢复隐藏/返回后的准确来源与清理恢复 | 实际通过 | 系统选择器选入合成完整备份；持有准确 scratch 的文件句柄导致真实清理失败。取消失败后新选择被阻断；隐藏并离开/返回页面，查回原来源及清理状态。释放原句柄后只重试原清理，确认该 scratch 不存在后才能继续。 |
| 退出清理失败反馈、留窗口与重试 | 实际通过 | 同类受控文件占用下关窗，显示原生“退出清理尚未完成”；选择留窗口后新业务调用被 `NATIVE_UNAVAILABLE` 拒绝。释放占用后正常退出码 0，原 scratch 已清除；新程序重开成功。 |
| 连续丢包、选择回复迟到、数据库保存/关闭失败 | 仍缺对应真实桌面故障证据 | 定向服务、adapter 与页面自动化通过，不能据此填为真实 Windows 故障验收通过。准确原请求/资源清理未确认时仍阻断。 |
| 无原 requestId，或整个原服务进程异常终结后的未知来源 | 不支持，仍阻断 | 不扫描认领任意 scratch、不猜来源或制造 token。当前 owner 仅在原 Service 生命周期有效；没有新增未知残留自动恢复功能。 |

实现及必要定向检查详见[普通恢复来源修复](desktop-restore-source-closeout.md)与[退出清理修复](desktop-shutdown-closeout.md)。实际 Windows 文件占用是在新程序上触发的受控故障，不是自然 IPC 丢失证据；两者没有混写。

## 五类缺口与关键流程

| 项目 | 判定 | 本轮证据 |
| --- | --- | --- |
| 真实批次取消与未执行反馈 | 实际通过 | native `Batch` 的 128 项计划取消后为 21 完成、0 失败、107 未执行。只创建 21 项；另一个 prepared journal 没有环境记录，不计成功。 |
| 同一批次重试 | 实际通过 | 原计划继续，只补 107 项，最后 128 完成；原 21 项及已预分配身份不变，没有重复创建。历史 preview8 的自然名称冲突 2 成功/1 失败与仅续做失败项结果保留原来源。 |
| 创建后打开失败的重试 | 实际通过 | 新环境已保存，关闭受控代理使打开返回 `PROXY_UNREACHABLE`；恢复该代理后仅重试原环境 ID，准确 148 启动并读回真实数据，没有再次创建。 |
| native BatchReport 的 `skipped` / 单条 Create 多项部分完成 | 不适用，有契约依据 | 实际批次报告 completed/failed/notExecuted；单条 Create 只创建一个环境。Create→Batch 移交不算单条 Create 部分完成。CookieReport 另有 skippedCount，不把 Batch 结论扩成所有 native 接口都不存在 skipped。 |
| 自动指纹、普通编辑/关闭重开、运行中关键字段保护 | 实际通过（所列操作） | Direct 原 seed `1600672548`、148 绑定与数据引用保持；运行中关键输入禁用，普通备注保存不改身份；换一套草稿被放弃后重开仍为原保存 seed。 |
| native 连续生成等待、在途关闭/迟到回复 | 仍缺真实时序证据 | 本次未捕获自然生成中的在途状态；UI Invoke 返回快不能证明 RPC 已结束或解释未捕获原因。未强行修改 demo 或生产生成时序。自动化 guard 证据保留，不能当桌面通过。 |
| 回收历史 | 历史实际通过，本次未重复 | preview7 使用正确 `Recycle.ReadPage` 核对历史、回收/恢复及三存储；不是 Operation.Read 顶替。本次仅沿用原来源，未重标 preview9。 |
| 迁移丢失来源与跨版本副本 | 当前跨版本操作不适用；未知来源保护继续有效 | 用户当前仅用 148；150 迁移崩溃仍未修复，不因范围缩小改成通过。没有借普通恢复的新接口宣称迁移 tokenless 已恢复。 |
| 双 148 独立环境与当前 Clash HTTPS | 实际通过（初次双开） | Direct 和 Clash 两环境同时运行，PID、创建时间及 profile 独立，精确 148 相同；真实浏览器 HTTPS 页面出口与本次预检一致。后续重开崩溃使“多实例稳定使用”仍阻塞。 |
| 代理故障不回退直连、原环境恢复 | 实际通过（受控范围） | 关闭本任务自己的代理 helper，原测试网页仍可达，但目标受保护浏览器被停止，原策略保留且观察器无新增流量；恢复 helper 后原 ID 与原三存储读回成功。不是完整 DNS/UDP/WebRTC 或全部远端故障矩阵。 |
| Cookie 真正受理后取消与准确范围 | 实际通过（部分完成/失败/取消） | 148 单独运行的空白导入会话实际受理一次 128 项 merge。原任务取消后第 1–99 条 written/verified，第 100 条 `COOKIE_READ_FAILED`，第 101–128 条 cancelled，合计 128、unknown=0。关闭重开窗口只重新 Prepare 同一输入，实际内核现存同键为 99；没有再次导入或重试写。环境和指纹记录不变。该第二次同键读取不等于再次逐值核对，不宣称 128 项全部成功。 |
| 完整备份、预检、恢复真实数据 | 实际通过 | 原 Direct 的 300 个备份条目受理、只读预检与明确覆盖 1 项成功；恢复前真实三存储为 NEW，恢复后真实 Cookie/LocalStorage/IndexedDB 均读回 OLD，原指纹身份保持。 |

独立只读核对 `repair9-data-audit.json` 确认：原 73 项保存身份零差异；两次预检清理故障前后的 73 项环境/指纹全行零差异；批次 128 项最终完整指纹与原 journal 匹配、身份互异，原 21 项各只创建一次；创建后打开失败/重试的全部 204 项保存环境/指纹行不变。恢复读回用准确窗口确认 NEW→恢复提交→OLD，未拿后续 Cookie 操作覆盖该历史结果。2026-10-07 06:46:24 UTC 只读 SQLite 完整性为 ok、外键违规 0。这些是上述流程的数据核对，不另加正式桌面测试数量。

## 148 崩溃与剩余限制

两个精确自有根 Chrome 都在启动后约一秒出现 `chrome.dll 148.0.7778.215 / 80000003 / offset 071c34db`；Windows 事件 PID、完整创建时间与安装路径均匹配。第一次发生在 Cookie 确认前 9.567 秒，服务没有受理 Cookie 导入，所以不能说是 Cookie 写入导致；第二次发生于普通打开，说明并不限于 Cookie 流程。

两次失败时另有一个自有受保护代理环境运行；停止另一个环境后，原 Direct 在保留身份与数据的条件下重开成功。这是条件对照，尚不能断言并发、ACL、内存或 Chromium 某条断言是根因。事后内存、ACL 与独立 Job/pipe/SID 核对未提供根因证据；不能用事后正常值替代故障瞬间证据。没有更换内核、关闭沙箱、修改指纹或重建 profile 来制造通过。

用户进一步要求核对开源源地址和本地接入。官方项目为 [adryfish/fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium)，源码须看[固定 148 标签](https://github.com/adryfish/fingerprint-chromium/tree/13b89eae304123f0710f2d33fd0816a2f61d7ffc)，不能用 main 的说明代替该版本补丁。已核对官方 release asset 摘要、指纹编译器与实际 Direct 命令，没有使用被上游删除的 GPU vendor/renderer/disable-gpu-fingerprint 参数，也没有关闭 GPU 或沙箱。148 GPU 补丁作用于 Blink 读值，并不改 GPU 进程启动；上游 README 的旧 Linux 描述与该标签 Windows/macOS 配置池不一致，以固定源码为准。对照记录在 ignored `upstream-148-parameters/`，不表示整条 Windows 启动管线已证明无误。

一次精确 PID 的诊断复现取得 9MB mini dump：故障线程栈中有 `gpu_data_manager_impl_private.cc:418` 的 GPU 不可用 fatal。精确 DLL 指令显示，原断点地址 `071c34db` 的 `int3` 后紧接 `071c34dc` 的 `ud2`；调试器下此次终结码为 `c000001d`。这是同一 fatal 出口的指令证据，不能仅凭共有出口断言先前两次也一定由同一 GPU 原因触发。[Chromium 固定 148 代码](https://chromium.googlesource.com/chromium/src/+/refs/tags/148.0.7778.215/content/browser/gpu/gpu_data_manager_impl_private.cc#1710)在备用 GPU 模式耗尽时调用该 fatal。

随后用原 Direct 的相同 148 文件、seed、profile 和 14 个原参数，仅移除私有 CDP pipe 并增加日志，以独立 `ProcessStartInfo` 启动：Clash 环境运行时，根进程 35104 的 GPU 连续 6 次以 `0xC0000022 / STATUS_ACCESS_DENIED` 退出，再触发上述 fatal；正常关闭该 Clash 环境后，根进程 54824 与 GPU 23640 存活超过 10 秒，正常关窗退出 0。两轮均不经过 Wails Job 或私有 HANDLE_LIST，因此不能把问题直接归结为 CDP 参数。原生管理程序仍为 preview9；这些独立启动是诊断对照，不冒充完整桌面验收。

再次保持 preview9 的同一个 Clash 会话 79296 运行，将精确 148 清单内 76 个文件复制到独立诊断目录，源与副本 SHA-256 全部相同；同一 Direct 的 seed/profile/14 个参数不变，仅换为该内核副本路径。根进程 22404 与 GPU 80704 实际存活 10.414 秒，正常关窗，根及 7 个观察到的后代均退出 0。副本继承诊断目录权限，没有原共享内核上的当前 package SID。

在这个同一 Clash 会话期间，给同一副本加入原内核当前 package SID 的 RX 允许项后，根进程 34692 失败，GPU 六次 `C0000022` 后 fatal；同会话下原内核路径的根进程 25328 也失败。撤销副本新加的 package 项后，根进程 31396 与 GPU 70112 再次存活 10.533 秒，正常关窗、根及 8 个观察到的后代全部退出 0。这组 PASS→FAIL→PASS 真实对照确认本程序当前共享内核的 package ACL 方案触发此机器故障；不能继续把它笼统归咎于上游内核。具体受拒绝的加载调用或对象仍待捕获。

第一组添加/撤销使用 PowerShell `Set-Acl`，不声称它与产品调用完全相同。后续独立结构重放核实 80 个对象仅增加该 DACL Allow，owner/group/control/LABEL 不变，撤销后完整 SD 摘要恢复。进一步用产品相同的原生 `SetSecurityInfo`（flags=4，owner/group/SACL 均 NULL），在副本加入 package RX 与 Restricted Code RX 的候选后，根进程 62840 仍因六次 GPU `C0000022` 后失败。启动前其他 ACE 字节和安全描述符其余字段不变；结束后仅撤回这两个新增 SID，80 份完整 SD 摘要全部恢复。**补 RC RX 的候选未解决问题，候选源码已精确撤回，没有构建或交付。** 全程未改原内核权限、保存的内核绑定、profile 权限、seed、沙箱或实际 Clash。

一个接入候选已作反证：隔离自有进程复刻 `CreateWindowStationW(NULL,...)` 后，实际当前窗口站始终 WinSta0、桌面始终 Default，准确新句柄正常关闭；没有更改 ACL 或设置窗口站。不能根据 API 摘要中的 associates 一词就假定本程序切错桌面并盲改启动方式。该对照不是实际 GPU 子进程的权限证明。

当前仍不能声称“一键配置指纹、多实例管理”已经达到稳定完整交付标准。原数据保护和失败可重试已经核实；多实例重开崩溃是实际使用阻塞。自然迟到指纹回复、实际 Clash 节点链、未知来源跨进程恢复，以及未列入本轮的外部全协议场景也不宣称通过。正式历史计数 **4/21** 不变，不用本轮局部结果填满原整项条件。

## 启动使用

日常入口为 `build/bin/prism-browser.exe`。正常关闭已有棱镜工作台后再启动；没有覆盖环境变量时使用 `%LOCALAPPDATA%\PrismBrowser`，已有数据继续使用。只读检查和本轮测试都没有把该真实数据根当夹具。

在“内核”确认精确 148 安装，创建环境时明确选择 148，再选直连或现有 Clash 代理；自动指纹保存后保持稳定。已创建而打开失败，使用该行“重试打开”，不要重复创建。当前多实例重开仍可能崩溃，单独重开只是已观测到的临时恢复路径，不能保证问题已解决。
